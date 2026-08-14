package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"proguidegh/api/internal/bookings"
)

// phase3Fixture bundles the Phase 3 journey setup: a verifier who can walk
// certification cases and a helper that produces a fully ACTIVE guide
// (the Phase 2 certification walk, reused).
type phase3Fixture struct {
	env            *integrationEnv
	verifierAccess string
	accraRegionID  string
	specialtyID    string
	cityTourPkgID  string
	heritagePkgID  string
}

// newPhase3Fixture creates the verifier and resolves seeded reference ids.
func newPhase3Fixture(t *testing.T) phase3Fixture {
	env := newIntegrationEnv(t)

	verifierEmail := uniqueEmail(t)
	env.registerAndLogin(verifierEmail)
	env.grantRole(verifierEmail, "verifier")
	verifierTokens := env.login(verifierEmail)

	fx := phase3Fixture{env: env, verifierAccess: verifierTokens["access_token"].(string)}

	status, body := env.get("/api/v1/regions", nil)
	if status != http.StatusOK {
		t.Fatalf("regions: got %d", status)
	}
	for _, r := range body["regions"].([]any) {
		rm := r.(map[string]any)
		if rm["code"] == "AA" {
			fx.accraRegionID = rm["id"].(string)
		}
	}
	status, body = env.get("/api/v1/specialties", nil)
	if status != http.StatusOK {
		t.Fatalf("specialties: got %d", status)
	}
	fx.specialtyID = body["specialties"].([]any)[0].(map[string]any)["id"].(string)

	status, body = env.get("/api/v1/tour-packages", nil)
	if status != http.StatusOK {
		t.Fatalf("tour-packages: got %d", status)
	}
	for _, p := range body["packages"].([]any) {
		pm := p.(map[string]any)
		switch pm["code"] {
		case "CITY_TOUR_4H":
			fx.cityTourPkgID = pm["id"].(string)
		case "HERITAGE_TOUR_8H":
			fx.heritagePkgID = pm["id"].(string)
		}
	}
	if fx.accraRegionID == "" || fx.cityTourPkgID == "" || fx.heritagePkgID == "" {
		t.Fatalf("seed resolution failed: %+v", fx)
	}
	return fx
}

// makeActiveGuide registers a guide, sets their profile (region AA, English
// language, one specialty, Accra operating base) and walks the certification
// pipeline to ACTIVE — the same walk as Phase 2's journey test.
func (fx phase3Fixture) makeActiveGuide(t *testing.T, name string) (guideID, access string) {
	t.Helper()
	env := fx.env

	email := uniqueEmail(t)
	tokens := env.registerAndLogin(email)
	access = tokens["access_token"].(string)

	status, body := env.post("/api/v1/guides/apply", map[string]any{"public_name": name}, bearer(access))
	if status != http.StatusCreated {
		t.Fatalf("apply %s: got %d: %v", name, status, body)
	}
	guideID = body["guide_profile"].(map[string]any)["user_id"].(string)
	caseID := body["certification_case"].(map[string]any)["id"].(string)

	status, body = env.doJSON(http.MethodPatch, "/api/v1/me/guide/profile", map[string]any{
		"bio":           "Accra guide.",
		"region_id":     fx.accraRegionID,
		"latitude":      "5.6037",
		"longitude":     "-0.1870",
		"languages":     []map[string]any{{"code": "en", "proficiency": "native"}, {"code": "tw", "proficiency": "fluent"}},
		"specialty_ids": []string{fx.specialtyID},
	}, bearer(access))
	if status != http.StatusOK {
		t.Fatalf("patch profile %s: got %d: %v", name, status, body)
	}

	uploadDoc := func(docType string) {
		st, b := env.post("/api/v1/guides/documents", map[string]any{
			"type": docType, "content_type": "image/png",
		}, bearer(access))
		if st != http.StatusCreated {
			t.Fatalf("upload %s: got %d: %v", docType, st, b)
		}
	}
	transition := func(to, evidenceRef string) {
		payload := map[string]any{"to_status": to, "reason": "phase3 setup"}
		if evidenceRef != "" {
			payload["evidence_ref"] = evidenceRef
		}
		st, b := env.post("/api/v1/admin/certification/"+caseID+"/transition", payload, bearer(fx.verifierAccess))
		if st != http.StatusOK {
			t.Fatalf("%s ->%s: got %d: %v", name, to, st, b)
		}
	}

	transition("IDENTITY_PENDING", "")
	uploadDoc("national_id")
	transition("IDENTITY_VERIFIED", "id-ref")
	transition("BACKGROUND_CHECK_PENDING", "")
	uploadDoc("background_check")
	transition("BACKGROUND_VERIFIED", "bg-ref")
	transition("TRAINING", "")
	transition("EXAM_PENDING", "")
	uploadDoc("certification")
	transition("CERTIFIED", "cert-ref")
	uploadDoc("insurance")
	transition("INSURANCE_ACTIVE", "policy-ref")
	transition("ACTIVE", "")
	return guideID, access
}

