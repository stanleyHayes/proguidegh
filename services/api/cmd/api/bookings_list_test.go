package main

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// bookingsListFixture creates SQL-level booking rows for the guide/admin
// list endpoints. The endpoints under test only read bookings (plus package /
// profile joins), so rows are inserted directly — no payment or dispatch
// walk needed. The shared integration DB carries bookings from other tests,
// so assertions use unique references and per-guide isolation instead of
// global totals.
type bookingsListFixture struct {
	env       *integrationEnv
	pkgID     string
	pkgName   string
	touristID string
	guideID   string
}

func newBookingsListFixture(t *testing.T) bookingsListFixture {
	t.Helper()
	env := newIntegrationEnv(t)
	ctx := context.Background()

	touristEmail := uniqueEmail(t)
	env.registerAndLogin(touristEmail) // tourist profile "Integration Tester"
	touristID := env.grantRole(touristEmail, "tourist")

	guideEmail := uniqueEmail(t)
	guideTokens := env.registerAndLogin(guideEmail)
	guideID := env.grantRole(guideEmail, "tourist") // resolves the user id
	// A guide profile so the admin board can render public_name.
	if _, err := env.pool.Exec(ctx, `
		INSERT INTO guide_profiles (user_id, public_name) VALUES ($1, 'Ama Ops')
		ON CONFLICT (user_id) DO NOTHING`, guideID); err != nil {
		t.Fatalf("guide profile: %v", err)
	}

	fx := bookingsListFixture{env: env, touristID: touristID, guideID: guideID}
	if err := env.pool.QueryRow(ctx,
		`SELECT id, name FROM tour_packages WHERE code = 'CITY_TOUR_4H'`).Scan(&fx.pkgID, &fx.pkgName); err != nil {
		t.Fatalf("package lookup: %v", err)
	}
	_ = guideTokens
	return fx
}

// insertBooking writes one booking (+ one status event) owned by the
// fixture tourist and returns its id. guide may be empty (unassigned).
func (fx bookingsListFixture) insertBooking(t *testing.T, ref, guideID, status string, start time.Time) string {
	t.Helper()
	ctx := context.Background()
	var guide any
	if guideID != "" {
		guide = guideID
	}
	var id string
	if err := fx.env.pool.QueryRow(ctx, `
		INSERT INTO bookings (reference, tourist_id, guide_id, package_id, starts_at, ends_at,
		                      status, meeting_point_text, num_guests, amount, currency)
		VALUES ($1, $2, $3, $4, $5::timestamptz, $5::timestamptz + interval '4 hours', $6, 'Flagstaff House', 2, 250.00, 'GHS')
		RETURNING id`, ref, fx.touristID, guide, fx.pkgID, start, status).Scan(&id); err != nil {
		t.Fatalf("insert booking %s: %v", ref, err)
	}
	if _, err := fx.env.pool.Exec(ctx, `
		INSERT INTO booking_status_events (booking_id, from_status, to_status, actor_id, metadata)
		VALUES ($1, NULL, $2, $3, '{"action":"test.setup"}')`, id, status, fx.touristID); err != nil {
		t.Fatalf("insert event %s: %v", ref, err)
	}
	return id
}

func bookingRefs(body map[string]any) []string {
	out := []string{}
	for _, b := range body["bookings"].([]any) {
		out = append(out, b.(map[string]any)["reference"].(string))
	}
	return out
}

