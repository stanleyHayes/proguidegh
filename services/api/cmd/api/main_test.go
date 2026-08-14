package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"proguidegh/api/internal/auth"
	"proguidegh/api/internal/migrations"
	"proguidegh/api/internal/platform/audit"
	pauth "proguidegh/api/internal/platform/auth"
	"proguidegh/api/internal/platform/config"
	"proguidegh/api/internal/platform/db"
	"proguidegh/api/internal/platform/observability"
	"proguidegh/api/internal/platform/rbac"
	"proguidegh/api/internal/platform/redis"
	"proguidegh/api/internal/platform/storage"
)

// integrationEnv connects to the test Postgres/Redis, applies migrations and
// builds the real route tree. Skips when DATABASE_URL/REDIS_URL are unset.
type integrationEnv struct {
	t      *testing.T
	server *httptest.Server
	pool   *pgxpool.Pool
	rdb    *goredis.Client
}

func newIntegrationEnv(t *testing.T) *integrationEnv {
	t.Helper()
	dbURL, redisURL := os.Getenv("DATABASE_URL"), os.Getenv("REDIS_URL")
	if dbURL == "" || redisURL == "" {
		t.Skip("DATABASE_URL/REDIS_URL not set; skipping integration test")
	}
	if os.Getenv("JWT_OR_SESSION_SECRET") == "" {
		t.Setenv("JWT_OR_SESSION_SECRET", "integration-test-secret")
	}

	cfg := config.Load()
	ctx := context.Background()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	rdb, err := redis.Connect(ctx, cfg.RedisURL)
	if err != nil {
		t.Fatalf("connect redis: %v", err)
	}
	if _, err := migrations.Up(ctx, pool); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	// Rate-limit buckets are per-IP sliding windows in the shared dev Redis;
	// the suite drives many logins from one test client, so clear leftover
	// quota from previous runs to keep tests isolated.
	if keys, err := rdb.Keys(ctx, "rl:*").Result(); err == nil && len(keys) > 0 {
		rdb.Del(ctx, keys...) //nolint:errcheck // best-effort cleanup
	}

	uploads := t.TempDir()
	t.Setenv("STORAGE_LOCAL_DIR", uploads)
	cfg = config.Load()

	handler, err := buildHandler(cfg, pool, rdb, observability.NewLogger("test"))
	if err != nil {
		t.Fatalf("build handler: %v", err)
	}
	env := &integrationEnv{t: t, server: httptest.NewServer(handler.handler), pool: pool, rdb: rdb}
	t.Cleanup(func() {
		env.server.Close()
		pool.Close()
		rdb.Close() //nolint:errcheck
	})
	return env
}

func uniqueEmail(t *testing.T) string {
	t.Helper()
	token, err := pauth.NewOpaqueToken()
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	return strings.ToLower(fmt.Sprintf("it-%s@example.com", token[:12]))
}

