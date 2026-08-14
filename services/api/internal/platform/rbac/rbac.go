// Package rbac enforces the authorization layer required by spec §3:
// permissions, not UI visibility, enforce authorization. Every privileged
// handler runs behind RequireAuth + RequirePermission, and services can
// additionally call Has for self-scope checks.
package rbac

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"proguidegh/api/internal/platform/auth"
	"proguidegh/api/internal/platform/httpx"
)

// cacheTTL is how long a user's permission set lives in Redis. Kept short so
// role changes propagate quickly; Invalidate clears it immediately anyway.
const cacheTTL = 60 * time.Second

type ctxKey struct{}

// Identity is the authenticated principal stored on the request context.
type Identity struct {
	UserID    string
	SessionID string
	Perms     map[string]struct{}
}

// Has reports whether the identity holds the given permission code.
func (id Identity) Has(code string) bool {
	_, ok := id.Perms[code]
	return ok
}

// FromContext returns the authenticated identity, or false when absent.
func FromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(ctxKey{}).(Identity)
	return id, ok
}

// Store loads and caches user permission codes
// (user_roles → role_permissions → permissions).
type Store struct {
	pool *pgxpool.Pool
	rdb  *goredis.Client
}

// NewStore builds the permission store.
func NewStore(pool *pgxpool.Pool, rdb *goredis.Client) *Store {
	return &Store{pool: pool, rdb: rdb}
}

func cacheKey(userID string) string { return "rbac:perms:" + userID }

// Permissions returns the permission codes for a user, served from the Redis
// cache when present.
// AccountActive reports whether the user may still authenticate.
//
// Deliberately NOT cached, unlike Permissions. Access tokens are stateless
// JWTs that stay cryptographically valid for their full lifetime, so this
// lookup is the only thing standing between a suspended or deleted account and
// a bearer token that still looks good. Spec §15.2 requires sessions to be
// suspended or revoked on compromise or role removal; a cached answer would
// mean "revoked" took effect a minute late. It is a primary-key lookup.
func (s *Store) AccountActive(ctx context.Context, userID string) (bool, error) {
	var status string
	err := s.pool.QueryRow(ctx, `SELECT status FROM users WHERE id = $1`, userID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("rbac: account status: %w", err)
	}
	return status == "active", nil
}

func (s *Store) Permissions(ctx context.Context, userID string) ([]string, error) {
	if s.rdb != nil {
		if raw, err := s.rdb.Get(ctx, cacheKey(userID)).Bytes(); err == nil {
			var codes []string
			if json.Unmarshal(raw, &codes) == nil {
				return codes, nil
			}
		}
	}

	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT p.code
		FROM user_roles ur
		JOIN role_permissions rp ON rp.role_id = ur.role_id
		JOIN permissions p ON p.id = rp.permission_id
		WHERE ur.user_id = $1
		ORDER BY p.code`, userID)
	if err != nil {
		return nil, fmt.Errorf("rbac: load permissions: %w", err)
	}
	defer rows.Close()

	codes := []string{}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, fmt.Errorf("rbac: scan permission: %w", err)
		}
		codes = append(codes, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rbac: read permissions: %w", err)
	}

	if s.rdb != nil {
		if raw, err := json.Marshal(codes); err == nil {
			s.rdb.Set(ctx, cacheKey(userID), raw, cacheTTL) //nolint:errcheck // best-effort cache
		}
	}
	return codes, nil
}

// Roles returns the role codes currently assigned to a user.
func (s *Store) Roles(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.code FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		WHERE ur.user_id = $1
		ORDER BY r.code`, userID)
	if err != nil {
		return nil, fmt.Errorf("rbac: load roles: %w", err)
	}
	defer rows.Close()

	codes := []string{}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, fmt.Errorf("rbac: scan role: %w", err)
		}
		codes = append(codes, c)
	}
	return codes, rows.Err()
}

// Invalidate drops the cached permission set for a user. Call it on every
// role change (spec §15.2: suspend/revoke on role removal).
func (s *Store) Invalidate(ctx context.Context, userID string) {
	if s.rdb != nil {
		s.rdb.Del(ctx, cacheKey(userID)) //nolint:errcheck // best-effort cache
	}
}

// Middleware bundles authentication and authorization middleware.
type Middleware struct {
	issuer *auth.TokenIssuer
	store  *Store
}

// NewMiddleware builds the RBAC middleware.
func NewMiddleware(issuer *auth.TokenIssuer, store *Store) *Middleware {
	return &Middleware{issuer: issuer, store: store}
}

// RequireAuth authenticates the request (access-token cookie or Bearer
// header), loads the live permission set, and attaches Identity to the
// context. Unauthenticated requests get 401 UNAUTHENTICATED.
func (m *Middleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := auth.AccessFromRequest(r)
		if token == "" {
			httpx.WriteError(w, r, http.StatusUnauthorized,
				"UNAUTHENTICATED", "authentication required", nil)
			return
		}
		claims, err := m.issuer.Verify(token)
		if err != nil {
			code := "UNAUTHENTICATED"
			if errors.Is(err, auth.ErrTokenExpired) {
				code = "TOKEN_EXPIRED"
			}
			httpx.WriteError(w, r, http.StatusUnauthorized,
				code, "invalid or expired access token", nil)
			return
		}
		// A valid signature is not enough: the account behind it may have been
		// suspended or deleted since the token was issued.
		active, err := m.store.AccountActive(r.Context(), claims.Subject)
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError,
				"INTERNAL", "could not verify account", nil)
			return
		}
		if !active {
			httpx.WriteError(w, r, http.StatusUnauthorized,
				"ACCOUNT_INACTIVE", "this account is no longer active", nil)
			return
		}
		perms, err := m.store.Permissions(r.Context(), claims.Subject)
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError,
				"INTERNAL", "could not load permissions", nil)
			return
		}
		id := Identity{UserID: claims.Subject, SessionID: claims.SessionID, Perms: map[string]struct{}{}}
		for _, p := range perms {
			id.Perms[p] = struct{}{}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, id)))
	})
}

// RequirePermission returns middleware (to be wrapped in RequireAuth) that
// rejects the request with 403 FORBIDDEN unless the identity holds code.
func RequirePermission(code string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, ok := FromContext(r.Context())
			if !ok {
				httpx.WriteError(w, r, http.StatusUnauthorized,
					"UNAUTHENTICATED", "authentication required", nil)
				return
			}
			if !id.Has(code) {
				httpx.WriteError(w, r, http.StatusForbidden,
					"FORBIDDEN", "missing required permission",
					map[string]string{"permission": code})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// HasRole reports whether the identity's role list contains code. Convenience
// for services that branch on coarse roles in addition to permissions.
func HasRole(roles []string, code string) bool {
	for _, r := range roles {
		if strings.EqualFold(r, code) {
			return true
		}
	}
	return false
}
