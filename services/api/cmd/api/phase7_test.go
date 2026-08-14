package main

// Phase 7 integration tests: guide wallet & statement, payout-account
// tokenization, the weekly batch (with idempotent re-run), the §8.4
// transition machine with ledger posting on PAID, the finance CSV export
// and the tourism-levy report (P7-01…P7-07).

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	pauth "proguidegh/api/internal/platform/auth"
)

// seedEarnings posts the two balanced ledger transactions a completed,
// paid booking produces (spec §9.2): the payment split (clearing → guide
// payable pending + tourism levy) and the completion move (pending →
// eligible). Amounts are major units; the payment provider and tour flow
// that normally write these are covered by the Phase 4/5 suites.
func (e *integrationEnv) seedEarnings(t *testing.T, bookingID string, payable, levy float64) {
	t.Helper()
	ctx := context.Background()
	accounts := map[string]string{}
	for _, code := range []string{"tourist_clearing", "guide_payable_pending", "guide_payable_eligible", "tourism_levy_payable"} {
		var id string
		if err := e.pool.QueryRow(ctx,
			`SELECT id FROM ledger_accounts WHERE owner_type = 'platform' AND owner_id IS NULL AND code = $1`,
			code).Scan(&id); err != nil {
			t.Fatalf("ledger account %s: %v", code, err)
		}
		accounts[code] = id
	}

	post := func(reference, txnType string, entries [][3]any) {
		var txnID string
		if err := e.pool.QueryRow(ctx,
			`INSERT INTO ledger_transactions (reference, type, booking_id) VALUES ($1, $2, $3) RETURNING id`,
			reference, txnType, bookingID).Scan(&txnID); err != nil {
			t.Fatalf("insert ledger txn: %v", err)
		}
		for _, en := range entries {
			if _, err := e.pool.Exec(ctx,
				`INSERT INTO ledger_entries (transaction_id, account_id, direction, amount) VALUES ($1, $2, $3, $4)`,
				txnID, en[0], en[1], en[2]); err != nil {
				t.Fatalf("insert ledger entry: %v", err)
			}
		}
	}

	post("PAYMENT:"+bookingID+":seed", "payment", [][3]any{
		{accounts["tourist_clearing"], "debit", payable + levy},
		{accounts["guide_payable_pending"], "credit", payable},
		{accounts["tourism_levy_payable"], "credit", levy},
	})
	post("ELIGIBLE:"+bookingID, "completion", [][3]any{
		{accounts["guide_payable_pending"], "debit", payable},
		{accounts["guide_payable_eligible"], "credit", payable},
	})
}

