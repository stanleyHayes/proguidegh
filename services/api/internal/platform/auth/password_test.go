package auth

import (
	"strings"
	"testing"
)

func TestHashPasswordRoundtrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Fatalf("unexpected hash format: %s", hash)
	}
	ok, err := VerifyPassword("correct horse battery staple", hash)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !ok {
		t.Fatal("expected password to verify")
	}
}

func TestVerifyPasswordWrongPassword(t *testing.T) {
	hash, err := HashPassword("hunter2-hunter2")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	ok, err := VerifyPassword("hunter3", hash)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if ok {
		t.Fatal("expected mismatch for wrong password")
	}
}

func TestVerifyPasswordMalformed(t *testing.T) {
	for _, bad := range []string{"", "not-a-hash", "$argon2i$v=19$m=65536,t=3,p=2$aaaa$bbbb", "$argon2id$v=16$m=1,t=1,p=1$aa$bb"} {
		if _, err := VerifyPassword("x", bad); err == nil {
			t.Fatalf("expected error for malformed hash %q", bad)
		}
	}
}

func TestHashPasswordEmpty(t *testing.T) {
	if _, err := HashPassword(""); err == nil {
		t.Fatal("expected error for empty password")
	}
}
