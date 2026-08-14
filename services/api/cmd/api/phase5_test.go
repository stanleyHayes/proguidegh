package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"proguidegh/api/internal/payments"
)

// --- Phase 5 helpers ---------------------------------------------------------

// suspendAllGuides makes every pre-existing guide profile ineligible. The
// integration DB is shared across the whole suite (and across runs), and
// dispatch ranks ALL eligible guides globally — leftover ACTIVE guides from
// earlier tests/runs would otherwise take top-N offer slots. Phase 5 tests
// call this before creating their own fixture guides so the candidate set
// is exactly the guides the test created.
func (e *integrationEnv) suspendAllGuides() {
	e.t.Helper()
	if _, err := e.pool.Exec(context.Background(), `
		UPDATE guide_profiles SET status = 'suspended', updated_at = now()
		WHERE status NOT IN ('suspended', 'disabled')`); err != nil {
		e.t.Fatalf("suspend leftover guides: %v", err)
	}
}

// wsDial opens a WebSocket connection with the access token in the ?token=
// query parameter (browser-style; the cookie path is covered by REST tests).
func wsDial(t *testing.T, env *integrationEnv, path, token string) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url := "ws" + strings.TrimPrefix(env.server.URL, "http") + path + "?token=" + token
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("ws dial %s: %v", path, err)
	}
	return conn
}

// wsDialExpectFail asserts the upgrade is rejected with the given HTTP status.
func wsDialExpectFail(t *testing.T, env *integrationEnv, path, token string, wantStatus int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url := "ws" + strings.TrimPrefix(env.server.URL, "http") + path
	if token != "" {
		url += "?token=" + token
	}
	_, resp, err := websocket.Dial(ctx, url, nil)
	if err == nil {
		t.Fatalf("ws dial %s unexpectedly succeeded", path)
	}
	if resp == nil || resp.StatusCode != wantStatus {
		t.Fatalf("ws dial %s status = %v, want %d (err %v)", path, resp, wantStatus, err)
	}
}

type wsMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func wsRead(t *testing.T, conn *websocket.Conn) wsMessage {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, raw, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("ws read: %v", err)
	}
	var msg wsMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatalf("ws unmarshal: %v (%s)", err, raw)
	}
	return msg
}

// payAndConfirm drives a booking through payment-intent + signed mock
// webhook so it lands CONFIRMED (and, for marketplace bookings, dispatched).
func payAndConfirm(t *testing.T, env *integrationEnv, touristAccess, bookingID, keyPrefix string) {
	t.Helper()
	withKey := map[string]string{
		"Authorization": "Bearer " + touristAccess, "Idempotency-Key": keyPrefix + "-intent",
	}
	status, body := env.post("/api/v1/bookings/"+bookingID+"/payment-intent", nil, withKey)
	if status != http.StatusCreated {
		t.Fatalf("intent: got %d: %v", status, body)
	}
	providerRef := body["payment"].(map[string]any)["provider_reference"].(string)

	mock := payments.NewMockProvider("dev-mock-webhook-secret")
	raw, sig := mock.SignWebhookPayload(providerRef)
	status, body = env.postRaw("/api/v1/webhooks/payments/mock", raw,
		map[string]string{payments.MockSignatureHeader: sig})
	if status != http.StatusOK {
		t.Fatalf("webhook: got %d: %v", status, body)
	}
}

// createMarketplaceBooking creates a guideless booking (dispatch flow).
func createMarketplaceBooking(t *testing.T, env *integrationEnv, fx phase3Fixture,
	touristAccess, key string, start time.Time) string {
	t.Helper()
	status, body := env.post("/api/v1/bookings", map[string]any{
		"package_id": fx.cityTourPkgID,
		"starts_at":  start.Format(time.RFC3339),
	}, map[string]string{"Authorization": "Bearer " + touristAccess, "Idempotency-Key": key})
	if status != http.StatusCreated {
		t.Fatalf("marketplace create: got %d: %v", status, body)
	}
	booking := body["booking"].(map[string]any)
	if booking["guide_id"] != nil {
		t.Fatalf("marketplace booking has guide %v, want unassigned", booking["guide_id"])
	}
	return booking["id"].(string)
}

func offerIDs(body map[string]any) []string {
	out := []string{}
	for _, o := range body["offers"].([]any) {
		out = append(out, o.(map[string]any)["id"].(string))
	}
	return out
}

