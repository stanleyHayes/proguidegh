package auth

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// Cookie names for the session model (spec §15.1).
// Renamed gg_* → pgh_* in Phase 0b name consolidation (NC-05); the rename
// invalidates every live session — safe pre-launch only.
const (
	AccessCookieName  = "pgh_access"
	RefreshCookieName = "pgh_refresh"
)

// RefreshTokenTTL is the lifetime of a refresh session (30 days).
const RefreshTokenTTL = 30 * 24 * time.Hour

// SetSessionCookies writes the access/refresh cookies: HttpOnly,
// SameSite=Lax, Secure outside local dev (spec §15.1).
func SetSessionCookies(w http.ResponseWriter, secure bool, accessToken, refreshToken string) {
	http.SetCookie(w, sessionCookie(secure, AccessCookieName, accessToken, AccessTokenTTL))
	http.SetCookie(w, sessionCookie(secure, RefreshCookieName, refreshToken, RefreshTokenTTL))
}

// ClearSessionCookies expires both session cookies (logout / revocation).
func ClearSessionCookies(w http.ResponseWriter, secure bool) {
	for _, name := range []string{AccessCookieName, RefreshCookieName} {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
			Expires:  time.Unix(0, 0),
		})
	}
}

// RefreshFromRequest extracts the refresh token, in priority order:
//  1. the X-Refresh-Token header (native mobile clients, M-05),
//  2. a refresh_token field in the JSON request body (native mobile clients),
//  3. the pgh_refresh session cookie (web clients).
//
// Rotation, reuse detection and revocation are identical across transports —
// the service layer never sees which one carried the token.
func RefreshFromRequest(r *http.Request) string {
	if v := strings.TrimSpace(r.Header.Get("X-Refresh-Token")); v != "" {
		return v
	}
	if v := refreshFromBody(r); v != "" {
		return v
	}
	if c, err := r.Cookie(RefreshCookieName); err == nil {
		return c.Value
	}
	return ""
}

// refreshFromBody reads an optional JSON body for a refresh_token field. The
// body is consumed; refresh/logout handlers never read it again. Malformed
// or empty bodies are not errors — the cookie fallback still applies.
func refreshFromBody(r *http.Request) string {
	if r.Body == nil {
		return ""
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil || len(raw) == 0 {
		return ""
	}
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return ""
	}
	return strings.TrimSpace(body.RefreshToken)
}

// AccessFromRequest extracts the access token, preferring the cookie and
// falling back to the Authorization: Bearer header (API/test clients).
func AccessFromRequest(r *http.Request) string {
	if c, err := r.Cookie(AccessCookieName); err == nil && c.Value != "" {
		return c.Value
	}
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) > len(prefix) && h[:len(prefix)] == prefix {
		return h[len(prefix):]
	}
	return ""
}

func sessionCookie(secure bool, name, value string, ttl time.Duration) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(ttl),
	}
}
