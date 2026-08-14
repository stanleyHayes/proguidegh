package main

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// del issues a DELETE with headers (the shared helpers only cover GET/POST).
func (e *integrationEnv) del(path string, headers map[string]string) (int, map[string]any) {
	e.t.Helper()
	req, err := http.NewRequest(http.MethodDelete, e.server.URL+path, nil)
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

// TestLegalPoliciesArePublic — both stores require the privacy policy to be
// reachable without an account, and sign-up must show it before one exists.
func TestLegalPoliciesArePublic(t *testing.T) {
	env := newIntegrationEnv(t)

	status, body := env.get("/api/v1/legal/policies", nil)
	if status != http.StatusOK {
		t.Fatalf("policies without auth = %d, want 200", status)
	}
	docs, _ := body["policies"].([]any)
	if len(docs) == 0 {
		t.Fatal("no legal documents published; the apps have nothing to link to")
	}
	seen := map[string]bool{}
	for _, d := range docs {
		if rec, ok := d.(map[string]any); ok {
			seen[rec["document"].(string)] = true
		}
	}
	for _, want := range []string{"terms", "privacy", "location"} {
		if !seen[want] {
			t.Errorf("missing %q policy document", want)
		}
	}
}

// TestConsentIsRecordedAndExported — Act 843 s.20 requires consent to be
// demonstrable, which means it has to be retrievable, not just written.
func TestConsentIsRecordedAndExported(t *testing.T) {
	env := newIntegrationEnv(t)
	tokens := env.registerAndLogin(uniqueEmail(t))
	access := tokens["access_token"].(string)

	status, _ := env.post("/api/v1/me/consent",
		map[string]any{"document": "privacy", "version": "2026-08-13"}, bearer(access))
	if status != http.StatusCreated {
		t.Fatalf("record consent = %d, want 201", status)
	}

	// An unknown document must be rejected rather than silently stored.
	status, _ = env.post("/api/v1/me/consent",
		map[string]any{"document": "marketing", "version": "1"}, bearer(access))
	if status != http.StatusBadRequest {
		t.Fatalf("consent for unknown document = %d, want 400", status)
	}

	status, body := env.get("/api/v1/me/export", bearer(access))
	if status != http.StatusOK {
		t.Fatalf("export = %d, want 200", status)
	}
	consents, _ := body["consents"].([]any)
	if len(consents) != 1 {
		t.Fatalf("export carried %d consents, want 1", len(consents))
	}
}

// TestExportReturnsOwnDataOnly — subject access must return the caller's data
// and nothing about anyone else.
func TestExportReturnsOwnDataOnly(t *testing.T) {
	env := newIntegrationEnv(t)
	email := uniqueEmail(t)
	tokens := env.registerAndLogin(email)
	access := tokens["access_token"].(string)

	status, body := env.get("/api/v1/me/export", bearer(access))
	if status != http.StatusOK {
		t.Fatalf("export = %d, want 200", status)
	}
	account, ok := body["account"].(map[string]any)
	if !ok {
		t.Fatal("export has no account section")
	}
	if account["email"] != email {
		t.Errorf("export email = %v, want %v", account["email"], email)
	}
	if _, ok := body["bookings"].([]any); !ok {
		t.Error("export must always carry a bookings array, even when empty")
	}
	if _, ok := body["notes"].([]any); !ok {
		t.Error("export must state what is retained and why")
	}

	// Unauthenticated access to someone's personal data must be impossible.
	if status, _ := env.get("/api/v1/me/export", nil); status != http.StatusUnauthorized {
		t.Errorf("export without auth = %d, want 401", status)
	}
}

// TestAccountDeletionAnonymizesAndRevokes is the Apple 5.1.1(v) / Play
// requirement, plus the invariant that matters most: erasure must not destroy
// append-only financial history.
func TestAccountDeletionAnonymizesAndRevokes(t *testing.T) {
	env := newIntegrationEnv(t)
	email := uniqueEmail(t)
	tokens := env.registerAndLogin(email)
	access := tokens["access_token"].(string)

	ctx := context.Background()
	var userID string
	if err := env.pool.QueryRow(ctx,
		`SELECT id FROM users WHERE email = $1`, email).Scan(&userID); err != nil {
		t.Fatalf("look up user: %v", err)
	}

	// Preview must say it can proceed and disclose what is kept.
	status, preview := env.get("/api/v1/me/deletion", bearer(access))
	if status != http.StatusOK {
		t.Fatalf("deletion preview = %d, want 200", status)
	}
	if preview["can_delete"] != true {
		t.Fatalf("fresh account cannot be deleted: %v", preview["blockers"])
	}
	if retained, _ := preview["retained"].([]any); len(retained) == 0 {
		t.Error("preview must disclose what is retained after deletion")
	}

	status, body := env.del("/api/v1/me", bearer(access))
	if status != http.StatusOK {
		t.Fatalf("delete account = %d, want 200 (body %v)", status, body)
	}

	// Identity is gone.
	var storedEmail, storedStatus, passwordHash string
	var phone *string
	var anonymizedAt *string
	if err := env.pool.QueryRow(ctx, `
		SELECT email, status, password_hash, phone_e164, anonymized_at::text
		FROM users WHERE id = $1`, userID).
		Scan(&storedEmail, &storedStatus, &passwordHash, &phone, &anonymizedAt); err != nil {
		t.Fatalf("re-read user: %v", err)
	}
	if storedEmail == email {
		t.Error("email still present after deletion")
	}
	if storedStatus != "deleted" {
		t.Errorf("status = %q, want deleted", storedStatus)
	}
	if phone != nil {
		t.Error("phone still present after deletion")
	}
	if anonymizedAt == nil {
		t.Error("anonymized_at not stamped")
	}

	// Profile PII is gone.
	var profiles int
	if err := env.pool.QueryRow(ctx,
		`SELECT count(*) FROM tourist_profiles WHERE user_id = $1`, userID).Scan(&profiles); err != nil {
		t.Fatalf("count profiles: %v", err)
	}
	if profiles != 0 {
		t.Error("tourist profile survived deletion")
	}

	// Every session is revoked — the token must stop working immediately.
	if status, _ := env.get("/api/v1/me/export", bearer(access)); status == http.StatusOK {
		t.Error("access token still works after account deletion")
	}
	var sessions int
	if err := env.pool.QueryRow(ctx,
		`SELECT count(*) FROM refresh_sessions WHERE user_id = $1`, userID).Scan(&sessions); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if sessions != 0 {
		t.Errorf("%d refresh sessions survived deletion", sessions)
	}

	// The erasure itself is audited, and the receipt row exists.
	var audits int
	if err := env.pool.QueryRow(ctx, `
		SELECT count(*) FROM audit_logs
		WHERE actor_id = $1 AND action = 'privacy.account.delete'`, userID).Scan(&audits); err != nil {
		t.Fatalf("count audit rows: %v", err)
	}
	if audits != 1 {
		t.Errorf("audit rows for deletion = %d, want 1", audits)
	}
	var deletions int
	if err := env.pool.QueryRow(ctx, `
		SELECT count(*) FROM account_deletions
		WHERE user_id = $1 AND completed_at IS NOT NULL`, userID).Scan(&deletions); err != nil {
		t.Fatalf("count deletion records: %v", err)
	}
	if deletions != 1 {
		t.Errorf("account_deletions rows = %d, want 1", deletions)
	}

	// The users row itself must survive: bookings, ledger entries, receipts
	// and audit rows all reference it, and spec §8 makes those append-only.
	var users int
	if err := env.pool.QueryRow(ctx,
		`SELECT count(*) FROM users WHERE id = $1`, userID).Scan(&users); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if users != 1 {
		t.Error("user row was hard-deleted; append-only references would break")
	}
}

// TestDeletionBlockedByActiveBooking — a refusal is acceptable to both stores
// only if the user is told the specific, temporary reason.
func TestDeletionBlockedByActiveBooking(t *testing.T) {
	env := newIntegrationEnv(t)
	email := uniqueEmail(t)
	tokens := env.registerAndLogin(email)
	access := tokens["access_token"].(string)

	ctx := context.Background()
	var userID string
	if err := env.pool.QueryRow(ctx,
		`SELECT id FROM users WHERE email = $1`, email).Scan(&userID); err != nil {
		t.Fatalf("look up user: %v", err)
	}

	// Insert an in-flight booking directly: this test is about the deletion
	// guard, not about the booking flow, and any package/pricing setup would
	// only add ways for it to fail for unrelated reasons.
	var packageID string
	if err := env.pool.QueryRow(ctx,
		`SELECT id FROM tour_packages LIMIT 1`).Scan(&packageID); err != nil {
		t.Skipf("no tour packages seeded: %v", err)
	}
	// Unique reference: the table has a UNIQUE constraint and the test DB
	// persists between runs.
	if _, err := env.pool.Exec(ctx, `
		INSERT INTO bookings (reference, tourist_id, package_id, starts_at, ends_at, status)
		VALUES ('PGH-T' || substr(replace($1, '-', ''), 1, 8), $1::uuid, $2::uuid,
		        now() + interval '1 day', now() + interval '1 day 2 hours', 'CONFIRMED')`,
		userID, packageID); err != nil {
		t.Fatalf("insert booking: %v", err)
	}

	status, preview := env.get("/api/v1/me/deletion", bearer(access))
	if status != http.StatusOK {
		t.Fatalf("deletion preview = %d, want 200", status)
	}
	if preview["can_delete"] != false {
		t.Error("preview allows deletion despite an active booking")
	}

	status, body := env.del("/api/v1/me", bearer(access))
	if status != http.StatusConflict {
		t.Fatalf("delete with active booking = %d, want 409", status)
	}
	errObj, _ := body["error"].(map[string]any)
	if errObj == nil || errObj["code"] != "DELETION_BLOCKED" {
		t.Errorf("error code = %v, want DELETION_BLOCKED", errObj)
	}
	if msg, _ := errObj["message"].(string); msg == "" {
		t.Error("blocked deletion must explain the specific reason to the user")
	}

	// The refusal is on the record.
	var blocked int
	if err := env.pool.QueryRow(ctx, `
		SELECT count(*) FROM account_deletions
		WHERE user_id = $1 AND blocked_reason = 'active_booking'`, userID).Scan(&blocked); err != nil {
		t.Fatalf("count blocked: %v", err)
	}
	if blocked != 1 {
		t.Errorf("blocked deletion records = %d, want 1", blocked)
	}

	// The account must still be intact after a refused deletion.
	var st string
	if err := env.pool.QueryRow(ctx,
		`SELECT status FROM users WHERE id = $1`, userID).Scan(&st); err != nil {
		t.Fatalf("re-read user: %v", err)
	}
	if st != "active" {
		t.Errorf("status = %q after blocked deletion, want active", st)
	}
}
