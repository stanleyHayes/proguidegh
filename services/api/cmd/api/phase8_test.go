package main

// Phase 8 integration tests: the light LMS (course authoring, enrollment,
// lesson progress, server-scored quiz, certificates), executive KPIs and
// operational reports with the permitted CSV export, versioned notification
// templates, the settings policy editor and the audit viewer
// (P8-01…P8-04).

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	pauth "proguidegh/api/internal/platform/auth"
)

func TestTrainingLMS(t *testing.T) {
	env := newIntegrationEnv(t)
	ctx := context.Background()

	// Content admin authors the course.
	contentEmail := uniqueEmail(t)
	env.registerAndLogin(contentEmail)
	env.grantRole(contentEmail, "content_admin")
	contentAccess := env.login(contentEmail)["access_token"].(string)

	token, _ := pauth.NewOpaqueToken()
	code := "course-" + token[:8]
	status, body := env.post("/api/v1/admin/training/courses", map[string]any{
		"code": code, "title": "Safety Fundamentals", "pass_score": 50,
		"required_for_certification": true,
		"quiz": []map[string]any{
			{"question": "First step on SOS?", "options": []string{"Run", "Call operations"}, "answer_index": 1},
		},
		"modules": []map[string]any{
			{"title": "Basics", "lessons": []map[string]any{
				{"title": "Intro", "body": "Welcome."},
				{"title": "Protocols", "body": "Follow protocol."},
			}},
		},
	}, bearer(contentAccess))
	if status != http.StatusCreated {
		t.Fatalf("create course: got %d: %v", status, body)
	}
	course := body["course"].(map[string]any)
	courseID := course["id"].(string)

	// Duplicate code conflicts.
	status, _ = env.post("/api/v1/admin/training/courses", map[string]any{
		"code": code, "title": "Copy", "modules": []map[string]any{
			{"title": "M", "lessons": []map[string]any{{"title": "L"}}},
		},
	}, bearer(contentAccess))
	if status != http.StatusConflict {
		t.Fatalf("duplicate code: got %d, want 409", status)
	}

	// Tourists cannot author courses.
	touristEmail := uniqueEmail(t)
	env.registerAndLogin(touristEmail)
	touristAccess := env.login(touristEmail)["access_token"].(string)
	if status, _ := env.get("/api/v1/admin/training/courses", bearer(touristAccess)); status != http.StatusForbidden {
		t.Fatalf("tourist admin training: got %d, want 403", status)
	}

	// Guide enrolls.
	guideEmail := uniqueEmail(t)
	env.registerAndLogin(guideEmail)
	var guideID string
	if err := env.pool.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, guideEmail).Scan(&guideID); err != nil {
		t.Fatalf("guide lookup: %v", err)
	}
	if _, err := env.pool.Exec(ctx,
		`INSERT INTO guide_profiles (user_id, public_name, status) VALUES ($1, 'LMS Guide', 'certified')`, guideID); err != nil {
		t.Fatalf("guide profile: %v", err)
	}
	guideAccess := env.login(guideEmail)["access_token"].(string)

	status, body = env.post("/api/v1/me/training/courses/"+courseID+"/enroll", map[string]any{}, bearer(guideAccess))
	if status != http.StatusOK {
		t.Fatalf("enroll: got %d: %v", status, body)
	}
	// Re-enroll is idempotent.
	status, _ = env.post("/api/v1/me/training/courses/"+courseID+"/enroll", map[string]any{}, bearer(guideAccess))
	if status != http.StatusOK {
		t.Fatalf("re-enroll: got %d, want 200", status)
	}

	// Course detail: quiz answers are stripped from the guide surface.
	status, body = env.get("/api/v1/me/training/courses/"+courseID, bearer(guideAccess))
	if status != http.StatusOK {
		t.Fatalf("guide detail: got %d: %v", status, body)
	}
	quiz := body["quiz"].([]any)
	if _, leaked := quiz[0].(map[string]any)["answer_index"]; leaked {
		t.Fatal("answer_index leaked to the guide surface")
	}
	modules := body["modules"].([]any)
	lessons := modules[0].(map[string]any)["lessons"].([]any)
	lesson1 := lessons[0].(map[string]any)["id"].(string)
	lesson2 := lessons[1].(map[string]any)["id"].(string)

	// Lesson progress.
	for _, lessonID := range []string{lesson1, lesson2} {
		status, body = env.post("/api/v1/me/training/lessons/"+lessonID+"/complete", map[string]any{}, bearer(guideAccess))
		if status != http.StatusOK {
			t.Fatalf("complete lesson %s: got %d: %v", lessonID, status, body)
		}
	}
	if done := body["enrollment"].(map[string]any)["lessons_done"]; minor(done) != 2 {
		t.Fatalf("lessons_done: %v, want 2", done)
	}

	// Quiz: fail first, then pass — enrollment completes with a certificate.
	status, body = env.post("/api/v1/me/training/courses/"+courseID+"/quiz",
		map[string]any{"answers": []int{0}}, bearer(guideAccess))
	if status != http.StatusOK || body["passed"] != false {
		t.Fatalf("quiz fail attempt: %d %v", status, body)
	}
	status, body = env.post("/api/v1/me/training/courses/"+courseID+"/quiz",
		map[string]any{"answers": []int{1}}, bearer(guideAccess))
	if status != http.StatusOK || body["passed"] != true {
		t.Fatalf("quiz pass attempt: %d %v", status, body)
	}
	enrollment := body["enrollment"].(map[string]any)
	if enrollment["status"] != "completed" || enrollment["certificate_serial"] == nil {
		t.Fatalf("enrollment not completed: %v", enrollment)
	}
	serial := enrollment["certificate_serial"].(string)
	if !strings.HasPrefix(serial, "PGH-CERT-") {
		t.Fatalf("certificate serial: %v", serial)
	}

	status, body = env.get("/api/v1/me/training/certificates", bearer(guideAccess))
	if status != http.StatusOK {
		t.Fatalf("certificates: got %d: %v", status, body)
	}
	certs := body["certificates"].([]any)
	if len(certs) != 1 || certs[0].(map[string]any)["serial"] != serial {
		t.Fatalf("certificates: %v", certs)
	}

	// Admin roster shows the completion.
	status, body = env.get("/api/v1/admin/training/courses/"+courseID+"/enrollments", bearer(contentAccess))
	if status != http.StatusOK {
		t.Fatalf("roster: got %d: %v", status, body)
	}
	roster := body["enrollments"].([]any)
	if len(roster) != 1 || roster[0].(map[string]any)["status"] != "completed" {
		t.Fatalf("roster: %v", roster)
	}
}