// setWeeklyWindow gives the guide one weekly window covering the given
// weekday, 08:00-20:00.
func (fx phase3Fixture) setWeeklyWindow(t *testing.T, access string, weekday int) {
	t.Helper()
	status, body := fx.env.doJSON(http.MethodPut, "/api/v1/me/guide/availability/schedule", map[string]any{
		"windows": []map[string]any{{"weekday": weekday, "start_time": "08:00", "end_time": "20:00"}},
	}, bearer(access))
	if status != http.StatusOK {
		t.Fatalf("put schedule: got %d: %v", status, body)
	}
}

func searchIDs(body map[string]any) map[string]bool {
	out := map[string]bool{}
	for _, g := range body["guides"].([]any) {
		out[g.(map[string]any)["user_id"].(string)] = true
	}
	return out
}

// nextWeekdayAt returns 10:00 UTC on the next occurrence of weekday,
// at least 2 days out so it is always in the future.
func nextWeekdayAt(weekday time.Weekday) time.Time {
	now := time.Now().UTC()
	d := time.Date(now.Year(), now.Month(), now.Day(), 10, 0, 0, 0, time.UTC)
	for int(d.Weekday()) != int(weekday) || d.Before(now.Add(48*time.Hour)) {
		d = d.AddDate(0, 0, 1)
	}
	return d
}

