package auth

import (
	"testing"
	"time"
)

// RFC 6238 Appendix B test vectors use the ASCII secret "12345678901234567890"
// (base32: GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ) with SHA-1 and 8-digit codes.
// Our codes are the 6 low digits of the same HMAC block.
func TestTOTPCodeKnownVector(t *testing.T) {
	const secret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	vectors := []struct {
		unix     int64
		eightDig string
	}{
		{59, "94287082"},
		{1111111109, "07081804"},
		{1111111111, "14050471"},
		{1234567890, "89005924"},
		{2000000000, "69279037"},
		{20000000000, "65353130"},
	}
	for _, v := range vectors {
		code, err := TOTPCode(secret, time.Unix(v.unix, 0))
		if err != nil {
			t.Fatalf("totp at %d: %v", v.unix, err)
		}
		want := v.eightDig[2:] // 6-digit truncation of the 8-digit vector
		if code != want {
			t.Fatalf("totp at %d: got %s, want %s", v.unix, code, want)
		}
	}
}

func TestVerifyTOTPAcceptsSkew(t *testing.T) {
	secret, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	now := time.Unix(1_700_000_000, 0)
	code, err := TOTPCode(secret, now)
	if err != nil {
		t.Fatalf("code: %v", err)
	}
	if !VerifyTOTP(secret, code, now) {
		t.Fatal("expected code to verify at same instant")
	}
	if !VerifyTOTP(secret, code, now.Add(30*time.Second)) {
		t.Fatal("expected code to verify one step ahead (clock skew)")
	}
	if VerifyTOTP(secret, code, now.Add(5*time.Minute)) {
		t.Fatal("expected code to fail far outside the skew window")
	}
	if VerifyTOTP(secret, "abcdef", now) {
		t.Fatal("expected malformed code to fail")
	}
}

func TestEncryptDecryptSecretRoundtrip(t *testing.T) {
	enc, err := EncryptSecret("app-secret", "GEZDGNBVGY3TQOJQ")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if enc == "GEZDGNBVGY3TQOJQ" {
		t.Fatal("ciphertext must differ from plaintext")
	}
	dec, err := DecryptSecret("app-secret", enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if dec != "GEZDGNBVGY3TQOJQ" {
		t.Fatalf("roundtrip mismatch: %s", dec)
	}
	if _, err := DecryptSecret("other-secret", enc); err == nil {
		t.Fatal("expected decrypt to fail with the wrong key")
	}
}
