package rbac

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func withIdentity(perms ...string) *http.Request {
	id := Identity{UserID: "u1", Perms: map[string]struct{}{}}
	for _, p := range perms {
		id.Perms[p] = struct{}{}
	}
	return httptest.NewRequest(http.MethodGet, "/", nil).
		WithContext(context.WithValue(context.Background(), ctxKey{}, id))
}

func TestRequirePermissionDecision(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	tests := []struct {
		name       string
		perms      []string
		required   string
		wantStatus int
	}{
		{"allowed with exact code", []string{"users.read"}, "users.read", http.StatusNoContent},
		{"allowed among many", []string{"guides.read", "users.read"}, "users.read", http.StatusNoContent},
		{"denied without code", []string{"guides.read"}, "users.read", http.StatusForbidden},
		{"denied with empty set", nil, "users.read", http.StatusForbidden},
		{"prefix is not enough", []string{"users"}, "users.read", http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			RequirePermission(tt.required)(okHandler).ServeHTTP(rec, withIdentity(tt.perms...))
			if rec.Code != tt.wantStatus {
				t.Fatalf("got %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestRequirePermissionWithoutIdentity(t *testing.T) {
	rec := httptest.NewRecorder()
	RequirePermission("users.read")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", rec.Code)
	}
}

func TestForbiddenEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	RequirePermission("users.read")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {})).
		ServeHTTP(rec, withIdentity())
	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403", rec.Code)
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Details struct {
				Permission string `json:"permission"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != "FORBIDDEN" || body.Error.Details.Permission != "users.read" {
		t.Fatalf("unexpected envelope: %s", rec.Body.String())
	}
}

func TestIdentityHas(t *testing.T) {
	id := Identity{UserID: "u1", Perms: map[string]struct{}{"a.b": {}}}
	if !id.Has("a.b") {
		t.Fatal("expected Has(a.b)")
	}
	if id.Has("a.c") {
		t.Fatal("unexpected Has(a.c)")
	}
}

func TestHasRole(t *testing.T) {
	if !HasRole([]string{"tourist", "guide"}, "guide") {
		t.Fatal("expected role match")
	}
	if HasRole([]string{"tourist"}, "administrator") {
		t.Fatal("unexpected role match")
	}
}