func (e *integrationEnv) getRaw(path string, headers map[string]string) (int, string) {
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
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

func (e *integrationEnv) put(path string, body any, headers map[string]string) (int, map[string]any) {
	e.t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		e.t.Fatalf("marshal: %v", err)
	}
	req, err := http.NewRequest(http.MethodPut, e.server.URL+path, bytes.NewReader(raw))
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

func minor(v any) int64 { return int64(v.(float64)) }

func TestWalletPayoutBatchAndTransitions(t *testing.T) {
	env := newIntegrationEnv(t)
	ctx := context.Background()
	today := time.Now().Format("2006-01-02")

	// --- Fixture: certified guide with two completed bookings -----------
	guideEmail := uniqueEmail(t)
	env.registerAndLogin(guideEmail)
	var guideID string
	if err := env.pool.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, guideEmail).Scan(&guideID); err != nil {
		t.Fatalf("guide lookup: %v", err)
	}
	if _, err := env.pool.Exec(ctx,
		`INSERT INTO guide_profiles (user_id, public_name, status) VALUES ($1, 'P7 Guide', 'certified')`, guideID); err != nil {
		t.Fatalf("guide profile: %v", err)
	}

	touristEmail := uniqueEmail(t)
	env.registerAndLogin(touristEmail)
	var touristID string
	if err := env.pool.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, touristEmail).Scan(&touristID); err != nil {
		t.Fatalf("tourist lookup: %v", err)
	}
	var packageID string
	if err := env.pool.QueryRow(ctx, `SELECT id FROM tour_packages WHERE active LIMIT 1`).Scan(&packageID); err != nil {
		t.Fatalf("package lookup: %v", err)
	}

	newBooking := func(endsInterval string) string {
		token, _ := pauth.NewOpaqueToken()
		var id string
		if err := env.pool.QueryRow(ctx,
			`INSERT INTO bookings (reference, tourist_id, guide_id, package_id, starts_at, ends_at, status)
			 VALUES ($1, $2, $3, $4, now() - interval '`+endsInterval+`' - interval '4 hours',
			         now() - interval '`+endsInterval+`', 'COMPLETED') RETURNING id`,
			"PGH-"+token[:8], touristID, guideID, packageID).Scan(&id); err != nil {
			t.Fatalf("insert booking: %v", err)
		}
		return id
	}
	// booking1 cleared the 7-day hold (GH₵200 gross → 180 guide / 20 levy);
	// booking2 completed yesterday (GH₵50 gross → 45 / 5) and is still held.
	b1 := newBooking("10 days")
	b2 := newBooking("1 day")
	// The test database is shared across the package, so the levy payable
	// account already carries earlier phases' entries — assert deltas.
	var levyStart float64
	if err := env.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(CASE WHEN e.direction = 'credit' THEN e.amount ELSE -e.amount END), 0)::float8
		 FROM ledger_entries e JOIN ledger_accounts a ON a.id = e.account_id
		 WHERE a.owner_type = 'platform' AND a.code = 'tourism_levy_payable'`).Scan(&levyStart); err != nil {
		t.Fatalf("levy start: %v", err)
	}
	env.seedEarnings(t, b1, 180.00, 20.00)
	env.seedEarnings(t, b2, 45.00, 5.00)

	guideAccess := env.login(guideEmail)["access_token"].(string)

	// --- P7-02: payout account tokenization -----------------------------
	status, body := env.put("/api/v1/me/guide/payout-account", map[string]any{
		"provider": "mtn_momo", "network": "MTN", "account_ref": "0244000111",
	}, bearer(guideAccess))
	if status != http.StatusOK {
		t.Fatalf("put payout account: got %d: %v", status, body)
	}
	account := body["account"].(map[string]any)
	if account["masked_ref"] != "****0111" {
		t.Fatalf("masked ref: %v", account)
	}
	rawJSON, _ := json.Marshal(body)
	if strings.Contains(string(rawJSON), "0244000111") {
		t.Fatal("plaintext account ref leaked in response")
	}
	accountID := account["id"].(string)

	status, body = env.get("/api/v1/me/guide/payout-account", bearer(guideAccess))
	if status != http.StatusOK || body["account"].(map[string]any)["verified_at"] != nil {
		t.Fatalf("get payout account: %d %v", status, body)
	}

	finEmail := uniqueEmail(t)
	env.registerAndLogin(finEmail)
	env.grantRole(finEmail, "finance_officer")
	finAccess := env.login(finEmail)["access_token"].(string)

	status, body = env.post("/api/v1/admin/payout-accounts/"+accountID+"/verify", map[string]any{}, bearer(finAccess))
	if status != http.StatusOK || body["account"].(map[string]any)["verified_at"] == nil {
		t.Fatalf("verify account: %d %v", status, body)
	}

	// Finance endpoints are permission-gated.
	touristAccess := env.login(touristEmail)["access_token"].(string)
	if status, _ := env.get("/api/v1/admin/payouts", bearer(touristAccess)); status != http.StatusForbidden {
		t.Fatalf("tourist admin payouts: got %d, want 403", status)
	}

	// --- P7-01: wallet ---------------------------------------------------
	status, body = env.get("/api/v1/me/guide/wallet", bearer(guideAccess))
	if status != http.StatusOK {
		t.Fatalf("wallet: got %d: %v", status, body)
	}
	wallet := body["wallet"].(map[string]any)
	if got := minor(wallet["eligible_minor"]); got != 22500 {
		t.Fatalf("eligible: got %d, want 22500", got)
	}
	if got := minor(wallet["payout_eligible_minor"]); got != 18000 {
		t.Fatalf("payout eligible: got %d, want 18000 (booking2 still held)", got)
	}
	if got := minor(wallet["in_flight_minor"]); got != 0 {
		t.Fatalf("in flight: got %d, want 0", got)
	}

	// --- P7-03 + P7-07: batch and idempotent re-run ----------------------
	status, body = env.post("/api/v1/admin/payouts/batch", map[string]any{}, bearer(finAccess))
	if status != http.StatusOK {
		t.Fatalf("batch: got %d: %v", status, body)
	}
	if minor(body["created"]) < 1 {
		t.Fatalf("batch created nothing: %v", body)
	}

	var payoutID, payoutStatus string
	var payoutAmount float64
	if err := env.pool.QueryRow(ctx,
		`SELECT id, status, amount::float8 FROM payouts WHERE guide_id = $1`, guideID).
		Scan(&payoutID, &payoutStatus, &payoutAmount); err != nil {
		t.Fatalf("payout row: %v", err)
	}
	if payoutStatus != "QUEUED" || payoutAmount != 180.00 {
		t.Fatalf("payout: %s %.2f, want QUEUED 180.00", payoutStatus, payoutAmount)
	}

	// Immediate re-run: per-(guide, date) uniqueness blocks duplicates.
	status, body = env.post("/api/v1/admin/payouts/batch", map[string]any{"scheduled_for": today}, bearer(finAccess))
	if status != http.StatusOK || minor(body["created"]) != 0 {
		t.Fatalf("batch re-run: %d %v", status, body)
	}
	var payoutCount int
	if err := env.pool.QueryRow(ctx,
		`SELECT COUNT(*)::int FROM payouts WHERE guide_id = $1`, guideID).Scan(&payoutCount); err != nil {
		t.Fatalf("payout count: %v", err)
	}
	if payoutCount != 1 {
		t.Fatalf("payout count after re-run: %d, want 1 (P7-07)", payoutCount)
	}

	// --- P7-04: finance CSV export (decrypts destination refs) -----------
	status, csv := env.getRaw("/api/v1/admin/payouts/export?scheduled_for="+today, bearer(finAccess))
	if status != http.StatusOK {
		t.Fatalf("export: got %d: %s", status, csv)
	}
	if !strings.Contains(csv, "0244000111") || !strings.Contains(csv, payoutID) || !strings.Contains(csv, "180.00") {
		t.Fatalf("export csv missing expected rows:\n%s", csv)
	}
	if s, _ := env.get("/api/v1/admin/payouts/export?scheduled_for="+today, bearer(touristAccess)); s != http.StatusForbidden {
		t.Fatalf("tourist export: got %d, want 403", s)
	}

	// --- P7-05: transition machine; PAID posts the ledger move -----------
	status, _ = env.post("/api/v1/admin/payouts/"+payoutID+"/transition",
		map[string]any{"to": "PAID"}, bearer(finAccess))
	if status != http.StatusConflict {
		t.Fatalf("QUEUED->PAID: got %d, want 409", status)
	}
	status, body = env.post("/api/v1/admin/payouts/"+payoutID+"/transition",
		map[string]any{"to": "PROCESSING"}, bearer(finAccess))
	if status != http.StatusOK {
		t.Fatalf("QUEUED->PROCESSING: got %d: %v", status, body)
	}
	status, body = env.post("/api/v1/admin/payouts/"+payoutID+"/transition",
		map[string]any{"to": "PAID", "provider_reference": "mtn-txn-001"}, bearer(finAccess))
	if status != http.StatusOK {
		t.Fatalf("PROCESSING->PAID: got %d: %v", status, body)
	}
	paid := body["payout"].(map[string]any)
	if paid["status"] != "PAID" || paid["ledger_transaction_id"] == nil {
		t.Fatalf("paid payout: %v", paid)
	}

	// The ledger posting exists, is balanced, and debits the eligible pool.
	var debitSum, creditSum float64
	if err := env.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount) FILTER (WHERE e.direction = 'debit'), 0)::float8,
		        COALESCE(SUM(amount) FILTER (WHERE e.direction = 'credit'), 0)::float8
		 FROM ledger_entries e JOIN ledger_transactions t ON t.id = e.transaction_id
		 WHERE t.reference = $1`, "PAYOUT:"+payoutID).Scan(&debitSum, &creditSum); err != nil {
		t.Fatalf("payout ledger: %v", err)
	}
	if debitSum != 180.00 || creditSum != 180.00 {
		t.Fatalf("payout ledger unbalanced: debit %.2f credit %.2f", debitSum, creditSum)
	}

	// Wallet reflects the payout: paid total up, eligible drawn down.
	_, body = env.get("/api/v1/me/guide/wallet", bearer(guideAccess))
	wallet = body["wallet"].(map[string]any)
	if got := minor(wallet["paid_total_minor"]); got != 18000 {
		t.Fatalf("paid total: got %d, want 18000", got)
	}
	if got := minor(wallet["eligible_minor"]); got != 4500 {
		t.Fatalf("eligible after payout: got %d, want 4500", got)
	}
	if got := minor(wallet["payout_eligible_minor"]); got != 0 {
		t.Fatalf("payout eligible after payout: got %d, want 0 (no double-pay)", got)
	}

	// --- FAILED flow: reason required, retry + manual review -------------
	var retryID string
	if err := env.pool.QueryRow(ctx,
		`INSERT INTO payouts (guide_id, amount, currency, status, scheduled_for)
		 VALUES ($1, 45.00, 'GHS', 'QUEUED', $2) RETURNING id`,
		guideID, time.Now().AddDate(0, 0, 1).Format("2006-01-02")).Scan(&retryID); err != nil {
		t.Fatalf("insert retry payout: %v", err)
	}
	transition := func(id string, req map[string]any) int {
		s, _ := env.post("/api/v1/admin/payouts/"+id+"/transition", req, bearer(finAccess))
		return s
	}
	if s := transition(retryID, map[string]any{"to": "PROCESSING"}); s != http.StatusOK {
		t.Fatalf("retry QUEUED->PROCESSING: %d", s)
	}
	if s := transition(retryID, map[string]any{"to": "FAILED"}); s != http.StatusBadRequest {
		t.Fatalf("FAILED without reason: got %d, want 400", s)
	}
	if s := transition(retryID, map[string]any{"to": "FAILED", "failure_reason": "network timeout"}); s != http.StatusOK {
		t.Fatalf("FAILED with reason: %d", s)
	}
	if s := transition(retryID, map[string]any{"to": "RETRY_QUEUED"}); s != http.StatusOK {
		t.Fatalf("FAILED->RETRY_QUEUED: %d", s)
	}
	if s := transition(retryID, map[string]any{"to": "MANUAL_REVIEW"}); s != http.StatusOK {
		t.Fatalf("RETRY_QUEUED->MANUAL_REVIEW: %d", s)
	}
	if s := transition(retryID, map[string]any{"to": "PAID"}); s != http.StatusConflict {
		t.Fatalf("MANUAL_REVIEW->PAID: got %d, want 409", s)
	}

	// --- P7-01: statement (ledger + payout lines, cursor pagination) -----
	status, body = env.get("/api/v1/me/guide/statement", bearer(guideAccess))
	if status != http.StatusOK {
		t.Fatalf("statement: got %d: %v", status, body)
	}
	entries := body["entries"].([]any)
	kinds := map[string]bool{}
	for _, en := range entries {
		kinds[en.(map[string]any)["kind"].(string)] = true
	}
	if !kinds["ledger"] || !kinds["payout"] {
		t.Fatalf("statement kinds: %v", kinds)
	}

	_, body = env.get("/api/v1/me/guide/statement?limit=1", bearer(guideAccess))
	page1 := body["entries"].([]any)
	cursor, _ := body["next_cursor"].(string)
	if len(page1) != 1 || cursor == "" {
		t.Fatalf("statement page1: %v", body)
	}
	_, body = env.get("/api/v1/me/guide/statement?limit=1&cursor="+cursor, bearer(guideAccess))
	page2 := body["entries"].([]any)
	if len(page2) == 0 || page2[0].(map[string]any)["id"] == page1[0].(map[string]any)["id"] {
		t.Fatalf("statement pagination did not advance: %v vs %v", page1, page2)
	}

	// --- P7-06: tourism-levy report --------------------------------------
	status, body = env.get("/api/v1/admin/reports/tourism-levy", bearer(finAccess))
	if status != http.StatusOK {
		t.Fatalf("levy report: got %d: %v", status, body)
	}
	report := body["report"].(map[string]any)
	if got := minor(report["balance_minor"]); got != int64(levyStart*100)+2500 {
		t.Fatalf("levy balance: got %d, want %d", got, int64(levyStart*100)+2500)
	}
	_, body = env.get("/api/v1/admin/reports/tourism-levy?from="+today, bearer(finAccess))
	if got := minor(body["report"].(map[string]any)["period_credits_minor"]); got < 2500 {
		t.Fatalf("levy period credits: got %d, want at least 2500", got)
	}
}