// TestSearchBookingJourney is the Phase 3 exit-criteria journey: an ACTIVE
// guide is discoverable through the §10.1 filters, the quote matches the
// server math, and a tourist creates an idempotent PAYMENT_PENDING booking
// without double-booking the guide.
func TestSearchBookingJourney(t *testing.T) {
	fx := newPhase3Fixture(t)
	env := fx.env

	guideID, guideAccess := fx.makeActiveGuide(t, "Ama Search")

	// Schedule: the weekday of the target booking date, 08:00-20:00.
	bookingStart := nextWeekdayAt(time.Monday)
	fx.setWeeklyWindow(t, guideAccess, int(bookingStart.Weekday()))

	// --- Search filters (§10.1) ----------------------------------------------
	mustContain := func(query string, want bool, label string) {
		t.Helper()
		status, body := env.get("/api/v1/guides/search?"+query, nil)
		if status != http.StatusOK {
			t.Fatalf("search %s: got %d: %v", label, status, body)
		}
		if got := searchIDs(body)[guideID]; got != want {
			t.Fatalf("search %s: guide present = %v, want %v (%v)", label, got, want, body)
		}
	}

	mustContain("region_id="+fx.accraRegionID, true, "region")
	mustContain("language=en", true, "language en")
	mustContain("language=fr", false, "language fr")
	mustContain("min_proficiency=native&language=en", true, "native en")
	mustContain("min_rating=4.5", false, "min_rating above current")
	mustContain("elite=true", false, "elite only")
	mustContain("lat=5.65&lng=-0.25&radius_km=20", true, "radius 20km")
	mustContain("lat=5.65&lng=-0.25&radius_km=5", false, "radius 5km")

	startsQ := bookingStart.Format(time.RFC3339)
	endsQ := bookingStart.Add(4 * time.Hour).Format(time.RFC3339)
	mustContain("starts_at="+startsQ+"&ends_at="+endsQ, true, "available window")
	// A weekday with no schedule window.
	otherDay := bookingStart.AddDate(0, 0, 1)
	mustContain("starts_at="+otherDay.Format(time.RFC3339)+
		"&ends_at="+otherDay.Add(4*time.Hour).Format(time.RFC3339), false, "unscheduled day")

	// Online presence (Redis, TTL): offline guide fails available_now.
	mustContain("available_now=true", false, "available_now while offline")
	status, body := env.post("/api/v1/me/guide/availability", map[string]any{"online": true}, bearer(guideAccess))
	if status != http.StatusOK {
		t.Fatalf("go online: got %d: %v", status, body)
	}
	mustContain("available_now=true", true, "available_now while online")
	mustContain("available_now=false", true, "listed when not filtering presence")

	// Time off blocks the dated search; deleting it restores.
	status, body = env.post("/api/v1/me/guide/availability/time-off", map[string]any{
		"starts_at": bookingStart.Add(-time.Hour).Format(time.RFC3339),
		"ends_at":   bookingStart.Add(5 * time.Hour).Format(time.RFC3339),
		"reason":    "family commitment",
	}, bearer(guideAccess))
	if status != http.StatusCreated {
		t.Fatalf("add time off: got %d: %v", status, body)
	}
	timeOffID := body["time_off"].(map[string]any)["id"].(string)
	mustContain("starts_at="+startsQ+"&ends_at="+endsQ, false, "time off blocks window")
	status, _ = env.doJSON(http.MethodDelete, "/api/v1/me/guide/availability/time-off/"+timeOffID, nil, bearer(guideAccess))
	if status != http.StatusNoContent {
		t.Fatalf("delete time off: got %d", status)
	}
	mustContain("starts_at="+startsQ+"&ends_at="+endsQ, true, "time off removed")

	// Bad filter combinations are 400.
	status, _ = env.get("/api/v1/guides/search?lat=5.6", nil)
	if status != http.StatusBadRequest {
		t.Fatalf("lat without lng: got %d, want 400", status)
	}

	// --- Quote (server-authoritative, spec §14/§27) ---------------------------
	status, body = env.post("/api/v1/bookings/quote", map[string]any{
		"package_id": fx.heritagePkgID, "starts_at": startsQ, "guests": 2,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("quote: got %d: %v", status, body)
	}
	quote := body["quote"].(map[string]any)
	price := quote["price"].(map[string]any)
	// Spec §9.1 example: GHS 450 at 15%/3%.
	if price["amount"] != "450.00" || price["platform_fee"] != "67.50" ||
		price["tourism_levy"] != "13.50" || price["guide_payable_estimate"] != "369.00" {
		t.Fatalf("quote price = %v, want 450.00/67.50/13.50/369.00", price)
	}
	quoteEnd, _ := time.Parse(time.RFC3339, quote["ends_at"].(string))
	if !quoteEnd.Equal(bookingStart.Add(8 * time.Hour)) {
		t.Fatalf("quote ends_at = %v, want +8h (480 min package)", quoteEnd)
	}
	status, _ = env.post("/api/v1/bookings/quote", map[string]any{
		"package_id": "00000000-0000-0000-0000-000000000000", "starts_at": startsQ,
	}, nil)
	if status != http.StatusNotFound {
		t.Fatalf("quote unknown package: got %d, want 404", status)
	}

	// --- Booking creation (idempotent, spec §14) ------------------------------
	touristEmail := uniqueEmail(t)
	touristTokens := env.registerAndLogin(touristEmail)
	touristAccess := touristTokens["access_token"].(string)

	createPayload := map[string]any{
		"package_id":    fx.cityTourPkgID,
		"guide_id":      guideID,
		"starts_at":     startsQ,
		"meeting_point": "Independence Arch, Accra",
		"meeting_lat":   json.Number("5.5474"),
		"meeting_lng":   json.Number("-0.2055"),
		"guests":        2,
		"notes":         "Two adults, one camera.",
	}

	// Idempotency-Key is required.
	status, body = env.post("/api/v1/bookings", createPayload, bearer(touristAccess))
	if status != http.StatusBadRequest || errCode(body) != "IDEMPOTENCY_KEY_REQUIRED" {
		t.Fatalf("create without key: got %d %v, want 400 IDEMPOTENCY_KEY_REQUIRED", status, body)
	}

	withKey := func(key string) map[string]string {
		return map[string]string{"Authorization": "Bearer " + touristAccess, "Idempotency-Key": key}
	}
	status, body = env.post("/api/v1/bookings", createPayload, withKey("idem-phase3-1"))
	if status != http.StatusCreated {
		t.Fatalf("create: got %d: %v", status, body)
	}
	booking := body["booking"].(map[string]any)
	bookingID := booking["id"].(string)
	reference := booking["reference"].(string)
	if !strings.HasPrefix(reference, "PGH-") || booking["status"] != "PAYMENT_PENDING" {
		t.Fatalf("booking = %v, want PGH-* reference in PAYMENT_PENDING", booking)
	}
	if booking["amount"] != "250.00" || booking["num_guests"] != float64(2) {
		t.Fatalf("booking snapshot = %v, want amount 250.00 (CITY_TOUR_4H) guests 2", booking)
	}
	bookEnd, _ := time.Parse(time.RFC3339, booking["ends_at"].(string))
	if !bookEnd.Equal(bookingStart.Add(4 * time.Hour)) {
		t.Fatalf("booking ends_at = %v, want +4h", bookEnd)
	}

	// Replay: same key + payload returns the same booking.
	status, body = env.post("/api/v1/bookings", createPayload, withKey("idem-phase3-1"))
	if status != http.StatusOK {
		t.Fatalf("replay: got %d: %v", status, body)
	}
	replayed := body["booking"].(map[string]any)
	if replayed["id"] != bookingID || replayed["reference"] != reference {
		t.Fatalf("replay returned %v, want same booking %s/%s", replayed["id"], bookingID, reference)
	}

	// Same key, different payload: conflict.
	changed := map[string]any{}
	for k, v := range createPayload {
		changed[k] = v
	}
	changed["guests"] = 4
	status, body = env.post("/api/v1/bookings", changed, withKey("idem-phase3-1"))
	if status != http.StatusConflict || errCode(body) != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("conflict replay: got %d %v, want 409 IDEMPOTENCY_CONFLICT", status, body)
	}

	// --- Eligibility & availability gates -------------------------------------
	// An applicant (never certified) is not bookable.
	applicantEmail := uniqueEmail(t)
	applicantTokens := env.registerAndLogin(applicantEmail)
	applicantAccess := applicantTokens["access_token"].(string)
	status, body = env.post("/api/v1/guides/apply", map[string]any{"public_name": "Not Certified"}, bearer(applicantAccess))
	applicantID := body["guide_profile"].(map[string]any)["user_id"].(string)
	ineligible := map[string]any{}
	for k, v := range createPayload {
		ineligible[k] = v
	}
	ineligible["guide_id"] = applicantID
	status, body = env.post("/api/v1/bookings", ineligible, withKey("idem-phase3-2"))
	if status != http.StatusUnprocessableEntity || errCode(body) != "GUIDE_NOT_ELIGIBLE" {
		t.Fatalf("non-ACTIVE guide: got %d %v, want 422 GUIDE_NOT_ELIGIBLE", status, body)
	}

	// Outside the weekly window: unavailable.
	outside := map[string]any{}
	for k, v := range createPayload {
		outside[k] = v
	}
	outside["starts_at"] = otherDay.Format(time.RFC3339)
	status, body = env.post("/api/v1/bookings", outside, withKey("idem-phase3-3"))
	if status != http.StatusUnprocessableEntity || errCode(body) != "GUIDE_UNAVAILABLE" {
		t.Fatalf("outside schedule: got %d %v, want 422 GUIDE_UNAVAILABLE", status, body)
	}

	// --- Overlap guard (§10.2) -------------------------------------------------
	// Confirm the first booking through the domain transition (payment
	// confirmation is Phase 4; the state machine is the only write path).
	bookingRepo := bookings.NewRepository(env.pool)
	if _, _, err := bookingRepo.Transition(context.Background(), bookingID, "",
		bookings.StatusConfirmed, json.RawMessage(`{"actor":"phase3-test"}`)); err != nil {
		t.Fatalf("confirm booking: %v", err)
	}

	// A second booking intersecting the confirmed slot is refused.
	status, body = env.post("/api/v1/bookings", createPayload, withKey("idem-phase3-4"))
	if status != http.StatusConflict || errCode(body) != "BOOKING_OVERLAP" {
		t.Fatalf("overlapping booking: got %d %v, want 409 BOOKING_OVERLAP", status, body)
	}

	// The exclusion constraint is the database backstop: a raw overlapping
	// CONFIRMED insert must fail even bypassing the service.
	err := env.pool.QueryRow(context.Background(), `
		INSERT INTO bookings (reference, tourist_id, guide_id, package_id, starts_at, ends_at, status, amount)
		VALUES ('PGH-BACKSTOP', $1, $2, $3, $4, $5, 'CONFIRMED', 100.00)
		RETURNING id`,
		booking["tourist_id"], guideID, fx.cityTourPkgID,
		bookingStart.Add(2*time.Hour), bookingStart.Add(6*time.Hour)).Scan(new(string))
	if err == nil {
		t.Fatal("exclusion constraint did not reject an overlapping CONFIRMED insert")
	}

	// An adjacent (non-overlapping) slot on the same day still books fine.
	later := map[string]any{}
	for k, v := range createPayload {
		later[k] = v
	}
	later["starts_at"] = bookingStart.Add(6 * time.Hour).Format(time.RFC3339) // 16:00-20:00
	status, body = env.post("/api/v1/bookings", later, withKey("idem-phase3-5"))
	if status != http.StatusCreated {
		t.Fatalf("adjacent booking: got %d: %v", status, body)
	}
	secondBookingID := body["booking"].(map[string]any)["id"].(string)

	// The confirmed booking also removes the guide from dated search results.
	mustContain("starts_at="+startsQ+"&ends_at="+endsQ, false, "confirmed booking blocks window")

	// --- Detail visibility -----------------------------------------------------
	status, body = env.get("/api/v1/bookings/"+bookingID, bearer(touristAccess))
	if status != http.StatusOK {
		t.Fatalf("owner detail: got %d: %v", status, body)
	}
	events := body["events"].([]any)
	if len(events) != 3 { // NULL->DRAFT, DRAFT->PAYMENT_PENDING, ->CONFIRMED
		t.Fatalf("events = %v, want 3", events)
	}

	status, _ = env.get("/api/v1/bookings/"+bookingID, bearer(guideAccess))
	if status != http.StatusOK {
		t.Fatalf("guide detail: got %d, want 200", status)
	}

	strangerTokens := env.registerAndLogin(uniqueEmail(t))
	status, _ = env.get("/api/v1/bookings/"+bookingID, bearer(strangerTokens["access_token"].(string)))
	if status != http.StatusNotFound {
		t.Fatalf("stranger detail: got %d, want 404 (existence must not leak)", status)
	}

	opsEmail := uniqueEmail(t)
	env.registerAndLogin(opsEmail)
	env.grantRole(opsEmail, "operations_agent")
	opsTokens := env.login(opsEmail)
	status, _ = env.get("/api/v1/bookings/"+bookingID, bearer(opsTokens["access_token"].(string)))
	if status != http.StatusOK {
		t.Fatalf("bookings.read detail: got %d, want 200", status)
	}

	// --- Own booking history, cursor pagination -------------------------------
	status, body = env.get("/api/v1/me/bookings?limit=1", bearer(touristAccess))
	if status != http.StatusOK {
		t.Fatalf("me/bookings: got %d: %v", status, body)
	}
	page1 := body["bookings"].([]any)
	if len(page1) != 1 || body["next_cursor"] == nil {
		t.Fatalf("page 1 = %v, want 1 row + next_cursor", body)
	}
	// Newest first: the adjacent (second) booking comes first.
	if page1[0].(map[string]any)["id"] != secondBookingID {
		t.Fatalf("page 1 first = %v, want second booking %s", page1[0], secondBookingID)
	}
	cursor := body["next_cursor"].(string)
	status, body = env.get("/api/v1/me/bookings?limit=1&cursor="+cursor, bearer(touristAccess))
	if status != http.StatusOK {
		t.Fatalf("me/bookings page 2: got %d: %v", status, body)
	}
	page2 := body["bookings"].([]any)
	if len(page2) != 1 || page2[0].(map[string]any)["id"] != bookingID {
		t.Fatalf("page 2 = %v, want the first booking %s", page2, bookingID)
	}
	if body["next_cursor"] != nil {
		t.Fatalf("page 2 next_cursor = %v, want null (2 bookings total)", body["next_cursor"])
	}
	status, _ = env.get("/api/v1/me/bookings?cursor=not-a-valid-cursor", bearer(touristAccess))
	if status != http.StatusBadRequest {
		t.Fatalf("bad cursor: got %d, want 400", status)
	}
}

