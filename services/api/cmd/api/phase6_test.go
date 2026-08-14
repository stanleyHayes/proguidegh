package main

// Phase 6 integration tests: SOS → incident workflow, and the verified
// review flow with quality flags (spec §4.4, §12).

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	pauth "proguidegh/api/internal/platform/auth"
)

// phase6Fixture builds one guide (user + profile) and one booking in the
// given status, returning ids. Fixtures go straight through SQL: the state
// machine that produces these rows is covered by the Phase 4/5 suites —
// these tests exercise the SOS/review/incident endpoints over HTTP.
type phase6Fixture struct {
	guideID   string
	touristID string
	bookingID string
}

func (e *integrationEnv) phase6Fixture(t *testing.T, touristEmail, status string) phase6Fixture {
	t.Helper()
	ctx := context.Background()

	guideToken, err := pauth.NewOpaqueToken()
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	var guideID string
	if err := e.pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, status)
		 VALUES ($1, 'x', 'active') RETURNING id`,
		fmt.Sprintf("it-guide-%s@example.com", guideToken[:12])).Scan(&guideID); err != nil {
		t.Fatalf("insert guide user: %v", err)
	}
	if _, err := e.pool.Exec(ctx,
		`INSERT INTO guide_profiles (user_id, public_name, status) VALUES ($1, 'IT Guide', 'certified')`,
		guideID); err != nil {
		t.Fatalf("insert guide profile: %v", err)
	}

	var touristID string
	if err := e.pool.QueryRow(ctx,
		`SELECT id FROM users WHERE email = $1`, touristEmail).Scan(&touristID); err != nil {
		t.Fatalf("lookup tourist: %v", err)
	}

	var packageID string
	if err := e.pool.QueryRow(ctx,
		`SELECT id FROM tour_packages WHERE active LIMIT 1`).Scan(&packageID); err != nil {
		t.Fatalf("lookup package: %v", err)
	}

	refToken, err := pauth.NewOpaqueToken()
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	var bookingID string
	err = e.pool.QueryRow(ctx,
		`INSERT INTO bookings (reference, tourist_id, guide_id, package_id, starts_at, ends_at, status)
		 VALUES ($1, $2, $3, $4, now() - interval '2 hours', now() - interval '1 hour', $5)
		 RETURNING id`,
		"PGH-"+refToken[:8], touristID, guideID, packageID, status).Scan(&bookingID)
	if err != nil {
		t.Fatalf("insert booking: %v", err)
	}
	return phase6Fixture{guideID: guideID, touristID: touristID, bookingID: bookingID}
}

func TestSOSAndIncidentWorkflow(t *testing.T) {
	env := newIntegrationEnv(t)
	ctx := context.Background()

	touristEmail := uniqueEmail(t)
	tokens := env.registerAndLogin(touristEmail)
	access := tokens["access_token"].(string)
	fix := env.phase6Fixture(t, touristEmail, "IN_PROGRESS")

	// SOS from the tourist on an active booking → 201, critical incident.
	status, body := env.post("/api/v1/bookings/"+fix.bookingID+"/sos", map[string]any{
		"latitude": 5.6037, "longitude": -0.1870, "accuracy_m": 12.5,
	}, bearer(access))
	if status != http.StatusCreated {
		t.Fatalf("sos: got %d: %v", status, body)
	}
	incident, ok := body["incident"].(map[string]any)
	if !ok || incident["severity"] != "critical" {
		t.Fatalf("sos incident: %v", body)
	}
	incidentID := incident["id"].(string)

	// SOS events are created with the coordinates.
	var ackedAt *time.Time
	if err := env.pool.QueryRow(ctx,
		`SELECT acknowledged_at FROM sos_events WHERE booking_id = $1`, fix.bookingID).Scan(&ackedAt); err != nil {
		t.Fatalf("sos_events row: %v", err)
	}
	if ackedAt != nil {
		t.Fatal("sos event should start unacknowledged")
	}

	// A stranger gets 404 (existence never leaks).
	stranger := env.registerAndLogin(uniqueEmail(t))
	if status, _ := env.post("/api/v1/bookings/"+fix.bookingID+"/sos", map[string]any{
		"latitude": 5.6, "longitude": -0.18,
	}, bearer(stranger["access_token"].(string))); status != http.StatusNotFound {
		t.Fatalf("stranger sos: got %d, want 404", status)
	}

	// Admin list/detail with incidents.read.
	opsEmail := uniqueEmail(t)
	env.registerAndLogin(opsEmail)
	env.grantRole(opsEmail, "operations_agent")
	opsAccess := env.login(opsEmail)["access_token"].(string)

	status, body = env.get("/api/v1/admin/incidents?type=sos", bearer(opsAccess))
	if status != http.StatusOK {
		t.Fatalf("list incidents: got %d: %v", status, body)
	}
	if total, _ := body["total"].(float64); total < 1 {
		t.Fatalf("expected at least one sos incident: %v", body)
	}

	// Acknowledge → incident status flips, trail records it, SOS events ack.
	status, body = env.post("/api/v1/admin/incidents/"+incidentID+"/acknowledge", map[string]any{}, bearer(opsAccess))
	if status != http.StatusOK {
		t.Fatalf("acknowledge: got %d: %v", status, body)
	}
	if err := env.pool.QueryRow(ctx,
		`SELECT acknowledged_at FROM sos_events WHERE booking_id = $1`, fix.bookingID).Scan(&ackedAt); err != nil {
		t.Fatalf("sos_events row: %v", err)
	}
	if ackedAt == nil {
		t.Fatal("acknowledging the incident should acknowledge its SOS events")
	}

	// Note + escalate (already critical → 400) + resolve with note.
	if status, body = env.post("/api/v1/admin/incidents/"+incidentID+"/notes", map[string]any{
		"body": "Called the tourist; safe at hotel.",
	}, bearer(opsAccess)); status != http.StatusOK {
		t.Fatalf("note: got %d: %v", status, body)
	}
	if status, _ = env.post("/api/v1/admin/incidents/"+incidentID+"/escalate", map[string]any{}, bearer(opsAccess)); status != http.StatusBadRequest {
		t.Fatalf("escalate at critical: got %d, want 400", status)
	}
	if status, body = env.post("/api/v1/admin/incidents/"+incidentID+"/resolve", map[string]any{
		"note": "Tourist confirmed safe; false alarm.",
	}, bearer(opsAccess)); status != http.StatusOK {
		t.Fatalf("resolve: got %d: %v", status, body)
	}

	// The trail carries every action, timestamped and attributed.
	status, body = env.get("/api/v1/admin/incidents/"+incidentID, bearer(opsAccess))
	if status != http.StatusOK {
		t.Fatalf("detail: got %d: %v", status, body)
	}
	events, _ := body["events"].([]any)
	if len(events) < 3 {
		t.Fatalf("expected acknowledge+note+resolve trail events: %v", events)
	}

	// Permission gate: the tourist cannot read the admin queue.
	if status, _ = env.get("/api/v1/admin/incidents", bearer(access)); status != http.StatusForbidden {
		t.Fatalf("tourist admin list: got %d, want 403", status)
	}

	// SOS on a completed booking → 409.
	done := env.phase6Fixture(t, touristEmail, "COMPLETED")
	if status, _ = env.post("/api/v1/bookings/"+done.bookingID+"/sos", map[string]any{
		"latitude": 5.6, "longitude": -0.18,
	}, bearer(access)); status != http.StatusConflict {
		t.Fatalf("sos on completed: got %d, want 409", status)
	}
}

func TestVerifiedReviewsAndQualityFlags(t *testing.T) {
	env := newIntegrationEnv(t)

	touristEmail := uniqueEmail(t)
	tokens := env.registerAndLogin(touristEmail)
	access := tokens["access_token"].(string)
	fix := env.phase6Fixture(t, touristEmail, "COMPLETED")

	// Only a completed booking can be reviewed.
	pending := env.phase6Fixture(t, touristEmail, "CONFIRMED")
	if status, _ := env.post("/api/v1/bookings/"+pending.bookingID+"/review", map[string]any{
		"rating": 5,
	}, bearer(access)); status != http.StatusUnprocessableEntity {
		t.Fatalf("review uncompleted: got %d, want 422", status)
	}

	// Invalid rating and unknown tag are rejected.
	if status, _ := env.post("/api/v1/bookings/"+fix.bookingID+"/review", map[string]any{
		"rating": 6,
	}, bearer(access)); status != http.StatusBadRequest {
		t.Fatalf("rating 6: got %d, want 400", status)
	}
	if status, _ := env.post("/api/v1/bookings/"+fix.bookingID+"/review", map[string]any{
		"rating": 5, "tags": []string{"Mind Reader"},
	}, bearer(access)); status != http.StatusBadRequest {
		t.Fatalf("unknown tag: got %d, want 400", status)
	}

	// The tourist reviews once; a second attempt conflicts.
	status, body := env.post("/api/v1/bookings/"+fix.bookingID+"/review", map[string]any{
		"rating": 5, "body": "Fantastic heritage walk.", "tags": []string{"Knowledgeable", "Punctual"},
	}, bearer(access))
	if status != http.StatusCreated {
		t.Fatalf("review: got %d: %v", status, body)
	}
	if status, _ = env.post("/api/v1/bookings/"+fix.bookingID+"/review", map[string]any{
		"rating": 4,
	}, bearer(access)); status != http.StatusConflict {
		t.Fatalf("second review: got %d, want 409", status)
	}

	// Another user cannot review the tourist's booking (404, no leak).
	stranger := env.registerAndLogin(uniqueEmail(t))
	if status, _ = env.post("/api/v1/bookings/"+fix.bookingID+"/review", map[string]any{
		"rating": 5,
	}, bearer(stranger["access_token"].(string))); status != http.StatusNotFound {
		t.Fatalf("stranger review: got %d, want 404", status)
	}

	// Public listing shows the review and refreshed aggregate.
	status, body = env.get("/api/v1/guides/"+fix.guideID+"/reviews", nil)
	if status != http.StatusOK {
		t.Fatalf("public reviews: got %d: %v", status, body)
	}
	if avg, _ := body["rating_avg"].(float64); avg != 5 {
		t.Fatalf("rating_avg: got %v, want 5", body)
	}

	// Three 1-star reviews on separate completed bookings push the rolling
	// average below the 4.0 threshold → low_rating quality flag opens once.
	for i := 0; i < 3; i++ {
		f := env.phase6Fixture(t, touristEmail, "COMPLETED")
		f.guideID = fix.guideID // same guide
		if _, err := env.pool.Exec(context.Background(),
			`UPDATE bookings SET guide_id = $1 WHERE id = $2`, fix.guideID, f.bookingID); err != nil {
			t.Fatalf("reattach guide: %v", err)
		}
		if status, body := env.post("/api/v1/bookings/"+f.bookingID+"/review", map[string]any{
			"rating": 1,
		}, bearer(access)); status != http.StatusCreated {
			t.Fatalf("low review %d: got %d: %v", i, status, body)
		}
	}

	// Quality queue: content_admin (reviews.moderate) sees exactly one open
	// low_rating flag and resolves it with a note.
	contentEmail := uniqueEmail(t)
	env.registerAndLogin(contentEmail)
	env.grantRole(contentEmail, "content_admin")
	contentAccess := env.login(contentEmail)["access_token"].(string)

	status, body = env.get("/api/v1/admin/quality-flags?status=open", bearer(contentAccess))
	if status != http.StatusOK {
		t.Fatalf("quality flags: got %d: %v", status, body)
	}
	flags, _ := body["flags"].([]any)
	var flagID string
	for _, raw := range flags {
		f, _ := raw.(map[string]any)
		if f["guide_id"] == fix.guideID && f["kind"] == "low_rating" {
			if flagID != "" {
				t.Fatal("duplicate open low_rating flags for one guide")
			}
			flagID = f["id"].(string)
		}
	}
	if flagID == "" {
		t.Fatalf("no low_rating flag opened: %v", body)
	}

	if status, body = env.post("/api/v1/admin/quality-flags/"+flagID+"/resolve", map[string]any{
		"note": "Guide enrolled in retraining module 1.",
	}, bearer(contentAccess)); status != http.StatusOK {
		t.Fatalf("resolve flag: got %d: %v", status, body)
	}
	// Resolving again conflicts.
	if status, _ = env.post("/api/v1/admin/quality-flags/"+flagID+"/resolve", map[string]any{
		"note": "again",
	}, bearer(contentAccess)); status != http.StatusConflict {
		t.Fatalf("re-resolve: got %d, want 409", status)
	}
}