// TestGuideBookingsList covers GET /api/v1/me/guide/bookings: the caller's
// assigned bookings only, upcoming-first (asc) then past (desc), with the
// split-friendly fields.
func TestGuideBookingsList(t *testing.T) {
	fx := newBookingsListFixture(t)
	env := fx.env

	guideEmail := uniqueEmail(t)
	guideAccess := env.registerAndLogin(guideEmail)["access_token"].(string)
	guideID := env.grantRole(guideEmail, "tourist")

	now := time.Now().UTC()
	up1 := fx.insertBooking(t, "PGH-GL-UP1-"+fmt.Sprint(now.UnixNano()), guideID, "CONFIRMED", now.Add(48*time.Hour))
	up2 := fx.insertBooking(t, "PGH-GL-UP2-"+fmt.Sprint(now.UnixNano()), guideID, "GUIDE_EN_ROUTE", now.Add(72*time.Hour))
	past := fx.insertBooking(t, "PGH-GL-PAST-"+fmt.Sprint(now.UnixNano()), guideID, "COMPLETED", now.Add(-24*time.Hour))
	_ = up1
	_ = up2
	_ = past
	// Another guide's booking must not leak into the list.
	fx.insertBooking(t, "PGH-GL-OTHER-"+fmt.Sprint(now.UnixNano()), fx.guideID, "CONFIRMED", now.Add(48*time.Hour))

	status, body := env.get("/api/v1/me/guide/bookings", bearer(guideAccess))
	if status != http.StatusOK {
		t.Fatalf("guide bookings: got %d: %v", status, body)
	}
	rows := body["bookings"].([]any)
	if len(rows) != 3 {
		t.Fatalf("guide bookings count = %d, want 3: %v", len(rows), body)
	}

	// Ordering: upcoming asc, then past desc.
	refs := bookingRefs(body)
	wantPrefix := []string{"PGH-GL-UP1-", "PGH-GL-UP2-", "PGH-GL-PAST-"}
	for i, prefix := range wantPrefix {
		if len(refs[i]) < len(prefix) || refs[i][:len(prefix)] != prefix {
			t.Fatalf("row %d = %s, want prefix %s (order %v)", i, refs[i], prefix, refs)
		}
	}

	// Field shape.
	first := rows[0].(map[string]any)
	for _, field := range []string{"id", "reference", "status", "package_name", "starts_at",
		"ends_at", "meeting_point", "num_guests", "amount", "tourist_name"} {
		if _, ok := first[field]; !ok {
			t.Fatalf("guide booking row missing %s: %v", field, first)
		}
	}
	if first["package_name"] != fx.pkgName {
		t.Fatalf("package_name = %v, want %s", first["package_name"], fx.pkgName)
	}
	if first["tourist_name"] != "Integration Tester" {
		t.Fatalf("tourist_name = %v", first["tourist_name"])
	}
	if first["meeting_point"] != "Flagstaff House" {
		t.Fatalf("meeting_point = %v", first["meeting_point"])
	}

	// Unauthenticated → 401.
	if status, _ := env.get("/api/v1/me/guide/bookings", nil); status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated guide bookings: got %d, want 401", status)
	}
}