// TestGuideAvailabilitySelfService covers the availability endpoints'
// validation and scoping without the full certification walk.
func TestGuideAvailabilitySelfService(t *testing.T) {
	env := newIntegrationEnv(t)

	// Auth required on all four endpoints.
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/me/guide/availability"},
		{http.MethodPut, "/api/v1/me/guide/availability/schedule"},
		{http.MethodPost, "/api/v1/me/guide/availability/time-off"},
		{http.MethodDelete, "/api/v1/me/guide/availability/time-off/00000000-0000-0000-0000-000000000000"},
	} {
		status, _ := env.doJSON(tc.method, tc.path, map[string]any{}, nil)
		if status != http.StatusUnauthorized {
			t.Fatalf("%s %s unauthenticated: got %d, want 401", tc.method, tc.path, status)
		}
	}

	// A tourist without a guide profile gets 404 NO_GUIDE_PROFILE.
	tokens := env.registerAndLogin(uniqueEmail(t))
	access := tokens["access_token"].(string)
	status, body := env.post("/api/v1/me/guide/availability", map[string]any{"online": true}, bearer(access))
	if status != http.StatusNotFound || errCode(body) != "NO_GUIDE_PROFILE" {
		t.Fatalf("availability without profile: got %d %v, want 404 NO_GUIDE_PROFILE", status, body)
	}

	// Apply (no certification needed for availability self-service).
	status, body = env.post("/api/v1/guides/apply", map[string]any{"public_name": "Availability Tester"}, bearer(access))
	if status != http.StatusCreated {
		t.Fatalf("apply: got %d: %v", status, body)
	}

	// Schedule validation.
	status, _ = env.doJSON(http.MethodPut, "/api/v1/me/guide/availability/schedule", map[string]any{
		"windows": []map[string]any{{"weekday": 7, "start_time": "08:00", "end_time": "12:00"}},
	}, bearer(access))
	if status != http.StatusBadRequest {
		t.Fatalf("weekday 7: got %d, want 400", status)
	}
	status, _ = env.doJSON(http.MethodPut, "/api/v1/me/guide/availability/schedule", map[string]any{
		"windows": []map[string]any{{"weekday": 1, "start_time": "18:00", "end_time": "08:00"}},
	}, bearer(access))
	if status != http.StatusBadRequest {
		t.Fatalf("overnight window: got %d, want 400", status)
	}
	status, body = env.doJSON(http.MethodPut, "/api/v1/me/guide/availability/schedule", map[string]any{
		"windows": []map[string]any{
			{"weekday": 1, "start_time": "08:00", "end_time": "12:00"},
			{"weekday": 1, "start_time": "14:00", "end_time": "18:00", "timezone": "Africa/Accra"},
		},
	}, bearer(access))
	if status != http.StatusOK {
		t.Fatalf("put schedule: got %d: %v", status, body)
	}
	if got := len(body["schedule"].([]any)); got != 2 {
		t.Fatalf("schedule = %v, want 2 windows", body["schedule"])
	}

	// Online toggle round trip.
	status, body = env.post("/api/v1/me/guide/availability", map[string]any{"online": true}, bearer(access))
	if status != http.StatusOK || body["online"] != true || body["ttl_seconds"] != float64(300) {
		t.Fatalf("go online: got %d: %v", status, body)
	}
	if err := env.rdb.Get(context.Background(), "presence:guide:"+body["guide_id"].(string)).Err(); err != nil {
		t.Fatalf("presence key missing after going online: %v", err)
	}
	status, _ = env.post("/api/v1/me/guide/availability", map[string]any{"online": false}, bearer(access))
	if status != http.StatusOK {
		t.Fatalf("go offline: got %d", status)
	}
	if err := env.rdb.Get(context.Background(), "presence:guide:"+body["guide_id"].(string)).Err(); err == nil {
		t.Fatal("presence key still set after going offline")
	}

	// Time-off validation and cross-guide scoping.
	status, _ = env.post("/api/v1/me/guide/availability/time-off", map[string]any{
		"starts_at": time.Now().Add(time.Hour).Format(time.RFC3339),
		"ends_at":   time.Now().Format(time.RFC3339), // ends before start
	}, bearer(access))
	if status != http.StatusBadRequest {
		t.Fatalf("inverted time off: got %d, want 400", status)
	}
	status, body = env.post("/api/v1/me/guide/availability/time-off", map[string]any{
		"starts_at": time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		"ends_at":   time.Now().Add(48 * time.Hour).Format(time.RFC3339),
	}, bearer(access))
	if status != http.StatusCreated {
		t.Fatalf("add time off: got %d: %v", status, body)
	}
	timeOffID := body["time_off"].(map[string]any)["id"].(string)

	otherTokens := env.registerAndLogin(uniqueEmail(t))
	otherAccess := otherTokens["access_token"].(string)
	if status, _ := env.post("/api/v1/guides/apply", map[string]any{"public_name": "Other Guide"}, bearer(otherAccess)); status != http.StatusCreated {
		t.Fatalf("other apply: got %d", status)
	}
	status, _ = env.doJSON(http.MethodDelete, "/api/v1/me/guide/availability/time-off/"+timeOffID, nil, bearer(otherAccess))
	if status != http.StatusNotFound {
		t.Fatalf("delete another guide's time off: got %d, want 404", status)
	}
	status, _ = env.doJSON(http.MethodDelete, "/api/v1/me/guide/availability/time-off/"+timeOffID, nil, bearer(access))
	if status != http.StatusNoContent {
		t.Fatalf("delete own time off: got %d, want 204", status)
	}
}

// TestQuoteMathOverHTTP spot-checks the server math for all three seeded
// packages (spec §27) through the public quote endpoint.
func TestQuoteMathOverHTTP(t *testing.T) {
	fx := newPhase3Fixture(t)
	startsAt := nextWeekdayAt(time.Wednesday).Format(time.RFC3339)

	want := map[string][4]string{
		fx.cityTourPkgID: {"250.00", "37.50", "7.50", "205.00"},
		fx.heritagePkgID: {"450.00", "67.50", "13.50", "369.00"},
	}
	for pkgID, w := range want {
		status, body := fx.env.post("/api/v1/bookings/quote", map[string]any{
			"package_id": pkgID, "starts_at": startsAt,
		}, nil)
		if status != http.StatusOK {
			t.Fatalf("quote %s: got %d: %v", pkgID, status, body)
		}
		price := body["quote"].(map[string]any)["price"].(map[string]any)
		got := fmt.Sprintf("%v/%v/%v/%v", price["amount"], price["platform_fee"],
			price["tourism_levy"], price["guide_payable_estimate"])
		if wantStr := strings.Join(w[:], "/"); got != wantStr {
			t.Fatalf("quote %s = %s, want %s", pkgID, got, wantStr)
		}
	}
}