// TestPayoutBatchDefersToDelayHold proves the batch never pays out earnings
// whose bookings completed inside the payout-delay window (P7-02/P7-03).
func TestPayoutBatchDefersToDelayHold(t *testing.T) {
	env := newIntegrationEnv(t)
	ctx := context.Background()

	guideEmail := uniqueEmail(t)
	env.registerAndLogin(guideEmail)
	var guideID string
	if err := env.pool.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, guideEmail).Scan(&guideID); err != nil {
		t.Fatalf("guide lookup: %v", err)
	}
	if _, err := env.pool.Exec(ctx,
		`INSERT INTO guide_profiles (user_id, public_name, status) VALUES ($1, 'Hold Guide', 'certified')`, guideID); err != nil {
		t.Fatalf("guide profile: %v", err)
	}
	var packageID, touristID string
	if err := env.pool.QueryRow(ctx, `SELECT id FROM tour_packages WHERE active LIMIT 1`).Scan(&packageID); err != nil {
		t.Fatalf("package: %v", err)
	}
	if err := env.pool.QueryRow(ctx, `SELECT id FROM users WHERE email <> $1 LIMIT 1`, guideEmail).Scan(&touristID); err != nil {
		t.Fatalf("tourist: %v", err)
	}
	token, _ := pauth.NewOpaqueToken()
	var bookingID string
	if err := env.pool.QueryRow(ctx,
		`INSERT INTO bookings (reference, tourist_id, guide_id, package_id, starts_at, ends_at, status)
		 VALUES ($1, $2, $3, $4, now() - interval '5 hours', now() - interval '1 hour', 'COMPLETED') RETURNING id`,
		"PGH-"+token[:8], touristID, guideID, packageID).Scan(&bookingID); err != nil {
		t.Fatalf("booking: %v", err)
	}
	env.seedEarnings(t, bookingID, 100.00, 10.00)

	finEmail := uniqueEmail(t)
	env.registerAndLogin(finEmail)
	env.grantRole(finEmail, "finance_officer")
	finAccess := env.login(finEmail)["access_token"].(string)

	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	status, _ := env.post("/api/v1/admin/payouts/batch", map[string]any{"scheduled_for": tomorrow}, bearer(finAccess))
	if status != http.StatusOK {
		t.Fatalf("batch: got %d", status)
	}
	var n int
	if err := env.pool.QueryRow(ctx,
		`SELECT COUNT(*)::int FROM payouts WHERE guide_id = $1`, guideID).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("payout queued inside delay hold: %d rows", n)
	}

	guideAccess := env.login(guideEmail)["access_token"].(string)
	_, body := env.get("/api/v1/me/guide/wallet", bearer(guideAccess))
	wallet := body["wallet"].(map[string]any)
	if got := minor(wallet["eligible_minor"]); got != 10000 {
		t.Fatalf("eligible: got %d, want 10000", got)
	}
	if got := minor(wallet["payout_eligible_minor"]); got != 0 {
		t.Fatalf("payout eligible inside hold: got %d, want 0", got)
	}
}
