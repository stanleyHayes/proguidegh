package auth

import (
	"errors"
	"testing"
	"time"
)

func newTestIssuer(t *testing.T) *TokenIssuer {
	t.Helper()
	iss, err := NewTokenIssuer("test-secret")
	if err != nil {
		t.Fatalf("issuer: %v", err)
	}
	return iss
}

func TestJWTSignVerifyRoundtrip(t *testing.T) {
	iss := newTestIssuer(t)
	token, err := iss.Sign(Claims{Subject: "user-1", SessionID: "sess-1", Perms: []string{"users.read"}})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	claims, err := iss.Verify(token)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.Subject != "user-1" || claims.SessionID != "sess-1" {
		t.Fatalf("claims mismatch: %+v", claims)
	}
	if len(claims.Perms) != 1 || claims.Perms[0] != "users.read" {
		t.Fatalf("perms mismatch: %+v", claims.Perms)
	}
	if claims.ExpiresAt.Sub(claims.IssuedAt) != AccessTokenTTL {
		t.Fatalf("exp-iat = %v, want %v", claims.ExpiresAt.Sub(claims.IssuedAt), AccessTokenTTL)
	}
}

func TestJWTVerifyRejectsTampering(t *testing.T) {
	iss := newTestIssuer(t)
	token, err := iss.Sign(Claims{Subject: "user-1", SessionID: "s"})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	// Flip a character in the payload.
	raw := []byte(token)
	raw[len(raw)/2] ^= 1
	if _, err := iss.Verify(string(raw)); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("expected ErrTokenInvalid, got %v", err)
	}

	// Wrong secret.
	other, _ := NewTokenIssuer("other-secret")
	if _, err := other.Verify(token); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("expected ErrTokenInvalid for wrong secret, got %v", err)
	}

	if _, err := iss.Verify("garbage"); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("expected ErrTokenInvalid for malformed token, got %v", err)
	}
}

func TestJWTVerifyExpiry(t *testing.T) {
	iss := newTestIssuer(t)
	past := time.Now().Add(-time.Hour)
	iss.now = func() time.Time { return past }
	token, err := iss.Sign(Claims{Subject: "user-1", SessionID: "s"})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	iss.now = time.Now
	if _, err := iss.Verify(token); !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("expected ErrTokenExpired, got %v", err)
	}
}

func TestHashTokenDeterministic(t *testing.T) {
	a := HashToken("token")
	b := HashToken("token")
	if a != b || len(a) != 64 {
		t.Fatalf("unexpected hash: %q vs %q", a, b)
	}
}