// --- Tests -------------------------------------------------------------------

// TestMigration0006 evidence for the §31.24 bar: the migration is recorded
// and the new query-path indexes exist.
func TestMigration0006(t *testing.T) {
	env := newIntegrationEnv(t)
	ctx := context.Background()

	var applied bool
	if err := env.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = 6)`).Scan(&applied); err != nil {
		t.Fatalf("schema_migrations: %v", err)
	}
	if !applied {
		t.Fatal("migration 0006 not recorded in schema_migrations")
	}
	for _, idx := range []string{
		"idx_dispatch_offers_guide_status_expiry",
		"idx_dispatch_offers_booking",
		"idx_dispatch_offers_expiry",
		"idx_location_checkpoints_booking",
	} {
		var exists bool
		if err := env.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE indexname = $1)`, idx).Scan(&exists); err != nil {
			t.Fatalf("pg_indexes: %v", err)
		}
		if !exists {
			t.Fatalf("index %s missing", idx)
		}
	}
}

// TestDispatchAcceptanceJourney is the §30.2 dispatch bar: a marketplace
// booking is dispatched on payment confirmation; only eligible, available
// guides receive offers; offers arrive over the guide WS; acceptance is
// atomic first-wins (sequential and concurrent losers get 409).
func TestDispatchAcceptanceJourney(t *testing.T) {
	fx := newPhase3Fixture(t)
	fx.env.suspendAllGuides()
	env := fx.env
	ctx := context.Background()
	bookingStart := nextWeekdayAt(time.Tuesday)

	guideA, accessA := fx.makeActiveGuide(t, "Ama Dispatch")
	_, accessB := fx.makeActiveGuide(t, "Kofi Dispatch")
	_, accessC := fx.makeActiveGuide(t, "No Schedule")
	fx.setWeeklyWindow(t, accessA, int(bookingStart.Weekday()))
	fx.setWeeklyWindow(t, accessB, int(bookingStart.Weekday()))
	// accessC deliberately has no weekly schedule → not dispatchable.

	touristAccess := env.registerAndLogin(uniqueEmail(t))["access_token"].(string)

	bookingID := createMarketplaceBooking(t, env, fx, touristAccess, "idem-p5-a", bookingStart)
	payAndConfirm(t, env, touristAccess, bookingID, "p5-a")

	// Only the two scheduled, ACTIVE guides got offers.
	offersA := offerIDs(mustOffers(t, env, accessA, 1))
	offersB := offerIDs(mustOffers(t, env, accessB, 1))
	mustOffers(t, env, accessC, 0)
	if offersA[0] == offersB[0] {
		t.Fatal("guides share an offer id")
	}

	// Redis TTL keys cache the DB expiry for live offers.
	for _, offerID := range []string{offersA[0], offersB[0]} {
		if n, err := env.rdb.Exists(ctx, "offer:"+offerID).Result(); err != nil || n != 1 {
			t.Fatalf("offer redis key for %s = %d/%v, want 1", offerID, n, err)
		}
	}

	// The guide WS replays the live offer as its connect snapshot.
	wsB := wsDial(t, env, "/ws/guide", accessB)
	snap := wsRead(t, wsB)
	if snap.Type != "dispatch.offer" {
		t.Fatalf("guide ws snapshot = %s, want dispatch.offer", snap.Type)
	}
	var offerMsg map[string]any
	if err := json.Unmarshal(snap.Data, &offerMsg); err != nil {
		t.Fatalf("offer msg: %v", err)
	}
	if offerMsg["id"] != offersB[0] || offerMsg["booking_id"] != bookingID {
		t.Fatalf("ws offer = %v, want offer %s on booking %s", offerMsg, offersB[0], bookingID)
	}
	wsB.Close(websocket.StatusNormalClosure, "") //nolint:errcheck

	// Accept (guide A): booking assigned, offer ACCEPTED, sibling SUPERSEDED.
	status, body := env.post("/api/v1/offers/"+offersA[0]+"/accept", nil, bearer(accessA))
	if status != http.StatusOK {
		t.Fatalf("accept A: got %d: %v", status, body)
	}
	assigned := body["booking"].(map[string]any)
	if assigned["guide_id"] != guideA {
		t.Fatalf("assigned guide = %v, want %s", assigned["guide_id"], guideA)
	}

	// Sequential second accept: 409 (first valid acceptance wins).
	status, body = env.post("/api/v1/offers/"+offersB[0]+"/accept", nil, bearer(accessB))
	if status != http.StatusConflict {
		t.Fatalf("accept B after A: got %d, want 409: %v", status, body)
	}
	// A stranger guide never sees the offer (existence does not leak).
	status, _ = env.post("/api/v1/offers/"+offersA[0]+"/accept", nil, bearer(accessC))
	if status != http.StatusNotFound {
		t.Fatalf("accept by non-offer guide: got %d, want 404", status)
	}

	// Redis keys are dropped once the batch resolves.
	for _, offerID := range []string{offersA[0], offersB[0]} {
		if n, _ := env.rdb.Exists(ctx, "offer:"+offerID).Result(); n != 0 {
			t.Fatalf("offer redis key for %s still set", offerID)
		}
	}

	// The immutable event trail records the assignment.
	status, body = env.get("/api/v1/bookings/"+bookingID, bearer(touristAccess))
	if status != http.StatusOK {
		t.Fatalf("booking detail: got %d", status)
	}
	var sawAssign bool
	for _, e := range body["events"].([]any) {
		meta, _ := json.Marshal(e.(map[string]any)["metadata"])
		if strings.Contains(string(meta), "dispatch.guide_assigned") {
			sawAssign = true
		}
	}
	if !sawAssign {
		t.Fatalf("no dispatch.guide_assigned event: %v", body["events"])
	}

	// Operations can see the outcome of every offer (§30.2).
	opsEmail := uniqueEmail(t)
	env.registerAndLogin(opsEmail)
	env.grantRole(opsEmail, "operations_agent")
	opsAccess := env.login(opsEmail)["access_token"].(string)
	status, body = env.get("/api/v1/admin/bookings/"+bookingID+"/dispatch", bearer(opsAccess))
	if status != http.StatusOK {
		t.Fatalf("admin dispatch view: got %d: %v", status, body)
	}
	statuses := map[string]int{}
	for _, o := range body["offers"].([]any) {
		statuses[o.(map[string]any)["status"].(string)]++
	}
	if statuses["ACCEPTED"] != 1 || statuses["SUPERSEDED"] != 1 {
		t.Fatalf("offer outcomes = %v, want 1 ACCEPTED + 1 SUPERSEDED", statuses)
	}

	// --- Concurrent accepts: exactly one winner (§30.2) -------------------------
	booking2 := createMarketplaceBooking(t, env, fx, touristAccess, "idem-p5-b", bookingStart.Add(6*time.Hour))
	payAndConfirm(t, env, touristAccess, booking2, "p5-b")
	raceA := offerIDs(mustOffers(t, env, accessA, 1))
	raceB := offerIDs(mustOffers(t, env, accessB, 1))

	var wg sync.WaitGroup
	results := make([]int, 2)
	for i, tc := range []struct{ offer, access string }{{raceA[0], accessA}, {raceB[0], accessB}} {
		wg.Add(1)
		go func(i int, offerID, access string) {
			defer wg.Done()
			st, _ := env.post("/api/v1/offers/"+offerID+"/accept", nil, bearer(access))
			results[i] = st
		}(i, tc.offer, tc.access)
	}
	wg.Wait()
	wins, conflicts := 0, 0
	for _, st := range results {
		switch st {
		case http.StatusOK:
			wins++
		case http.StatusConflict:
			conflicts++
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("concurrent accepts = %v, want exactly one 200 and one 409", results)
	}

	// The winning guide holds the slot; the loser cannot double-book it.
	var winnerCount int
	if err := env.pool.QueryRow(ctx, `
		SELECT count(*) FROM bookings WHERE id = $1 AND guide_id IS NOT NULL`, booking2).Scan(&winnerCount); err != nil {
		t.Fatalf("winner check: %v", err)
	}
	if winnerCount != 1 {
		t.Fatalf("booking2 assigned %d guides, want 1", winnerCount)
	}
}

func mustOffers(t *testing.T, env *integrationEnv, access string, want int) map[string]any {
	t.Helper()
	status, body := env.get("/api/v1/me/guide/offers", bearer(access))
	if status != http.StatusOK {
		t.Fatalf("list offers: got %d: %v", status, body)
	}
	if got := len(body["offers"].([]any)); got != want {
		t.Fatalf("offers = %v, want %d", body["offers"], want)
	}
	return body
}

// TestDispatchDeclineExpiryPresence covers decline (+ re-dispatch), lazy and
// swept expiry (410 on accept), the presence gate for imminent bookings and
// the admin manual dispatch endpoint.
func TestDispatchDeclineExpiryPresence(t *testing.T) {
	fx := newPhase3Fixture(t)
	fx.env.suspendAllGuides()
	env := fx.env
	ctx := context.Background()
	bookingStart := nextWeekdayAt(time.Wednesday)

	_, accessA := fx.makeActiveGuide(t, "Declining Guide")
	fx.setWeeklyWindow(t, accessA, int(bookingStart.Weekday()))

	opsEmail := uniqueEmail(t)
	env.registerAndLogin(opsEmail)
	env.grantRole(opsEmail, "operations_agent")
	opsAccess := env.login(opsEmail)["access_token"].(string)

	touristAccess := env.registerAndLogin(uniqueEmail(t))["access_token"].(string)

	// --- Decline empties the batch; no candidates remain → no new offers. ------
	bookingID := createMarketplaceBooking(t, env, fx, touristAccess, "idem-p5-c", bookingStart)
	payAndConfirm(t, env, touristAccess, bookingID, "p5-c")
	offerID := offerIDs(mustOffers(t, env, accessA, 1))[0]

	status, body := env.post("/api/v1/offers/"+offerID+"/decline", nil, bearer(accessA))
	if status != http.StatusOK || body["offer"].(map[string]any)["status"] != "DECLINED" {
		t.Fatalf("decline: got %d: %v", status, body)
	}
	// Decline again → 409.
	status, _ = env.post("/api/v1/offers/"+offerID+"/decline", nil, bearer(accessA))
	if status != http.StatusConflict {
		t.Fatalf("double decline: got %d, want 409", status)
	}
	mustOffers(t, env, accessA, 0)

	// Admin manual dispatch runs a new batch but excludes the decliner.
	status, body = env.post("/api/v1/admin/bookings/"+bookingID+"/dispatch", nil, bearer(opsAccess))
	if status != http.StatusOK {
		t.Fatalf("admin dispatch: got %d: %v", status, body)
	}
	d := body["dispatch"].(map[string]any)
	if d["batch_seq"] != float64(2) || len(d["offers"].([]any)) != 0 {
		t.Fatalf("re-dispatch = %v, want batch 2 with no offers (decliner excluded)", d)
	}
	// Without dispatch.manage → 403.
	status, _ = env.post("/api/v1/admin/bookings/"+bookingID+"/dispatch", nil, bearer(touristAccess))
	if status != http.StatusForbidden {
		t.Fatalf("admin dispatch without permission: got %d, want 403", status)
	}
	// Audit row exists for the manual dispatch.
	var auditCount int
	if err := env.pool.QueryRow(ctx, `
		SELECT count(*) FROM audit_logs
		WHERE action = 'admin.bookings.dispatch' AND entity_id = $1`, bookingID).Scan(&auditCount); err != nil {
		t.Fatalf("audit: %v", err)
	}
	if auditCount == 0 {
		t.Fatal("no audit row for admin dispatch")
	}

	// --- Expired offers cannot be accepted (410) --------------------------------
	booking2 := createMarketplaceBooking(t, env, fx, touristAccess, "idem-p5-d", bookingStart.Add(6*time.Hour))
	payAndConfirm(t, env, touristAccess, booking2, "p5-d")
	// guideA declined only booking 1 → offered again here.
	offer2 := offerIDs(mustOffers(t, env, accessA, 1))[0]
	if _, err := env.pool.Exec(ctx, `
		UPDATE dispatch_offers
		SET offered_at = now() - interval '2 seconds', expires_at = now() - interval '1 second'
		WHERE id = $1`, offer2); err != nil {
		t.Fatalf("force expiry: %v", err)
	}
	status, body = env.post("/api/v1/offers/"+offer2+"/accept", nil, bearer(accessA))
	if status != http.StatusGone || errCode(body) != "OFFER_EXPIRED" {
		t.Fatalf("expired accept: got %d %v, want 410 OFFER_EXPIRED", status, body)
	}
	// Lazy expiry persisted the terminal state.
	var st string
	if err := env.pool.QueryRow(ctx,
		`SELECT status FROM dispatch_offers WHERE id = $1`, offer2).Scan(&st); err != nil {
		t.Fatalf("offer status: %v", err)
	}
	if st != "EXPIRED" {
		t.Fatalf("offer status = %s, want EXPIRED after lazy expiry", st)
	}

	// --- Presence gate: imminent bookings only offer to online guides -----------
	_, accessB := fx.makeActiveGuide(t, "Online Guide")
	fx.setWeeklyWindow(t, accessB, int(bookingStart.Weekday()))
	// Force every booking into the "available now" window.
	if _, err := env.pool.Exec(ctx, `
		UPDATE system_settings SET value_json = '200000' WHERE key = 'dispatch_presence_window_minutes'`); err != nil {
		t.Fatalf("presence window setting: %v", err)
	}
	defer func() {
		env.pool.Exec(ctx, `UPDATE system_settings SET value_json = '120'
			WHERE key = 'dispatch_presence_window_minutes'`) //nolint:errcheck
	}()

	// guideB online, guideA offline.
	status, _ = env.post("/api/v1/me/guide/availability", map[string]any{"online": true}, bearer(accessB))
	if status != http.StatusOK {
		t.Fatalf("go online: got %d", status)
	}
	booking3 := createMarketplaceBooking(t, env, fx, touristAccess, "idem-p5-e", bookingStart.Add(2*time.Hour))
	payAndConfirm(t, env, touristAccess, booking3, "p5-e")
	mustOffers(t, env, accessB, 1)
	mustOffers(t, env, accessA, 0)
}

// TestTourOperationsLifecycle covers the §8.2 tour edges: legal order,
// wrong-order 409, completion side effects (ends_at, pending→eligible
// ledger move), no dispatch for direct bookings, and the audited operations
// override.
func TestTourOperationsLifecycle(t *testing.T) {
	fx := newPhase3Fixture(t)
	fx.env.suspendAllGuides()
	env := fx.env
	ctx := context.Background()
	bookingStart := nextWeekdayAt(time.Thursday)

	guideID, guideAccess := fx.makeActiveGuide(t, "Tour Guide")
	fx.setWeeklyWindow(t, guideAccess, int(bookingStart.Weekday()))
	touristAccess := env.registerAndLogin(uniqueEmail(t))["access_token"].(string)

	// Direct booking (guide chosen at creation): dispatch is skipped.
	status, body := env.post("/api/v1/bookings", map[string]any{
		"package_id": fx.cityTourPkgID, "guide_id": guideID,
		"starts_at": bookingStart.Format(time.RFC3339),
	}, map[string]string{"Authorization": "Bearer " + touristAccess, "Idempotency-Key": "idem-p5-f"})
	if status != http.StatusCreated {
		t.Fatalf("direct create: got %d: %v", status, body)
	}
	bookingID := body["booking"].(map[string]any)["id"].(string)
	payAndConfirm(t, env, touristAccess, bookingID, "p5-f")

	var offerCount int
	if err := env.pool.QueryRow(ctx,
		`SELECT count(*) FROM dispatch_offers WHERE booking_id = $1`, bookingID).Scan(&offerCount); err != nil {
		t.Fatalf("offer count: %v", err)
	}
	if offerCount != 0 {
		t.Fatalf("direct booking produced %d dispatch offers, want 0", offerCount)
	}

	// Only the assigned guide drives the tour.
	status, _ = env.post("/api/v1/bookings/"+bookingID+"/en-route", nil, bearer(touristAccess))
	if status != http.StatusNotFound {
		t.Fatalf("tourist en-route: got %d, want 404", status)
	}

	// Legal chain: en-route → arrived → start → complete.
	for _, edge := range []string{"en-route", "arrived", "start", "complete"} {
		status, body = env.post("/api/v1/bookings/"+bookingID+"/"+edge, nil, bearer(guideAccess))
		if status != http.StatusOK {
			t.Fatalf("%s: got %d: %v", edge, status, body)
		}
	}
	got := body["booking"].(map[string]any)
	if got["status"] != "COMPLETED" || got["ends_at"] == nil {
		t.Fatalf("completed booking = %v, want COMPLETED with ends_at", got)
	}

	// Completion moved the guide payable pending → eligible (§9.2).
	var eligibleRef string
	if err := env.pool.QueryRow(ctx, `
		SELECT reference FROM ledger_transactions
		WHERE reference = $1 AND type = 'PAYOUT_ELIGIBILITY'`, "ELIGIBLE:"+bookingID).Scan(&eligibleRef); err != nil {
		t.Fatalf("eligible ledger move missing: %v", err)
	}

	// Terminal: further tour edges are illegal.
	status, _ = env.post("/api/v1/bookings/"+bookingID+"/complete", nil, bearer(guideAccess))
	if status != http.StatusConflict {
		t.Fatalf("re-complete: got %d, want 409", status)
	}

	// --- Wrong order ------------------------------------------------------------
	status, body = env.post("/api/v1/bookings", map[string]any{
		"package_id": fx.cityTourPkgID, "guide_id": guideID,
		"starts_at": bookingStart.Add(6 * time.Hour).Format(time.RFC3339),
	}, map[string]string{"Authorization": "Bearer " + touristAccess, "Idempotency-Key": "idem-p5-g"})
	if status != http.StatusCreated {
		t.Fatalf("second create: got %d: %v", status, body)
	}
	booking2 := body["booking"].(map[string]any)["id"].(string)
	payAndConfirm(t, env, touristAccess, booking2, "p5-g")

	status, _ = env.post("/api/v1/bookings/"+booking2+"/start", nil, bearer(guideAccess))
	if status != http.StatusConflict {
		t.Fatalf("start before en-route/arrived: got %d, want 409", status)
	}
	status, _ = env.post("/api/v1/bookings/"+booking2+"/en-route", nil, bearer(guideAccess))
	if status != http.StatusOK {
		t.Fatalf("en-route: got %d", status)
	}
	status, _ = env.post("/api/v1/bookings/"+booking2+"/start", nil, bearer(guideAccess))
	if status != http.StatusConflict {
		t.Fatalf("start skipping arrived: got %d, want 409", status)
	}

	// --- Operations override -----------------------------------------------------
	opsEmail := uniqueEmail(t)
	env.registerAndLogin(opsEmail)
	env.grantRole(opsEmail, "operations_agent")
	opsAccess := env.login(opsEmail)["access_token"].(string)

	// Reason is mandatory.
	status, _ = env.post("/api/v1/admin/bookings/"+booking2+"/transition",
		map[string]any{"to_status": "CANCELLED_BY_ADMIN"}, bearer(opsAccess))
	if status != http.StatusBadRequest {
		t.Fatalf("override without reason: got %d, want 400", status)
	}
	// Illegal edge (GUIDE_EN_ROUTE -> COMPLETED is not in the machine).
	status, _ = env.post("/api/v1/admin/bookings/"+booking2+"/transition",
		map[string]any{"to_status": "COMPLETED", "reason": "jump the queue"}, bearer(opsAccess))
	if status != http.StatusConflict {
		t.Fatalf("illegal override: got %d, want 409", status)
	}
	// Legal override, audited.
	status, body = env.post("/api/v1/admin/bookings/"+booking2+"/transition",
		map[string]any{"to_status": "CANCELLED_BY_ADMIN", "reason": "customer request via phone"}, bearer(opsAccess))
	if status != http.StatusOK || body["booking"].(map[string]any)["status"] != "CANCELLED_BY_ADMIN" {
		t.Fatalf("override: got %d: %v", status, body)
	}
	var auditCount int
	if err := env.pool.QueryRow(ctx, `
		SELECT count(*) FROM audit_logs
		WHERE action = 'admin.bookings.transition' AND entity_id = $1`, booking2).Scan(&auditCount); err != nil {
		t.Fatalf("audit: %v", err)
	}
	if auditCount == 0 {
		t.Fatal("no audit row for admin transition")
	}
	// Without dispatch.manage → 403.
	status, _ = env.post("/api/v1/admin/bookings/"+booking2+"/transition",
		map[string]any{"to_status": "REFUND_PENDING", "reason": "x"}, bearer(touristAccess))
	if status != http.StatusForbidden {
		t.Fatalf("override without permission: got %d, want 403", status)
	}
}

// TestLocationRealtimeJourney covers §11/§31.27: the location POST validates
// and gates on the active window and assignment; the tourist WS receives
// position updates; strangers and permissionless admins are refused;
// reconnects catch up via snapshot + REST; checkpoints stay coarse.
func TestLocationRealtimeJourney(t *testing.T) {
	fx := newPhase3Fixture(t)
	fx.env.suspendAllGuides()
	env := fx.env
	ctx := context.Background()
	bookingStart := nextWeekdayAt(time.Friday)

	guideID, guideAccess := fx.makeActiveGuide(t, "Tracking Guide")
	_, otherGuideAccess := fx.makeActiveGuide(t, "Other Guide")
	fx.setWeeklyWindow(t, guideAccess, int(bookingStart.Weekday()))
	touristAccess := env.registerAndLogin(uniqueEmail(t))["access_token"].(string)
	strangerAccess := env.registerAndLogin(uniqueEmail(t))["access_token"].(string)

	status, body := env.post("/api/v1/bookings", map[string]any{
		"package_id": fx.cityTourPkgID, "guide_id": guideID,
		"starts_at": bookingStart.Format(time.RFC3339),
	}, map[string]string{"Authorization": "Bearer " + touristAccess, "Idempotency-Key": "idem-p5-h"})
	if status != http.StatusCreated {
		t.Fatalf("create: got %d: %v", status, body)
	}
	bookingID := body["booking"].(map[string]any)["id"].(string)

	// Location before the active window → 409; from a non-assigned guide → 403.
	payAndConfirm(t, env, touristAccess, bookingID, "p5-h")
	loc := map[string]any{"latitude": 5.6037, "longitude": -0.1870, "accuracy_m": 8.5, "heading": 120, "speed_mps": 3.1}
	status, _ = env.post("/api/v1/bookings/"+bookingID+"/location", loc, bearer(guideAccess))
	if status != http.StatusConflict {
		t.Fatalf("location before en-route: got %d, want 409", status)
	}
	status, _ = env.post("/api/v1/bookings/"+bookingID+"/en-route", nil, bearer(guideAccess))
	if status != http.StatusOK {
		t.Fatalf("en-route: got %d", status)
	}
	status, _ = env.post("/api/v1/bookings/"+bookingID+"/location", loc, bearer(otherGuideAccess))
	if status != http.StatusForbidden {
		t.Fatalf("location from other guide: got %d, want 403", status)
	}
	// Out-of-range payload → 400.
	status, _ = env.post("/api/v1/bookings/"+bookingID+"/location",
		map[string]any{"latitude": 95.0, "longitude": -0.1870}, bearer(guideAccess))
	if status != http.StatusBadRequest {
		t.Fatalf("invalid latitude: got %d, want 400", status)
	}

	// Tourist WS: connect snapshot carries the booking state.
	wsTourist := wsDial(t, env, "/ws/booking/"+bookingID, touristAccess)
	snap := wsRead(t, wsTourist)
	if snap.Type != "booking.updated" {
		t.Fatalf("booking ws snapshot = %s, want booking.updated", snap.Type)
	}

	// Guide posts a position → tourist WS receives it; REST reads it too.
	status, body = env.post("/api/v1/bookings/"+bookingID+"/location", loc, bearer(guideAccess))
	if status != http.StatusAccepted {
		t.Fatalf("location post: got %d: %v", status, body)
	}
	msg := wsRead(t, wsTourist)
	if msg.Type != "location.update" {
		t.Fatalf("tourist ws = %s, want location.update", msg.Type)
	}
	var pos map[string]any
	if err := json.Unmarshal(msg.Data, &pos); err != nil {
		t.Fatalf("position msg: %v", err)
	}
	if pos["latitude"] != 5.6037 || pos["booking_id"] != bookingID {
		t.Fatalf("position = %v", pos)
	}

	status, body = env.get("/api/v1/bookings/"+bookingID+"/location", bearer(touristAccess))
	if status != http.StatusOK {
		t.Fatalf("tourist location get: got %d: %v", status, body)
	}
	// Stranger REST → 404; stranger WS → 403.
	status, _ = env.get("/api/v1/bookings/"+bookingID+"/location", bearer(strangerAccess))
	if status != http.StatusNotFound {
		t.Fatalf("stranger location get: got %d, want 404", status)
	}
	wsDialExpectFail(t, env, "/ws/booking/"+bookingID, strangerAccess, http.StatusForbidden)
	wsDialExpectFail(t, env, "/ws/booking/"+bookingID, "", http.StatusUnauthorized)

	// Admin operations WS requires dispatch.manage.
	wsDialExpectFail(t, env, "/ws/admin/operations", strangerAccess, http.StatusForbidden)
	opsEmail := uniqueEmail(t)
	env.registerAndLogin(opsEmail)
	env.grantRole(opsEmail, "operations_agent")
	opsAccess := env.login(opsEmail)["access_token"].(string)
	wsOps := wsDial(t, env, "/ws/admin/operations", opsAccess)
	wsOps.Close(websocket.StatusNormalClosure, "") //nolint:errcheck
	// Operations reads the live map position over REST too.
	status, _ = env.get("/api/v1/bookings/"+bookingID+"/location", bearer(opsAccess))
	if status != http.StatusOK {
		t.Fatalf("ops location get: got %d, want 200", status)
	}

	// Checkpoint coarseness: a second ping inside the interval does not add
	// a durable row; the arrived tour event does.
	var checkpoints int
	countCheckpoints := func() int {
		if err := env.pool.QueryRow(ctx, `
			SELECT count(*) FROM location_checkpoints WHERE booking_id = $1`, bookingID).Scan(&checkpoints); err != nil {
			t.Fatalf("checkpoints: %v", err)
		}
		return checkpoints
	}
	if n := countCheckpoints(); n != 1 {
		t.Fatalf("checkpoints after first ping = %d, want 1", n)
	}
	status, _ = env.post("/api/v1/bookings/"+bookingID+"/location", loc, bearer(guideAccess))
	if status != http.StatusAccepted {
		t.Fatalf("second ping: got %d", status)
	}
	if n := countCheckpoints(); n != 1 {
		t.Fatalf("checkpoints after rapid second ping = %d, want 1 (coarse)", n)
	}
	status, _ = env.post("/api/v1/bookings/"+bookingID+"/arrived", nil, bearer(guideAccess))
	if status != http.StatusOK {
		t.Fatalf("arrived: got %d", status)
	}
	if n := countCheckpoints(); n != 2 {
		t.Fatalf("checkpoints after tour event = %d, want 2 (event forced)", n)
	}

	// Disconnect/reconnect (§31.27): the tourist catches up from the connect
	// snapshot (booking state + last position) and REST.
	wsTourist.Close(websocket.StatusNormalClosure, "network flap") //nolint:errcheck
	wsTourist2 := wsDial(t, env, "/ws/booking/"+bookingID, touristAccess)
	first := wsRead(t, wsTourist2)
	second := wsRead(t, wsTourist2)
	got := map[string]bool{first.Type: true, second.Type: true}
	if !got["booking.updated"] || !got["location.update"] {
		t.Fatalf("reconnect snapshot types = %v, want booking.updated + location.update", got)
	}
	wsTourist2.Close(websocket.StatusNormalClosure, "") //nolint:errcheck
	status, body = env.get("/api/v1/bookings/"+bookingID, bearer(touristAccess))
	if status != http.StatusOK || body["booking"].(map[string]any)["status"] != "GUIDE_ARRIVED" {
		t.Fatalf("REST catch-up: got %d: %v", status, body)
	}

	// After completion the window closes: no more location reads (§11.2).
	status, _ = env.post("/api/v1/bookings/"+bookingID+"/start", nil, bearer(guideAccess))
	if status != http.StatusOK {
		t.Fatalf("start: got %d", status)
	}
	status, _ = env.post("/api/v1/bookings/"+bookingID+"/complete", nil, bearer(guideAccess))
	if status != http.StatusOK {
		t.Fatalf("complete: got %d", status)
	}
	status, _ = env.get("/api/v1/bookings/"+bookingID+"/location", bearer(touristAccess))
	if status != http.StatusNotFound {
		t.Fatalf("location after completion: got %d, want 404 (window closed)", status)
	}
	status, _ = env.post("/api/v1/bookings/"+bookingID+"/location", loc, bearer(guideAccess))
	if status != http.StatusConflict {
		t.Fatalf("post after completion: got %d, want 409", status)
	}
}