func TestReportingTemplatesAndSettings(t *testing.T) {
	env := newIntegrationEnv(t)

	// KPIs and bookings report: reports.read (content_admin has it).
	contentEmail := uniqueEmail(t)
	env.registerAndLogin(contentEmail)
	env.grantRole(contentEmail, "content_admin")
	contentAccess := env.login(contentEmail)["access_token"].(string)

	status, body := env.get("/api/v1/admin/reports/kpis", bearer(contentAccess))
	if status != http.StatusOK {
		t.Fatalf("kpis: got %d: %v", status, body)
	}
	kpis := body["kpis"].(map[string]any)
	for _, field := range []string{"users_total", "guides_certified", "bookings_30d", "gmv_30d_minor"} {
		if _, ok := kpis[field]; !ok {
			t.Fatalf("kpis missing %s: %v", field, kpis)
		}
	}

	status, body = env.get("/api/v1/admin/reports/bookings", bearer(contentAccess))
	if status != http.StatusOK {
		t.Fatalf("bookings report: got %d: %v", status, body)
	}
	if body["report"].(map[string]any)["total"] == nil {
		t.Fatalf("bookings report shape: %v", body)
	}

	// CSV export needs reports.export — content_admin lacks it.
	if status, _ := env.get("/api/v1/admin/reports/bookings/export", bearer(contentAccess)); status != http.StatusForbidden {
		t.Fatalf("export without reports.export: got %d, want 403", status)
	}
	finEmail := uniqueEmail(t)
	env.registerAndLogin(finEmail)
	env.grantRole(finEmail, "finance_officer")
	finAccess := env.login(finEmail)["access_token"].(string)
	status, csv := env.getRaw("/api/v1/admin/reports/bookings/export", bearer(finAccess))
	if status != http.StatusOK || !strings.Contains(csv, "reference,tourist_email") {
		t.Fatalf("bookings export: %d %s", status, csv)
	}

	// Notification templates: version + activate (settings.manage).
	adminEmail := uniqueEmail(t)
	env.registerAndLogin(adminEmail)
	env.grantRole(adminEmail, "administrator")
	adminAccess := env.login(adminEmail)["access_token"].(string)

	status, body = env.post("/api/v1/admin/notification-templates", map[string]any{
		"key": "booking.confirmed", "channel": "email",
		"subject": "Confirmed v2", "body": "v2 body for {{booking_reference}}",
	}, bearer(adminAccess))
	if status != http.StatusCreated {
		t.Fatalf("create template: got %d: %v", status, body)
	}
	tmpl := body["template"].(map[string]any)
	if minor(tmpl["version"]) < 2 || tmpl["active"] != false {
		t.Fatalf("template version: %v", tmpl)
	}
	tmplID := tmpl["id"].(string)

	// Exactly one active version exists before activation, and it is not the
	// new one (earlier runs of this suite share the database, so the active
	// version number itself is not asserted).
	_, body = env.get("/api/v1/admin/notification-templates", bearer(adminAccess))
	activeBefore := 0
	for _, item := range body["templates"].([]any) {
		tp := item.(map[string]any)
		if tp["key"] == "booking.confirmed" && tp["active"] == true {
			activeBefore++
			if tp["id"] == tmplID {
				t.Fatal("new version should not be active before activation")
			}
		}
	}
	if activeBefore != 1 {
		t.Fatalf("active versions before activation: %d, want 1", activeBefore)
	}
	status, body = env.post("/api/v1/admin/notification-templates/"+tmplID+"/activate", map[string]any{}, bearer(adminAccess))
	if status != http.StatusOK || body["template"].(map[string]any)["active"] != true {
		t.Fatalf("activate: %d %v", status, body)
	}
	// Exactly one active version survives activation.
	_, body = env.get("/api/v1/admin/notification-templates", bearer(adminAccess))
	active := 0
	for _, item := range body["templates"].([]any) {
		tp := item.(map[string]any)
		if tp["key"] == "booking.confirmed" && tp["active"] == true {
			active++
		}
	}
	if active != 1 {
		t.Fatalf("active versions after activation: %d, want 1", active)
	}

	// Settings policy editor: put + read back, version bumps.
	status, body = env.put("/api/v1/admin/settings/payout_delay_days",
		map[string]any{"value": "10"}, bearer(adminAccess))
	if status != http.StatusOK {
		t.Fatalf("put setting: got %d: %v", status, body)
	}
	var stored string
	if err := env.pool.QueryRow(context.Background(),
		`SELECT value_json #>> '{}' FROM system_settings WHERE key = 'payout_delay_days'`).Scan(&stored); err != nil {
		t.Fatalf("setting lookup: %v", err)
	}
	if stored != "10" {
		t.Fatalf("setting value: %q, want 10", stored)
	}
	// Restore the default so other suites keep their assumptions.
	if _, err := env.pool.Exec(context.Background(),
		`UPDATE system_settings SET value_json = '"7"' WHERE key = 'payout_delay_days'`); err != nil {
		t.Fatalf("setting restore: %v", err)
	}

	// Audit viewer (audit.read — super_admin only).
	if status, _ := env.get("/api/v1/admin/audit-logs", bearer(adminAccess)); status != http.StatusForbidden {
		t.Fatalf("audit viewer without audit.read: got %d, want 403", status)
	}
	superEmail := uniqueEmail(t)
	env.registerAndLogin(superEmail)
	env.grantRole(superEmail, "super_admin")
	superAccess := env.login(superEmail)["access_token"].(string)
	status, body = env.get("/api/v1/admin/audit-logs?action=settings.updated", bearer(superAccess))
	if status != http.StatusOK {
		t.Fatalf("audit viewer: got %d: %v", status, body)
	}
	entries := body["entries"].([]any)
	if len(entries) == 0 {
		t.Fatal("audit viewer: settings.updated entry missing")
	}
	found := false
	for _, item := range entries {
		if item.(map[string]any)["action"] == "settings.updated" {
			found = true
		}
	}
	if !found {
		t.Fatalf("audit entries: %v", entries)
	}
	fmt.Println("audit entries verified:", len(entries))
}
