package payouts

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// Payout-account destination references are encrypted at rest
// (AES-256-GCM) into payout_accounts.account_ref_tokenized. The plaintext
// only ever leaves the database in the finance CSV export; every other
// surface shows the masked form.

// deriveKey builds the 32-byte AES key. An explicit PAYOUT_ACCOUNT_KEY
// wins; otherwise the key is derived from the JWT/session secret so local
// development works without extra configuration.
func deriveKey(explicit, fallbackSecret string) []byte {
	secret := explicit
	if secret == "" {
		secret = fallbackSecret
	}
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}

// encryptRef seals a plaintext account reference, returning base64
// (nonce prepended).
func encryptRef(key []byte, plaintext string) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("payouts: nonce: %w", err)
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(plaintext), nil)), nil
}

// decryptRef opens a base64 sealed reference.
func decryptRef(key []byte, encoded string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("payouts: decode ref: %w", err)
	}
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("payouts: ciphertext too short")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", fmt.Errorf("payouts: open ref: %w", err)
	}
	return string(plain), nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("payouts: cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("payouts: gcm: %w", err)
	}
	return gcm, nil
}

// maskRef renders a reference for display: last four characters only.
func maskRef(ref string) string {
	if len(ref) <= 4 {
		return "****"
	}
	return "****" + ref[len(ref)-4:]
}