// post issues a JSON POST and returns status + decoded body.
func (e *integrationEnv) post(path string, body any, headers map[string]string) (int, map[string]any) {
	e.t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		e.t.Fatalf("marshal: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, e.server.URL+path, bytes.NewReader(raw))
	if err != nil {
		e.t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := e.server.Client().Do(req)
	if err != nil {
		e.t.Fatalf("do %s: %v", path, err)
	}
	defer resp.Body.Close() //nolint:errcheck
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func (e *integrationEnv) get(path string, headers map[string]string) (int, map[string]any) {
	e.t.Helper()
	req, err := http.NewRequest(http.MethodGet, e.server.URL+path, nil)
	if err != nil {
		e.t.Fatalf("request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := e.server.Client().Do(req)
	if err != nil {
		e.t.Fatalf("do %s: %v", path, err)
	}
	defer resp.Body.Close() //nolint:errcheck
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func bearer(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

func refreshCookie(token string) map[string]string {
	return map[string]string{"Cookie": pauth.RefreshCookieName + "=" + token}
}

// registerAndLogin creates a tourist account and returns its tokens.
func (e *integrationEnv) registerAndLogin(email string) map[string]any {
	e.t.Helper()
	status, body := e.post("/api/v1/auth/register", map[string]any{
		"intent": "tourist", "email": email, "password": "passw0rd-passw0rd", "full_name": "Integration Tester",
	}, nil)
	if status != http.StatusCreated {
		e.t.Fatalf("register: got %d: %v", status, body)
	}
	return e.login(email)
}

func (e *integrationEnv) login(email string) map[string]any {
	e.t.Helper()
	status, body := e.post("/api/v1/auth/login", map[string]any{
		"email": email, "password": "passw0rd-passw0rd",
	}, nil)
	if status != http.StatusOK {
		e.t.Fatalf("login: got %d: %v", status, body)
	}
	return body
}

// grantRole assigns a role directly in SQL (test setup shortcut).
func (e *integrationEnv) grantRole(email, roleCode string) string {
	e.t.Helper()
	var userID string
	if err := e.pool.QueryRow(context.Background(), `
		INSERT INTO user_roles (user_id, role_id)
		SELECT u.id, r.id FROM users u, roles r
		WHERE u.email = $1 AND r.code = $2
		ON CONFLICT DO NOTHING
		RETURNING user_id`, email, roleCode).Scan(&userID); err != nil {
		if err := e.pool.QueryRow(context.Background(),
			`SELECT id FROM users WHERE email = $1`, email).Scan(&userID); err != nil {
			e.t.Fatalf("grant role lookup: %v", err)
		}
	}
	// Direct-SQL grants bypass the admin endpoint, so flush the permission
	// cache the endpoint would have invalidated.
	e.rdb.Del(context.Background(), "rbac:perms:"+userID) //nolint:errcheck
	return userID
}

func TestRegisterLoginRefreshReuse(t *testing.T) {
	env := newIntegrationEnv(t)
	email := uniqueEmail(t)
	tokens := env.registerAndLogin(email)

	access, _ := tokens["access_token"].(string)
	refresh, _ := tokens["refresh_token"].(string)
	if access == "" || refresh == "" {
		t.Fatalf("missing tokens in login response: %v", tokens)
	}

	// Access token authenticates a protected route.
	status, body := env.get("/api/v1/me/tourist-profile", bearer(access))
	if status != http.StatusOK {
		t.Fatalf("tourist profile: got %d: %v", status, body)
	}

	// Refresh rotates: new tokens issued.
	status, rotated := env.post("/api/v1/auth/refresh", nil, refreshCookie(refresh))
	if status != http.StatusOK {
		t.Fatalf("refresh: got %d: %v", status, rotated)
	}
	newRefresh, _ := rotated["refresh_token"].(string)
	if newRefresh == "" || newRefresh == refresh {
		t.Fatalf("rotation did not issue a new refresh token: %v", rotated)
	}

	// Reuse of the rotated-out token is detected and rejected.
	status, reuse := env.post("/api/v1/auth/refresh", nil, refreshCookie(refresh))
	if status != http.StatusUnauthorized {
		t.Fatalf("reuse: got %d, want 401: %v", status, reuse)
	}
	if errBody, ok := reuse["error"].(map[string]any); !ok || errBody["code"] != "SESSION_REUSE" {
		t.Fatalf("reuse: expected SESSION_REUSE envelope: %v", reuse)
	}

	// Reuse revoked the whole chain: the rotated token no longer works.
	status, _ = env.post("/api/v1/auth/refresh", nil, refreshCookie(newRefresh))
	if status != http.StatusUnauthorized {
		t.Fatalf("chain revocation: got %d, want 401", status)
	}
}

// TestNativeRefreshTransports covers M-05: the refresh token is accepted via
// the X-Refresh-Token header or a JSON body field (native clients), with the
// cookie as fallback — and rotation/reuse/revocation semantics are identical
// across transports.
func TestNativeRefreshTransports(t *testing.T) {
	env := newIntegrationEnv(t)

	header := func(tok string) map[string]string {
		return map[string]string{"X-Refresh-Token": tok}
	}

	// --- Header transport rotates; old token rejected; reuse kills the chain ---
	email := uniqueEmail(t)
	tokens := env.registerAndLogin(email)
	refresh, _ := tokens["refresh_token"].(string)

	status, rotated := env.post("/api/v1/auth/refresh", nil, header(refresh))
	if status != http.StatusOK {
		t.Fatalf("header refresh: got %d: %v", status, rotated)
	}
	newRefresh, _ := rotated["refresh_token"].(string)
	if newRefresh == "" || newRefresh == refresh {
		t.Fatalf("header refresh did not rotate: %v", rotated)
	}
	if access, _ := rotated["access_token"].(string); access == "" {
		t.Fatalf("header refresh missing access_token: %v", rotated)
	}

	// Reuse of the rotated-out token over the header: SESSION_REUSE, chain revoked.
	status, reuse := env.post("/api/v1/auth/refresh", nil, header(refresh))
	if status != http.StatusUnauthorized {
		t.Fatalf("header reuse: got %d, want 401: %v", status, reuse)
	}
	if errBody, ok := reuse["error"].(map[string]any); !ok || errBody["code"] != "SESSION_REUSE" {
		t.Fatalf("header reuse: expected SESSION_REUSE envelope: %v", reuse)
	}
	status, _ = env.post("/api/v1/auth/refresh", nil, header(newRefresh))
	if status != http.StatusUnauthorized {
		t.Fatalf("header chain revocation: got %d, want 401", status)
	}

	// --- Body transport rotates the same way ---
	tokens = env.login(email)
	refresh, _ = tokens["refresh_token"].(string)
	status, rotated = env.post("/api/v1/auth/refresh", map[string]any{"refresh_token": refresh}, nil)
	if status != http.StatusOK {
		t.Fatalf("body refresh: got %d: %v", status, rotated)
	}
	newRefresh, _ = rotated["refresh_token"].(string)
	if newRefresh == "" || newRefresh == refresh {
		t.Fatalf("body refresh did not rotate: %v", rotated)
	}
	// And the rotated-out body token is rejected.
	status, _ = env.post("/api/v1/auth/refresh", map[string]any{"refresh_token": refresh}, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("body reuse: got %d, want 401", status)
	}

	// --- Header wins over the cookie (priority order) ---
	tokens = env.login(email)
	refresh, _ = tokens["refresh_token"].(string)
	status, _ = env.post("/api/v1/auth/refresh", nil, map[string]string{
		"X-Refresh-Token": "bogus-token",
		"Cookie":          pauth.RefreshCookieName + "=" + refresh,
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("header-over-cookie priority: got %d, want 401 (header must win)", status)
	}
	// The cookie token is untouched by the failed header attempt and still rotates.
	status, rotated = env.post("/api/v1/auth/refresh", nil, refreshCookie(refresh))
	if status != http.StatusOK {
		t.Fatalf("cookie refresh regression: got %d: %v", status, rotated)
	}

	// --- Logout accepts all three transports ---
	// Header.
	tokens = env.login(email)
	refresh, _ = tokens["refresh_token"].(string)
	if status, _ = env.post("/api/v1/auth/logout", nil, header(refresh)); status != http.StatusOK {
		t.Fatalf("header logout: got %d", status)
	}
	if status, _ = env.post("/api/v1/auth/refresh", nil, header(refresh)); status != http.StatusUnauthorized {
		t.Fatalf("refresh after header logout: got %d, want 401", status)
	}
	// Body.
	tokens = env.login(email)
	refresh, _ = tokens["refresh_token"].(string)
	if status, _ = env.post("/api/v1/auth/logout", map[string]any{"refresh_token": refresh}, nil); status != http.StatusOK {
		t.Fatalf("body logout: got %d", status)
	}
	if status, _ = env.post("/api/v1/auth/refresh", nil, refreshCookie(refresh)); status != http.StatusUnauthorized {
		t.Fatalf("refresh after body logout: got %d, want 401", status)
	}
	// Cookie (existing behavior).
	tokens = env.login(email)
	refresh, _ = tokens["refresh_token"].(string)
	if status, _ = env.post("/api/v1/auth/logout", nil, refreshCookie(refresh)); status != http.StatusOK {
		t.Fatalf("cookie logout: got %d", status)
	}
	if status, _ = env.post("/api/v1/auth/refresh", nil, refreshCookie(refresh)); status != http.StatusUnauthorized {
		t.Fatalf("refresh after cookie logout: got %d, want 401", status)
	}
}

func TestPermissionDeniedWithoutUsersRead(t *testing.T) {
	env := newIntegrationEnv(t)
	tokens := env.registerAndLogin(uniqueEmail(t))
	access, _ := tokens["access_token"].(string)

	status, body := env.get("/api/v1/admin/users", bearer(access))
	if status != http.StatusForbidden {
		t.Fatalf("got %d, want 403: %v", status, body)
	}
	if errBody, ok := body["error"].(map[string]any); !ok || errBody["code"] != "FORBIDDEN" {
		t.Fatalf("expected FORBIDDEN envelope: %v", body)
	}

	// No token at all → 401.
	status, _ = env.get("/api/v1/admin/users", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", status)
	}
}

func TestRoleChangeWritesAuditRow(t *testing.T) {
	env := newIntegrationEnv(t)

	adminEmail := uniqueEmail(t)
	env.registerAndLogin(adminEmail)
	env.grantRole(adminEmail, "super_admin")
	adminTokens := env.login(adminEmail)
	adminAccess, _ := adminTokens["access_token"].(string)

	// Super admin can read the user list.
	status, body := env.get("/api/v1/admin/users", bearer(adminAccess))
	if status != http.StatusOK {
		t.Fatalf("admin list users: got %d: %v", status, body)
	}

	// Target user.
	targetEmail := uniqueEmail(t)
	env.registerAndLogin(targetEmail)
	targetID := env.grantRole(targetEmail, "tourist") // no-op; resolves the id

	// Change roles via the admin endpoint.
	req, err := http.NewRequest(http.MethodPatch, env.server.URL+"/api/v1/admin/users/"+targetID+"/roles",
		bytes.NewReader([]byte(`{"roles":["guide_applicant"]}`)))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminAccess)
	resp, err := env.server.Client().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set roles: got %d", resp.StatusCode)
	}

	// Audit row exists with before/after role sets.
	var action, before, after string
	err = env.pool.QueryRow(context.Background(), `
		SELECT action, before_json::text, after_json::text
		FROM audit_logs
		WHERE entity_type = 'user' AND entity_id = $1
		ORDER BY created_at DESC LIMIT 1`, targetID).
		Scan(&action, &before, &after)
	if err != nil {
		t.Fatalf("audit lookup: %v", err)
	}
	if action != "admin.users.roles.update" {
		t.Fatalf("unexpected audit action: %s", action)
	}
	if !bytes.Contains([]byte(after), []byte("guide_applicant")) {
		t.Fatalf("audit after_json missing new role: %s", after)
	}
	if !bytes.Contains([]byte(before), []byte("tourist")) {
		t.Fatalf("audit before_json missing old role: %s", before)
	}

	// A super admin cannot remove their own super_admin role and lock
	// themselves out of access recovery.
	adminID := env.grantRole(adminEmail, "super_admin")
	selfReq, err := http.NewRequest(http.MethodPatch, env.server.URL+"/api/v1/admin/users/"+adminID+"/roles", bytes.NewReader([]byte(`{"roles":["administrator"]}`)))
	if err != nil {
		t.Fatalf("self-lockout request: %v", err)
	}
	selfReq.Header.Set("Content-Type", "application/json")
	selfReq.Header.Set("Authorization", "Bearer "+adminAccess)
	selfResp, err := env.server.Client().Do(selfReq)
	if err != nil {
		t.Fatalf("self-lockout request: %v", err)
	}
	selfResp.Body.Close() //nolint:errcheck
	if selfResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("self-lockout: got %d, want 400", selfResp.StatusCode)
	}
}

func TestAdminInvitationIsAuditedSingleUseAndAssignsRole(t *testing.T) {
	env := newIntegrationEnv(t)
	adminEmail := uniqueEmail(t)
	env.registerAndLogin(adminEmail)
	env.grantRole(adminEmail, "super_admin")
	adminAccess, _ := env.login(adminEmail)["access_token"].(string)

	inviteeEmail := uniqueEmail(t)
	status, created := env.post("/api/v1/admin/invitations", map[string]any{
		"email": inviteeEmail, "roles": []string{"operations_agent"},
	}, bearer(adminAccess))
	if status != http.StatusCreated {
		t.Fatalf("create invitation: got %d: %v", status, created)
	}
	token, _ := created["accept_token"].(string)
	if token == "" {
		t.Fatalf("missing one-time token: %v", created)
	}

	status, accepted := env.post("/api/v1/auth/invitations/accept", map[string]any{
		"token": token, "password": "new-admin-passw0rd",
	}, nil)
	if status != http.StatusCreated {
		t.Fatalf("accept invitation: got %d: %v", status, accepted)
	}
	status, _ = env.post("/api/v1/auth/invitations/accept", map[string]any{
		"token": token, "password": "new-admin-passw0rd",
	}, nil)
	if status != http.StatusGone {
		t.Fatalf("replayed invitation: got %d, want 410", status)
	}

	status, login := env.post("/api/v1/auth/login", map[string]any{"email": inviteeEmail, "password": "new-admin-passw0rd"}, nil)
	if status != http.StatusOK {
		t.Fatalf("invited login: got %d: %v", status, login)
	}
	var roleCount, auditCount int
	if err := env.pool.QueryRow(context.Background(), `SELECT count(*) FROM user_roles ur JOIN users u ON u.id=ur.user_id JOIN roles r ON r.id=ur.role_id WHERE u.email=$1 AND r.code='operations_agent'`, inviteeEmail).Scan(&roleCount); err != nil || roleCount != 1 {
		t.Fatalf("invited role count=%d err=%v", roleCount, err)
	}
	if err := env.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE action='admin.invitations.create' AND after_json->>'email'=$1`, inviteeEmail).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("invitation audit count=%d err=%v", auditCount, err)
	}
}

func TestOTPAttemptLimiting(t *testing.T) {
	env := newIntegrationEnv(t)

	svc := auth.NewService(auth.NewRepository(env.pool),
		nil /* issuer unused for OTP */, rbac.NewStore(env.pool, env.rdb),
		audit.NewRecorder(env.pool), env.rdb, "local", "integration-test-secret")

	ctx := context.Background()
	destination := fmt.Sprintf("it-%d@example.com", time.Now().UnixNano())
	code, err := svc.RequestOTP(ctx, nil, destination, "email", "login")
	if err != nil {
		t.Fatalf("request otp: %v", err)
	}
	if code == "" {
		t.Fatal("expected dev code in local env")
	}

	// Four wrong attempts are rejected as invalid; the fifth locks the code.
	for i := 1; i <= 4; i++ {
		if err := svc.VerifyOTP(ctx, destination, "login", "000000"); err == nil {
			t.Fatalf("attempt %d: expected error", i)
		}
	}
	if err := svc.VerifyOTP(ctx, destination, "login", "000000"); err == nil {
		t.Fatal("attempt 5: expected error")
	}

	// Now even the correct code is refused (attempt cap reached).
	if err := svc.VerifyOTP(ctx, destination, "login", code); err == nil {
		t.Fatal("expected correct code to be refused after attempt cap")
	}
}

// TestIdentityJourneys exercises the remaining Phase 1 flows end to end:
// OTP over HTTP, password reset, guide apply/documents with a signed
// upload/download roundtrip, and TOTP MFA enrollment.
func TestIdentityJourneys(t *testing.T) {
	env := newIntegrationEnv(t)
	email := uniqueEmail(t)
	tokens := env.registerAndLogin(email)
	access, _ := tokens["access_token"].(string)

	// --- OTP request/verify over HTTP (local env returns dev_code) ---
	status, body := env.post("/api/v1/auth/otp/request", map[string]any{
		"destination": email, "channel": "email", "purpose": "verify_contact",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("otp request: got %d: %v", status, body)
	}
	devCode, _ := body["dev_code"].(string)
	if devCode == "" {
		t.Fatalf("expected dev_code in local env: %v", body)
	}
	status, body = env.post("/api/v1/auth/otp/verify", map[string]any{
		"destination": email, "purpose": "verify_contact", "code": devCode,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("otp verify: got %d: %v", status, body)
	}

	// --- Password forgot/reset revokes sessions ---
	status, body = env.post("/api/v1/auth/password/forgot", map[string]any{"email": email}, nil)
	if status != http.StatusOK {
		t.Fatalf("forgot: got %d: %v", status, body)
	}
	resetCode, _ := body["dev_code"].(string)
	if resetCode == "" {
		t.Fatalf("expected reset dev_code: %v", body)
	}
	status, body = env.post("/api/v1/auth/password/reset", map[string]any{
		"email": email, "code": resetCode, "password": "newpassw0rd-newpassw0rd",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("reset: got %d: %v", status, body)
	}
	// Old refresh token must be dead.
	refresh, _ := tokens["refresh_token"].(string)
	status, _ = env.post("/api/v1/auth/refresh", nil, refreshCookie(refresh))
	if status != http.StatusUnauthorized {
		t.Fatalf("post-reset refresh: got %d, want 401", status)
	}
	// New password logs in.
	status, body = env.post("/api/v1/auth/login", map[string]any{
		"email": email, "password": "newpassw0rd-newpassw0rd",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("login after reset: got %d: %v", status, body)
	}
	access, _ = body["access_token"].(string)

	// --- Guide apply is idempotent; documents get signed upload URLs ---
	apply := map[string]any{"public_name": "Kofi Integration"}
	status, body = env.post("/api/v1/guides/apply", apply, bearer(access))
	if status != http.StatusCreated {
		t.Fatalf("apply: got %d: %v", status, body)
	}
	status, body2 := env.post("/api/v1/guides/apply", apply, bearer(access))
	if status != http.StatusCreated {
		t.Fatalf("apply repeat: got %d: %v", status, body2)
	}

	status, body = env.post("/api/v1/guides/documents", map[string]any{
		"type": "national_id", "content_type": "image/png",
	}, bearer(access))
	if status != http.StatusCreated {
		t.Fatalf("register document: got %d: %v", status, body)
	}
	uploadURL, _ := body["upload_url"].(string)
	doc, _ := body["document"].(map[string]any)
	objectKey, _ := doc["object_key"].(string)
	if uploadURL == "" || objectKey == "" {
		t.Fatalf("missing upload_url/object_key: %v", body)
	}

	// Signed upload roundtrip: PUT bytes, then presign a GET and read back.
	putReq, err := http.NewRequest(http.MethodPut, env.server.URL+uploadURL,
		bytes.NewReader([]byte("fake-png-bytes")))
	if err != nil {
		t.Fatalf("put request: %v", err)
	}
	putResp, err := env.server.Client().Do(putReq)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	putResp.Body.Close() //nolint:errcheck
	if putResp.StatusCode != http.StatusCreated {
		t.Fatalf("upload: got %d", putResp.StatusCode)
	}

	localStore, err := storage2(env)
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	getURL, err := localStore.PresignGet(context.Background(), objectKey)
	if err != nil {
		t.Fatalf("presign get: %v", err)
	}
	getResp, err := env.server.Client().Get(env.server.URL + getURL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	got := make([]byte, 64)
	n, _ := getResp.Body.Read(got)
	getResp.Body.Close() //nolint:errcheck
	if getResp.StatusCode != http.StatusOK || string(got[:n]) != "fake-png-bytes" {
		t.Fatalf("download: got %d %q", getResp.StatusCode, got[:n])
	}

	// Unsigned access is refused (documents never public — stop condition 8).
	unsigned, err := env.server.Client().Get(env.server.URL + "/api/v1/files/" + objectKey)
	if err != nil {
		t.Fatalf("unsigned get: %v", err)
	}
	unsigned.Body.Close() //nolint:errcheck
	if unsigned.StatusCode != http.StatusForbidden {
		t.Fatalf("unsigned download: got %d, want 403", unsigned.StatusCode)
	}

	// --- MFA enroll + verify + step-up login ---
	status, body = env.post("/api/v1/me/mfa/enroll", nil, bearer(access))
	if status != http.StatusOK {
		t.Fatalf("mfa enroll: got %d: %v", status, body)
	}
	secret, _ := body["secret"].(string)
	if secret == "" {
		t.Fatalf("missing mfa secret: %v", body)
	}
	totp, err := pauth.TOTPCode(secret, time.Now())
	if err != nil {
		t.Fatalf("totp: %v", err)
	}
	status, body = env.post("/api/v1/me/mfa/verify", map[string]any{"code": totp}, bearer(access))
	if status != http.StatusOK {
		t.Fatalf("mfa verify: got %d: %v", status, body)
	}
	backupCodes, _ := body["backup_codes"].([]any)
	if len(backupCodes) != 8 {
		t.Fatalf("expected 8 backup codes: %v", body)
	}

	// Next login demands the step-up challenge.
	status, body = env.post("/api/v1/auth/login", map[string]any{
		"email": email, "password": "newpassw0rd-newpassw0rd",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("mfa login: got %d: %v", status, body)
	}
	challenge, _ := body["challenge"].(string)
	if ok, _ := body["mfa_required"].(bool); !ok || challenge == "" {
		t.Fatalf("expected mfa challenge: %v", body)
	}
	totp, err = pauth.TOTPCode(secret, time.Now())
	if err != nil {
		t.Fatalf("totp: %v", err)
	}
	status, body = env.post("/api/v1/auth/login/mfa", map[string]any{
		"challenge": challenge, "code": totp,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("mfa step-up: got %d: %v", status, body)
	}
	if tok, _ := body["access_token"].(string); tok == "" {
		t.Fatalf("step-up did not issue a session: %v", body)
	}

	// Audit row for the MFA enable exists.
	var count int
	if err := env.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_logs WHERE action = 'me.mfa.enable'`).Scan(&count); err != nil {
		t.Fatalf("mfa audit lookup: %v", err)
	}
	if count == 0 {
		t.Fatal("expected an audit row for me.mfa.enable")
	}
}

// storage2 rebuilds the local storage adapter against the test uploads dir.
func storage2(env *integrationEnv) (*storage.Local, error) {
	return storage.NewLocal(os.Getenv("STORAGE_LOCAL_DIR"), os.Getenv("JWT_OR_SESSION_SECRET"))
}
