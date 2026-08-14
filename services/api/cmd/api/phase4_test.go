package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"proguidegh/api/internal/payments"
)

// postRaw issues a POST with a raw body and returns status + decoded JSON.
func (e *integrationEnv) postRaw(path string, body []byte, headers map[string]string) (int, map[string]any) {
	e.t.Helper()
	req, err := http.NewRequest(http.MethodPost, e.server.URL+path, bytes.NewReader(body))
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

// TestPaymentLedgerReceiptJourney is the Phase 4 exit-criteria journey
// (spec §30.4): quote → booking → payment-intent → signed webhook →
// CONFIRMED with exactly one balanced ledger allocation and a downloadable
// receipt; webhook replays are 200 no-ops; a bad signature is 401 with zero
// side effects; the refund reverses the ledger and refunds the booking.
func TestPaymentLedgerReceiptJourney(t *testing.T) {
	fx := newPhase3Fixture(t)
	env := fx.env
	ctx := context.Background()

	guideID, guideAccess := fx.makeActiveGuide(t, "Ama Payments")
	bookingStart := nextWeekdayAt(time.Tuesday)
	fx.setWeeklyWindow(t, guideAccess, int(bookingStart.Weekday()))

	touristEmail := uniqueEmail(t)
	touristTokens := env.registerAndLogin(touristEmail)
	touristAccess := touristTokens["access_token"].(string)

	// Quote the heritage tour (spec §9.1: GHS 450.00 at 15%/3%).
	status, body := env.post("/api/v1/bookings/quote", map[string]any{
		"package_id": fx.heritagePkgID, "starts_at": bookingStart.Format(time.RFC3339),
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("quote: got %d: %v", status, body)
	}

	withKey := func(key string) map[string]string {
		return map[string]string{"Authorization": "Bearer " + touristAccess, "Idempotency-Key": key}
	}
	status, body = env.post("/api/v1/bookings", map[string]any{
		"package_id": fx.heritagePkgID,
		"guide_id":   guideID,
		"starts_at":  bookingStart.Format(time.RFC3339),
	}, withKey("idem-p4-booking"))
	if status != http.StatusCreated {
		t.Fatalf("create booking: got %d: %v", status, body)
	}
	booking := body["booking"].(map[string]any)
	bookingID := booking["id"].(string)
	if booking["amount"] != "450.00" {
		t.Fatalf("booking amount = %v, want 450.00", booking["amount"])
	}

	// --- Payment initiation ---------------------------------------------------
	// Idempotency-Key is required (spec §14).
	status, body = env.post("/api/v1/bookings/"+bookingID+"/payment-intent", nil, bearer(touristAccess))
	if status != http.StatusBadRequest || errCode(body) != "IDEMPOTENCY_KEY_REQUIRED" {
		t.Fatalf("intent without key: got %d %v, want 400 IDEMPOTENCY_KEY_REQUIRED", status, body)
	}

	// A stranger cannot initiate payment on someone else's booking.
	strangerTokens := env.registerAndLogin(uniqueEmail(t))
	strangerAccess := strangerTokens["access_token"].(string)
	status, _ = env.post("/api/v1/bookings/"+bookingID+"/payment-intent", nil,
		map[string]string{"Authorization": "Bearer " + strangerAccess, "Idempotency-Key": "idem-p4-stranger"})
	if status != http.StatusNotFound {
		t.Fatalf("stranger intent: got %d, want 404 (existence must not leak)", status)
	}

	status, body = env.post("/api/v1/bookings/"+bookingID+"/payment-intent", nil, withKey("idem-p4-intent"))
	if status != http.StatusCreated {
		t.Fatalf("intent: got %d: %v", status, body)
	}
	payment := body["payment"].(map[string]any)
	paymentID := payment["id"].(string)
	providerRef := payment["provider_reference"].(string)
	authURL := payment["authorization_url"].(string)
	if payment["status"] != "PENDING" || payment["provider"] != "mock" ||
		payment["amount"] != "450.00" || authURL == "" {
		t.Fatalf("payment = %v, want PENDING mock 450.00 with authorization_url", payment)
	}

	// Idempotent replay returns the SAME payment, reference and URL (§14).
	status, body = env.post("/api/v1/bookings/"+bookingID+"/payment-intent", nil, withKey("idem-p4-intent"))
	if status != http.StatusOK {
		t.Fatalf("intent replay: got %d: %v", status, body)
	}
	replayed := body["payment"].(map[string]any)
	if replayed["id"] != paymentID || replayed["provider_reference"] != providerRef ||
		replayed["authorization_url"] != authURL {
		t.Fatalf("replay = %v, want same payment %s/%s", replayed, paymentID, providerRef)
	}

	// A second key would double-charge: refused while the first is active.
	status, body = env.post("/api/v1/bookings/"+bookingID+"/payment-intent", nil, withKey("idem-p4-intent-2"))
	if status != http.StatusConflict || errCode(body) != "PAYMENT_ALREADY_ACTIVE" {
		t.Fatalf("second intent: got %d %v, want 409 PAYMENT_ALREADY_ACTIVE", status, body)
	}

	// No receipt exists before payment lands.
	status, _ = env.get("/api/v1/bookings/"+bookingID+"/receipt", bearer(touristAccess))
	if status != http.StatusNotFound {
		t.Fatalf("receipt before payment: got %d, want 404", status)
	}

	// --- Bad signature: 401, zero side effects ---------------------------------
	mock := payments.NewMockProvider("dev-mock-webhook-secret") // config default
	goodBody, goodSig := mock.SignWebhookPayload(providerRef)

	status, _ = env.postRaw("/api/v1/webhooks/payments/mock", goodBody,
		map[string]string{payments.MockSignatureHeader: "deadbeef"})
	if status != http.StatusUnauthorized {
		t.Fatalf("bad signature: got %d, want 401", status)
	}
	var sideEffects int
	if err := env.pool.QueryRow(ctx,
		`SELECT count(*) FROM webhook_events WHERE event_reference = $1`, providerRef).Scan(&sideEffects); err != nil {
		t.Fatalf("webhook_events count: %v", err)
	}
	if sideEffects != 0 {
		t.Fatalf("bad signature left %d webhook_events rows, want 0", sideEffects)
	}

	// Unknown provider endpoint is 404 (does not reveal the live provider).
	status, _ = env.postRaw("/api/v1/webhooks/payments/hubtel", goodBody,
		map[string]string{payments.MockSignatureHeader: goodSig})
	if status != http.StatusNotFound {
		t.Fatalf("unknown provider: got %d, want 404", status)
	}

	// --- Signed success webhook: the full atomic side-effect set ---------------
	status, body = env.postRaw("/api/v1/webhooks/payments/mock", goodBody,
		map[string]string{payments.MockSignatureHeader: goodSig})
	if status != http.StatusOK || body["received"] != true || body["replay"] != false {
		t.Fatalf("webhook: got %d: %v", status, body)
	}

	// Booking CONFIRMED through the state machine.
	status, body = env.get("/api/v1/bookings/"+bookingID, bearer(touristAccess))
	if status != http.StatusOK {
		t.Fatalf("booking after webhook: got %d: %v", status, body)
	}
	if body["booking"].(map[string]any)["status"] != "CONFIRMED" {
		t.Fatalf("booking status = %v, want CONFIRMED", body["booking"])
	}

	// Payment SUCCEEDED with paid_at.
	status, body = env.get("/api/v1/payments/"+paymentID, bearer(touristAccess))
	if status != http.StatusOK {
		t.Fatalf("payment detail: got %d: %v", status, body)
	}
	gotP := body["payment"].(map[string]any)
	if gotP["status"] != "SUCCEEDED" || gotP["paid_at"] == nil {
		t.Fatalf("payment = %v, want SUCCEEDED with paid_at", gotP)
	}
	// Stranger cannot read the payment.
	status, _ = env.get("/api/v1/payments/"+paymentID, bearer(strangerAccess))
	if status != http.StatusNotFound {
		t.Fatalf("stranger payment detail: got %d, want 404", status)
	}

	assertOneBalancedAllocation(t, env, providerRef, bookingID)

	// Receipt issued and downloadable over a signed URL (spec §17).
	receiptNumber := assertReceipt(t, env, bookingID, touristAccess)

	// Notification stubs queued for the worker (spec §20/§21).
	var notifCount int
	if err := env.pool.QueryRow(ctx, `
		SELECT count(*) FROM notifications
		WHERE template IN ('booking.payment_confirmed', 'booking.new_confirmed')`).Scan(&notifCount); err != nil {
		t.Fatalf("notifications count: %v", err)
	}
	if notifCount < 2 {
		t.Fatalf("notifications = %d, want tourist + guide stubs", notifCount)
	}

	// --- Replay the same webhook 3x: 200s, zero additional side effects --------
	for i := 0; i < 3; i++ {
		status, body = env.postRaw("/api/v1/webhooks/payments/mock", goodBody,
			map[string]string{payments.MockSignatureHeader: goodSig})
		if status != http.StatusOK || body["replay"] != true {
			t.Fatalf("replay %d: got %d: %v, want 200 replay=true", i, status, body)
		}
	}
	assertExactlyOne(t, env, providerRef, bookingID)

	// --- Duplicate provider_reference insert is rejected (§9.2) -----------------
	err := env.pool.QueryRow(ctx, `
		INSERT INTO payments (booking_id, provider, provider_reference, amount)
		VALUES ($1, 'mock', $2, 1.00) RETURNING id`, bookingID, providerRef).Scan(new(string))
	if err == nil {
		t.Fatal("duplicate provider_reference insert must violate the UNIQUE constraint")
	}

	// Initiating again after confirmation is refused (booking moved on).
	status, body = env.post("/api/v1/bookings/"+bookingID+"/payment-intent", nil, withKey("idem-p4-intent-3"))
	if status != http.StatusConflict || errCode(body) != "NOT_PAYABLE" {
		t.Fatalf("intent after confirm: got %d %v, want 409 NOT_PAYABLE", status, body)
	}

	// --- Refund: reversing entries, originals intact (§9.2) ---------------------
	// Tourists have no payments.refund permission.
	status, _ = env.post("/api/v1/payments/"+paymentID+"/refund",
		map[string]any{"reason": "tourist changed plans"}, withKey("idem-p4-refund"))
	if status != http.StatusForbidden {
		t.Fatalf("tourist refund: got %d, want 403", status)
	}

	financeEmail := uniqueEmail(t)
	env.registerAndLogin(financeEmail)
	env.grantRole(financeEmail, "finance_officer")
	financeTokens := env.login(financeEmail)
	financeAccess := financeTokens["access_token"].(string)
	financeKey := map[string]string{"Authorization": "Bearer " + financeAccess, "Idempotency-Key": "idem-p4-refund"}

	status, body = env.post("/api/v1/payments/"+paymentID+"/refund",
		map[string]any{"reason": "tourist changed plans"}, financeKey)
	if status != http.StatusOK {
		t.Fatalf("refund: got %d: %v", status, body)
	}
	if body["payment"].(map[string]any)["status"] != "REFUNDED" {
		t.Fatalf("refund payment = %v, want REFUNDED", body["payment"])
	}
	reversalRef := body["refund"].(map[string]any)["reversal_reference"].(string)
	if reversalRef != "REV:"+providerRef {
		t.Fatalf("reversal_reference = %v, want REV:%s", reversalRef, providerRef)
	}

	// Booking driven to REFUNDED strictly through the state machine.
	status, body = env.get("/api/v1/bookings/"+bookingID, bearer(touristAccess))
	if status != http.StatusOK || body["booking"].(map[string]any)["status"] != "REFUNDED" {
		t.Fatalf("booking after refund = %v, want REFUNDED", body["booking"])
	}
	// Every state machine hop was recorded (…CONFIRMED → CANCELLED_BY_ADMIN →
	// REFUND_PENDING → REFUNDED).
	var refundHops int
	if err := env.pool.QueryRow(ctx, `
		SELECT count(*) FROM booking_status_events
		WHERE booking_id = $1 AND to_status IN ('CANCELLED_BY_ADMIN','REFUND_PENDING','REFUNDED')`,
		bookingID).Scan(&refundHops); err != nil {
		t.Fatalf("refund hops: %v", err)
	}
	if refundHops != 3 {
		t.Fatalf("refund status events = %d, want 3", refundHops)
	}

	// The reversal transaction nets every account back to zero; originals stay.
	assertReversalNetsToZero(t, env, providerRef)

	// Audit row for the privileged financial action (spec §1.2).
	var auditCount int
	if err := env.pool.QueryRow(ctx, `
		SELECT count(*) FROM audit_logs
		WHERE action = 'payments.refund' AND entity_id = $1`, paymentID).Scan(&auditCount); err != nil {
		t.Fatalf("audit count: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("payments.refund audit rows = %d, want 1", auditCount)
	}

	// Refund replay: same key, no second refund row, no second reversal.
	status, body = env.post("/api/v1/payments/"+paymentID+"/refund",
		map[string]any{"reason": "tourist changed plans"}, financeKey)
	if status != http.StatusOK {
		t.Fatalf("refund replay: got %d: %v", status, body)
	}
	var refundRows, reversalRows int
	if err := env.pool.QueryRow(ctx,
		`SELECT count(*) FROM refunds WHERE payment_id = $1`, paymentID).Scan(&refundRows); err != nil {
		t.Fatalf("refund rows: %v", err)
	}
	if err := env.pool.QueryRow(ctx,
		`SELECT count(*) FROM ledger_transactions WHERE reference = $1`, reversalRef).Scan(&reversalRows); err != nil {
		t.Fatalf("reversal rows: %v", err)
	}
	if refundRows != 1 || reversalRows != 1 {
		t.Fatalf("refund replay produced refunds=%d reversals=%d, want 1/1", refundRows, reversalRows)
	}

	// Receipt survives the refund (immutable issued document, §17).
	status, body = env.get("/api/v1/bookings/"+bookingID+"/receipt", bearer(touristAccess))
	if status != http.StatusOK ||
		body["receipt"].(map[string]any)["receipt_number"] != receiptNumber {
		t.Fatalf("receipt after refund: got %d: %v", status, body)
	}
}

// assertOneBalancedAllocation verifies exactly one ledger transaction for the
// payment, carrying the spec §9.1 GHS 450 allocation, balanced to the pesewa.
func assertOneBalancedAllocation(t *testing.T, env *integrationEnv, providerRef, bookingID string) {
	t.Helper()
	ctx := context.Background()

	var txnID, txnBooking string
	err := env.pool.QueryRow(ctx, `
		SELECT id, booking_id FROM ledger_transactions WHERE reference = $1`,
		"PAY:"+providerRef).Scan(&txnID, &txnBooking)
	if err != nil {
		t.Fatalf("ledger allocation missing for %s: %v", providerRef, err)
	}
	if txnBooking != bookingID {
		t.Fatalf("ledger booking_id = %s, want %s", txnBooking, bookingID)
	}

	rows, err := env.pool.Query(ctx, `
		SELECT a.code, e.direction, ROUND(e.amount * 100)::bigint
		FROM ledger_entries e JOIN ledger_accounts a ON a.id = e.account_id
		WHERE e.transaction_id = $1`, txnID)
	if err != nil {
		t.Fatalf("ledger entries: %v", err)
	}
	defer rows.Close()

	// Expected §9.1 allocation: debit tourist_clearing 450.00; credits
	// platform_revenue 67.50, tourism_levy_payable 13.50,
	// guide_payable_pending 369.00.
	want := map[string]int64{
		"tourist_clearing/debit":       45000,
		"platform_revenue/credit":      6750,
		"tourism_levy_payable/credit":  1350,
		"guide_payable_pending/credit": 36900,
	}
	got := map[string]int64{}
	var debits, credits int64
	for rows.Next() {
		var code, dir string
		var pesewas int64
		if err := rows.Scan(&code, &dir, &pesewas); err != nil {
			t.Fatalf("scan entry: %v", err)
		}
		got[code+"/"+dir] = pesewas
		if dir == "debit" {
			debits += pesewas
		} else {
			credits += pesewas
		}
	}
	if len(got) != 4 {
		t.Fatalf("allocation entries = %v, want 4 legs", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("allocation leg %s = %d, want %d (all: %v)", k, got[k], v, got)
		}
	}
	if debits != credits || debits != 45000 {
		t.Fatalf("allocation unbalanced: debits %d credits %d", debits, credits)
	}
}

// assertReceipt fetches the receipt metadata, downloads the PDF over the
// signed URL and returns the receipt number.
func assertReceipt(t *testing.T, env *integrationEnv, bookingID, access string) string {
	t.Helper()
	status, body := env.get("/api/v1/bookings/"+bookingID+"/receipt", bearer(access))
	if status != http.StatusOK {
		t.Fatalf("receipt: got %d: %v", status, body)
	}
	receipt := body["receipt"].(map[string]any)
	number := receipt["receipt_number"].(string)
	if !strings.HasPrefix(number, "PGH-") {
		t.Fatalf("receipt number = %q, want PGH-*", number)
	}
	downloadURL, _ := body["download_url"].(string)
	if downloadURL == "" {
		t.Fatalf("missing download_url: %v", body)
	}
	resp, err := env.server.Client().Get(env.server.URL + downloadURL)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("download status = %d", resp.StatusCode)
	}
	head := make([]byte, 4)
	if _, err := io.ReadFull(resp.Body, head); err != nil {
		t.Fatalf("download read: %v", err)
	}
	if string(head) != "%PDF" {
		t.Fatalf("download starts with %q, want %%PDF", head)
	}
	return number
}

// assertExactlyOne proves the replayed webhooks left no duplicates: one
// payment, one ledger transaction, one receipt, one webhook_events row.
func assertExactlyOne(t *testing.T, env *integrationEnv, providerRef, bookingID string) {
	t.Helper()
	ctx := context.Background()
	counts := map[string]int{}
	queries := map[string]string{
		"payments":       `SELECT count(*) FROM payments WHERE provider_reference = $1`,
		"ledger_txns":    `SELECT count(*) FROM ledger_transactions WHERE reference = 'PAY:' || $1`,
		"receipts":       `SELECT count(*) FROM receipts WHERE booking_id = $1`,
		"webhook_events": `SELECT count(*) FROM webhook_events WHERE event_reference = $1`,
	}
	args := map[string]string{
		"payments": providerRef, "ledger_txns": providerRef,
		"receipts": bookingID, "webhook_events": providerRef,
	}
	for name, q := range queries {
		var n int
		if err := env.pool.QueryRow(ctx, q, args[name]).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", name, err)
		}
		counts[name] = n
		if n != 1 {
			t.Fatalf("%s = %d, want exactly 1 after replays (%v)", name, n, counts)
		}
	}
}

// assertReversalNetsToZero verifies the refund reversal: a REV:<ref>
// transaction exists whose entries mirror the originals with flipped
// directions, so every account's derived balance returns to zero — while the
// original allocation entries remain intact (spec §9.2).
func assertReversalNetsToZero(t *testing.T, env *integrationEnv, providerRef string) {
	t.Helper()
	ctx := context.Background()

	var revID string
	err := env.pool.QueryRow(ctx,
		`SELECT id FROM ledger_transactions WHERE reference = $1 AND type = 'PAYMENT_REVERSAL'`,
		"REV:"+providerRef).Scan(&revID)
	if err != nil {
		t.Fatalf("reversal transaction missing: %v", err)
	}

	// Every account touched by the payment must now net to zero.
	var nonzero int
	if err := env.pool.QueryRow(ctx, `
		SELECT count(*) FROM (
			SELECT account_id,
				COALESCE(ROUND(SUM(CASE WHEN e.direction = 'credit' THEN e.amount ELSE -e.amount END) * 100), 0)::bigint AS bal
			FROM ledger_entries e
			JOIN ledger_transactions t ON t.id = e.transaction_id
			WHERE t.reference IN ('PAY:' || $1, 'REV:' || $1)
			GROUP BY account_id
		) s WHERE bal <> 0`, providerRef).Scan(&nonzero); err != nil {
		t.Fatalf("net balances: %v", err)
	}
	if nonzero != 0 {
		t.Fatalf("%d accounts non-zero after reversal, want 0", nonzero)
	}

	// Originals intact: the PAY: transaction still carries its 4 entries.
	var origEntries int
	if err := env.pool.QueryRow(ctx, `
		SELECT count(*) FROM ledger_entries e
		JOIN ledger_transactions t ON t.id = e.transaction_id
		WHERE t.reference = 'PAY:' || $1`, providerRef).Scan(&origEntries); err != nil {
		t.Fatalf("original entries: %v", err)
	}
	if origEntries != 4 {
		t.Fatalf("original entries = %d, want 4 preserved", origEntries)
	}
}

// TestPaymentStateMachine unit-checks the §8.3 edge set without a database.
func TestPaymentStateMachine(t *testing.T) {
	for _, tc := range []struct {
		from, to string
		want     bool
	}{
		{"CREATED", "PENDING", true},
		{"CREATED", "SUCCEEDED", false}, // must pass through PENDING
		{"PENDING", "SUCCEEDED", true},
		{"PENDING", "FAILED", true},
		{"PENDING", "EXPIRED", true},
		{"SUCCEEDED", "REFUND_PENDING", true},
		{"SUCCEEDED", "REFUNDED", false}, // must pass through REFUND_PENDING
		{"REFUND_PENDING", "PARTIALLY_REFUNDED", true},
		{"REFUND_PENDING", "REFUNDED", true},
		{"REFUNDED", "REFUND_PENDING", false}, // terminal
		{"FAILED", "PENDING", false},          // terminal
	} {
		if got := payments.CanTransition(tc.from, tc.to); got != tc.want {
			t.Fatalf("%s -> %s = %v, want %v", tc.from, tc.to, got, tc.want)
		}
	}
}