// TestAdminBookingsList covers GET /api/v1/admin/bookings: the status=active
// alias, explicit single/comma status filters, permission enforcement and
// offset pagination over the operations-board shape.
func TestAdminBookingsList(t *testing.T) {
	fx := newBookingsListFixture(t)
	env := fx.env
	now := time.Now().UTC()

	adminEmail := uniqueEmail(t)
	env.registerAndLogin(adminEmail)
	env.grantRole(adminEmail, "super_admin")
	adminAccess := env.login(adminEmail)["access_token"].(string)

	tag := fmt.Sprint(now.UnixNano())
	fx.insertBooking(t, "PGH-AD-CONF-"+tag, fx.guideID, "CONFIRMED", now.Add(48*time.Hour))
	fx.insertBooking(t, "PGH-AD-ENR-"+tag, fx.guideID, "GUIDE_EN_ROUTE", now.Add(56*time.Hour))
	fx.insertBooking(t, "PGH-AD-ML-"+tag, "", "CONFIRMED", now.Add(48*time.Hour)) // guideless marketplace
	fx.insertBooking(t, "PGH-AD-DONE-"+tag, fx.guideID, "COMPLETED", now.Add(-24*time.Hour))

	contains := func(refs []string, prefix string) bool {
		for _, r := range refs {
			if len(r) >= len(prefix) && r[:len(prefix)] == prefix {
				return true
			}
		}
		return false
	}
	activeStatus := map[string]bool{"CONFIRMED": true, "GUIDE_EN_ROUTE": true, "GUIDE_ARRIVED": true, "IN_PROGRESS": true}

	// status=active: only on-calendar statuses; our three active rows present.
	status, body := env.get("/api/v1/admin/bookings?status=active&limit=100", bearer(adminAccess))
	if status != http.StatusOK {
		t.Fatalf("admin active bookings: got %d: %v", status, body)
	}
	for _, b := range body["bookings"].([]any) {
		s := b.(map[string]any)["status"].(string)
		if !activeStatus[s] {
			t.Fatalf("status=active returned %s row", s)
		}
	}
	refs := bookingRefs(body)
	for _, prefix := range []string{"PGH-AD-CONF-", "PGH-AD-ENR-", "PGH-AD-ML-"} {
		if !contains(refs, prefix) {
			t.Fatalf("status=active missing %s (got %v)", prefix, refs)
		}
	}
	if contains(refs, "PGH-AD-DONE-") {
		t.Fatalf("status=active leaked a COMPLETED row: %v", refs)
	}

	// Row shape: guide/tourist refs, package, event time.
	var doneRow map[string]any
	status, body = env.get("/api/v1/admin/bookings?status=COMPLETED&limit=100", bearer(adminAccess))
	if status != http.StatusOK {
		t.Fatalf("admin completed bookings: got %d: %v", status, body)
	}
	for _, b := range body["bookings"].([]any) {
		m := b.(map[string]any)
		// Rows are newest-first and earlier suite runs leave fixtures in the
		// shared dev database, so the FIRST prefix match is this run's row.
		if r := m["reference"].(string); len(r) >= 11 && r[:11] == "PGH-AD-DONE" {
			doneRow = m
			break
		}
	}
	if doneRow == nil {
		t.Fatalf("COMPLETED row not found: %v", bookingRefs(body))
	}
	guide := doneRow["guide"].(map[string]any)
	if guide["id"] != fx.guideID || guide["name"] != "Ama Ops" {
		t.Fatalf("guide ref = %v", guide)
	}
	tourist := doneRow["tourist"].(map[string]any)
	if tourist["id"] != fx.touristID || tourist["name"] != "Integration Tester" {
		t.Fatalf("tourist ref = %v", tourist)
	}
	if doneRow["package_name"] != fx.pkgName {
		t.Fatalf("package_name = %v", doneRow["package_name"])
	}
	if doneRow["last_event_at"] == nil || doneRow["updated_at"] == nil {
		t.Fatalf("missing timestamps: %v", doneRow)
	}

	// Guideless booking renders a null guide.
	status, body = env.get("/api/v1/admin/bookings?status=CONFIRMED&limit=100", bearer(adminAccess))
	if status != http.StatusOK {
		t.Fatalf("admin confirmed bookings: got %d: %v", status, body)
	}
	var marketplace map[string]any
	for _, b := range body["bookings"].([]any) {
		m := b.(map[string]any)
		if r := m["reference"].(string); len(r) >= 9 && r[:9] == "PGH-AD-ML" {
			marketplace = m
		}
	}
	if marketplace == nil {
		t.Fatal("guideless CONFIRMED row not found")
	}
	if marketplace["guide"] != nil {
		t.Fatalf("guideless booking has guide %v", marketplace["guide"])
	}

	// Comma list: CONFIRMED,COMPLETED includes both, excludes GUIDE_EN_ROUTE.
	status, body = env.get("/api/v1/admin/bookings?status=CONFIRMED,COMPLETED&limit=100", bearer(adminAccess))
	if status != http.StatusOK {
		t.Fatalf("admin comma filter: got %d: %v", status, body)
	}
	refs = bookingRefs(body)
	if !contains(refs, "PGH-AD-CONF-") || !contains(refs, "PGH-AD-DONE-") {
		t.Fatalf("comma filter rows: %v", refs)
	}
	if contains(refs, "PGH-AD-ENR-") {
		t.Fatalf("comma filter leaked GUIDE_EN_ROUTE: %v", refs)
	}

	// Invalid filter → 400.
	if status, _ := env.get("/api/v1/admin/bookings?status=bogus", bearer(adminAccess)); status != http.StatusBadRequest {
		t.Fatalf("invalid status filter: got %d, want 400", status)
	}

	// Offset pagination shape.
	status, body = env.get("/api/v1/admin/bookings?limit=1&offset=1", bearer(adminAccess))
	if status != http.StatusOK {
		t.Fatalf("pagination: got %d: %v", status, body)
	}
	if got := len(body["bookings"].([]any)); got != 1 {
		t.Fatalf("limit=1 returned %d rows", got)
	}
	if body["limit"].(float64) != 1 || body["offset"].(float64) != 1 {
		t.Fatalf("pagination echo: %v", body)
	}

	// Permission: a plain tourist gets 403.
	touristAccess := env.registerAndLogin(uniqueEmail(t))["access_token"].(string)
	if status, _ := env.get("/api/v1/admin/bookings", bearer(touristAccess)); status != http.StatusForbidden {
		t.Fatalf("non-privileged admin bookings: got %d, want 403", status)
	}
}
