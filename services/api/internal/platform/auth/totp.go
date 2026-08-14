package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// TOTP parameters (RFC 6238): SHA-1, 6 digits, 30-second step, ±1 step of
// clock-drift tolerance.
const (
	totpDigits    = 6
	totpPeriod    = 30
	totpSkew      = 1
	totpSecretLen = 20 // 160 bits, base32-encodes to 32 chars
)

// GenerateTOTPSecret returns a new base32 TOTP secret (no padding).
func GenerateTOTPSecret() (string, error) {
	b := make([]byte, totpSecretLen)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth: generate totp secret: %w", err)
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b), nil
}

// TOTPURI builds an otpauth:// provisioning URI for authenticator apps.
func TOTPURI(issuer, account, secret string) string {
	label := url.PathEscape(issuer) + ":" + url.PathEscape(account)
	q := url.Values{
		"secret": {secret},
		"issuer": {issuer},
		"digits": {"6"},
		"period": {"30"},
	}
	return "otpauth://totp/" + label + "?" + q.Encode()
}

// TOTPCode computes the TOTP code for the given base32 secret at time t.
func TOTPCode(secret string, t time.Time) (string, error) {
	key, err := decodeBase32Secret(secret)
	if err != nil {
		return "", err
	}
	counter := uint64(t.Unix() / totpPeriod)
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	code := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", totpDigits, code%1_000_000), nil
}

// VerifyTOTP reports whether code matches the secret within ±totpSkew steps.
func VerifyTOTP(secret, code string, at time.Time) bool {
	if len(code) != totpDigits {
		return false
	}
	for step := -totpSkew; step <= totpSkew; step++ {
		want, err := TOTPCode(secret, at.Add(time.Duration(step)*totpPeriod*time.Second))
		if err != nil {
			return false
		}
		if hmac.Equal([]byte(want), []byte(code)) {
			return true
		}
	}
	return false
}

func decodeBase32Secret(secret string) ([]byte, error) {
	s := strings.ToUpper(strings.TrimSpace(secret))
	if pad := len(s) % 8; pad != 0 {
		s += strings.Repeat("=", 8-pad)
	}
	key, err := base32.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("auth: decode totp secret: %w", err)
	}
	return key, nil
}

// EncryptSecret / DecryptSecret protect TOTP secrets at rest using AES-256-GCM
// keyed by the application secret (mfa_secrets.totp_secret_encrypted, spec
// §15.3 field-level protection). The output is base64(nonce || ciphertext).

// EncryptSecret encrypts plaintext with a key derived from appSecret.
func EncryptSecret(appSecret, plaintext string) (string, error) {
	gcm, err := secretGCM(appSecret)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("auth: encrypt nonce: %w", err)
	}
	out := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(out), nil
}

// DecryptSecret reverses EncryptSecret.
func DecryptSecret(appSecret, encoded string) (string, error) {
	gcm, err := secretGCM(appSecret)
	if err != nil {
		return "", err
	}
	raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(encoded)
	if err != nil {
		return "", errors.New("auth: malformed encrypted secret")
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("auth: malformed encrypted secret")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", errors.New("auth: decrypt secret failed")
	}
	return string(plain), nil
}

func secretGCM(appSecret string) (cipher.AEAD, error) {
	if appSecret == "" {
		return nil, errors.New("auth: app secret required for field encryption")
	}
	key := sha256.Sum256([]byte("guide-ghana/mfa/v1/" + appSecret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
