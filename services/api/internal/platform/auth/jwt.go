package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// AccessTokenTTL is the lifetime of an access token (spec §15.1: short-lived).
const AccessTokenTTL = 15 * time.Minute

// Claims is the payload of the HMAC-SHA256 access token (JWT, HS256).
// Perms carries a snapshot of permission codes at issue time; handlers still
// re-check against the (Redis-cached) live permission set.
type Claims struct {
	Subject   string    `json:"sub"` // user id
	SessionID string    `json:"sid"` // refresh session id
	Perms     []string  `json:"perms,omitempty"`
	IssuedAt  time.Time `json:"iat"`
	ExpiresAt time.Time `json:"exp"`
}

// TokenIssuer signs and verifies HS256 JWTs with the configured secret.
type TokenIssuer struct {
	secret []byte
	now    func() time.Time // overridable in tests
}

// NewTokenIssuer builds an issuer. Secret must be non-empty.
func NewTokenIssuer(secret string) (*TokenIssuer, error) {
	if secret == "" {
		return nil, errors.New("auth: JWT_OR_SESSION_SECRET is required")
	}
	return &TokenIssuer{secret: []byte(secret), now: time.Now}, nil
}

// Sign issues a signed JWT for the given claims, stamping iat/exp.
func (t *TokenIssuer) Sign(c Claims) (string, error) {
	if c.Subject == "" {
		return "", errors.New("auth: claims subject required")
	}
	c.IssuedAt = t.now().UTC()
	c.ExpiresAt = c.IssuedAt.Add(AccessTokenTTL)

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("auth: marshal claims: %w", err)
	}
	body := header + "." + base64.RawURLEncoding.EncodeToString(payload)
	return body + "." + base64.RawURLEncoding.EncodeToString(t.mac(body)), nil
}

// ErrTokenExpired is returned by Verify when the token is past its expiry.
var ErrTokenExpired = errors.New("auth: token expired")

// ErrTokenInvalid is returned by Verify for malformed or mis-signed tokens.
var ErrTokenInvalid = errors.New("auth: invalid token")

// Verify parses and validates a JWT, returning its claims.
func (t *TokenIssuer) Verify(token string) (Claims, error) {
	var c Claims
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return c, ErrTokenInvalid
	}
	body := parts[0] + "." + parts[1]
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(sig, t.mac(body)) {
		return c, ErrTokenInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || json.Unmarshal(payload, &c) != nil {
		return c, ErrTokenInvalid
	}
	if c.Subject == "" || c.ExpiresAt.IsZero() {
		return c, ErrTokenInvalid
	}
	if !t.now().Before(c.ExpiresAt) {
		return c, ErrTokenExpired
	}
	return c, nil
}

func (t *TokenIssuer) mac(body string) []byte {
	h := hmac.New(sha256.New, t.secret)
	h.Write([]byte(body))
	return h.Sum(nil)
}

// NewOpaqueToken returns a URL-safe random token (refresh tokens, OTP
// delivery refs). 256 bits of entropy.
func NewOpaqueToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth: generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashToken returns the sha256 hex digest used to store tokens and OTP codes
// at rest — the raw value never touches the database or logs.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
