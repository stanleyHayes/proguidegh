package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// doJSON issues a JSON request with an arbitrary method and returns
// status + decoded body.
func (e *integrationEnv) doJSON(method, path string, body any, headers map[string]string) (int, map[string]any) {
	e.t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			e.t.Fatalf("marshal: %v", err)
		}
	}
	req, err := http.NewRequest(method, e.server.URL+path, bytes.NewReader(raw))
	if err != nil {
		e.t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := e.server.Client().Do(req)
	if err != nil {
		e.t.Fatalf("do %s %s: %v", method, path, err)
	}
	defer resp.Body.Close() //nolint:errcheck
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func errCode(body map[string]any) string {
	if e, ok := body["error"].(map[string]any); ok {
		c, _ := e["code"].(string)
		return c
	}
	return ""
}

// TestCertificationPipelineJourney exercises the full Phase 2 flow: apply
// opens a case in APPLIED, a verifier walks the pipeline to ACTIVE with
// evidence-gated stages, the guide becomes publicly visible, suspension
// hides them, and reactivation preserves the event history.
func TestCertificationPipelineJourney(t *testing.T) {
	env := newIntegrationEnv(t)

	// --- Guide applies; case opens in APPLIED -------------------------------
	guideEmail := uniqueEmail(t)
	guideTokens := env.registerAndLogin(guideEmail)
	guideAccess, _ := guideTokens["access_token"].(string)

	status, body := env.post("/api/v1/guides/apply", map[string]any{"public_name": "Ama Pipeline"}, bearer(guideAccess))
	if status != http.StatusCreated {
		t.Fatalf("apply: got %d: %v", status, body)
	}
	profile, _ := body["guide_profile"].(map[string]any)
	guideID, _ := profile["user_id"].(string)
	kase, _ := body["certification_case"].(map[string]any)
	caseID, _ := kase["id"].(string)
	if guideID == "" || caseID == "" {
		t.Fatalf("apply response missing profile/case: %v", body)
	}
	if kase["status"] != "APPLIED" {
		t.Fatalf("new case status = %v, want APPLIED", kase["status"])
	}

	// Dashboard aggregate: case + outstanding requirements + documents.
	status, body = env.get("/api/v1/me/guide", bearer(guideAccess))
	if status != http.StatusOK {
		t.Fatalf("me/guide: got %d: %v", status, body)
	}
	cert, _ := body["certification"].(map[string]any)
	if cert["status"] != "APPLIED" {
		t.Fatalf("me/guide certification = %v, want APPLIED", cert)
	}
	outstanding, _ := body["outstanding_requirements"].([]any)
	if len(outstanding) != 4 {
		t.Fatalf("outstanding requirements = %v, want 4", outstanding)
	}

	// Pipeline detail: opening event only.
	status, body = env.get("/api/v1/me/guide/certification", bearer(guideAccess))
	if status != http.StatusOK {
		t.Fatalf("me/guide/certification: got %d: %v", status, body)
	}
	events, _ := body["events"].([]any)
	if len(events) != 1 {
		t.Fatalf("events after apply = %d, want 1 (opening)", len(events))
	}

	// --- Guide profile patch: languages + region + specialties --------------
	status, body = env.get("/api/v1/regions", nil)
	if status != http.StatusOK {
		t.Fatalf("regions: got %d", status)
	}
	regions, _ := body["regions"].([]any)
	if len(regions) != 16 {
		t.Fatalf("regions = %d, want 16", len(regions))
	}
	var accraID string
	for _, r := range regions {
		rm, _ := r.(map[string]any)
		if rm["code"] == "AA" {
			accraID, _ = rm["id"].(string)
		}
	}
	if accraID == "" {
		t.Fatalf("Greater Accra region not found: %v", regions)
	}

	status, body = env.get("/api/v1/specialties", nil)
	if status != http.StatusOK {
		t.Fatalf("specialties: got %d", status)
	}
	specialties, _ := body["specialties"].([]any)
	if len(specialties) != 13 {
		t.Fatalf("specialties = %d, want 13 (Appendix C)", len(specialties))
	}
	specID, _ := specialties[0].(map[string]any)["id"].(string)

	status, body = env.doJSON(http.MethodPatch, "/api/v1/me/guide/profile", map[string]any{
		"bio":           "Accra-born storyteller.",
		"region_id":     accraID,
		"languages":     []map[string]any{{"code": "en", "proficiency": "native"}, {"code": "tw", "proficiency": "fluent"}},
		"specialty_ids": []string{specID},
	}, bearer(guideAccess))
	if status != http.StatusOK {
		t.Fatalf("patch profile: got %d: %v", status, body)
	}
	if langs, _ := body["languages"].([]any); len(langs) != 2 {
		t.Fatalf("patched languages = %v, want 2", langs)
	}
	// Unknown language code is rejected.
	status, body = env.doJSON(http.MethodPatch, "/api/v1/me/guide/profile", map[string]any{
		"languages": []map[string]any{{"code": "xx", "proficiency": "native"}},
	}, bearer(guideAccess))
	if status != http.StatusBadRequest {
		t.Fatalf("patch unknown language: got %d, want 400: %v", status, body)
	}

	// --- Verifier works the queue -------------------------------------------
	verifierEmail := uniqueEmail(t)
	env.registerAndLogin(verifierEmail)
	env.grantRole(verifierEmail, "verifier")
	verifierTokens := env.login(verifierEmail)
	verifierAccess, _ := verifierTokens["access_token"].(string)

	// The guide has no staff permissions.
	status, _ = env.get("/api/v1/admin/certification/queue", bearer(guideAccess))
	if status != http.StatusForbidden {
		t.Fatalf("queue as guide: got %d, want 403", status)
	}

	status, body = env.get("/api/v1/admin/certification/queue?status=APPLIED", bearer(verifierAccess))
	if status != http.StatusOK {
		t.Fatalf("queue: got %d: %v", status, body)
	}
	cases, _ := body["cases"].([]any)
	found := false
	for _, c := range cases {
		if cm, _ := c.(map[string]any); cm["id"] == caseID {
			found = true
		}
	}
	if !found {
		t.Fatalf("case %s not in APPLIED queue: %v", caseID, cases)
	}

	status, body = env.get("/api/v1/admin/certification/"+caseID, bearer(verifierAccess))
	if status != http.StatusOK {
		t.Fatalf("case detail: got %d: %v", status, body)
	}

	transition := func(to, reason, evidenceRef string) (int, map[string]any) {
		payload := map[string]any{"to_status": to, "reason": reason}
		if evidenceRef != "" {
			payload["evidence_ref"] = evidenceRef
		}
		return env.post("/api/v1/admin/certification/"+caseID+"/transition", payload, bearer(verifierAccess))
	}
	uploadDoc := func(docType string) {
		e := env
		st, b := e.post("/api/v1/guides/documents", map[string]any{
			"type": docType, "content_type": "image/png",
		}, bearer(guideAccess))
		if st != http.StatusCreated {
			e.t.Fatalf("upload %s: got %d: %v", docType, st, b)
		}
	}

	// Non-evidence stage: straight through.
	if status, body = transition("IDENTITY_PENDING", "picked up for review", ""); status != http.StatusOK {
		t.Fatalf("->IDENTITY_PENDING: got %d: %v", status, body)
	}

	// Illegal jump: IDENTITY_PENDING -> CERTIFIED is not an edge.
	status, body = transition("CERTIFIED", "skip everything", "cert-1")
	if status != http.StatusConflict || errCode(body) != "ILLEGAL_TRANSITION" {
		t.Fatalf("illegal transition: got %d %v, want 409 ILLEGAL_TRANSITION", status, body)
	}

	// Evidence stage without evidence_ref -> 422.
	status, body = transition("IDENTITY_VERIFIED", "checked", "")
	if status != http.StatusUnprocessableEntity || errCode(body) != "EVIDENCE_REQUIRED" {
		t.Fatalf("missing evidence_ref: got %d %v, want 422 EVIDENCE_REQUIRED", status, body)
	}

	// Evidence_ref but no valid ID document -> 422.
	status, body = transition("IDENTITY_VERIFIED", "checked", "id-ref-1")
	if status != http.StatusUnprocessableEntity || errCode(body) != "EVIDENCE_REQUIRED" {
		t.Fatalf("missing ID document: got %d %v, want 422 EVIDENCE_REQUIRED", status, body)
	}

	// Upload the ID document; now the transition passes.
	uploadDoc("national_id")
	if status, body = transition("IDENTITY_VERIFIED", "ID verified", "id-ref-1"); status != http.StatusOK {
		t.Fatalf("->IDENTITY_VERIFIED: got %d: %v", status, body)
	}

	// Walk the remaining stages, uploading evidence where required.
	if status, body = transition("BACKGROUND_CHECK_PENDING", "police check requested", ""); status != http.StatusOK {
		t.Fatalf("->BACKGROUND_CHECK_PENDING: got %d: %v", status, body)
	}
	uploadDoc("background_check")
	if status, body = transition("BACKGROUND_VERIFIED", "police clearance verified", "bg-ref-1"); status != http.StatusOK {
		t.Fatalf("->BACKGROUND_VERIFIED: got %d: %v", status, body)
	}
	if status, body = transition("TRAINING", "enrolled in core modules", ""); status != http.StatusOK {
		t.Fatalf("->TRAINING: got %d: %v", status, body)
	}
	if status, body = transition("EXAM_PENDING", "modules complete", ""); status != http.StatusOK {
		t.Fatalf("->EXAM_PENDING: got %d: %v", status, body)
	}
	uploadDoc("certification")
	if status, body = transition("CERTIFIED", "exam passed", "cert-no-123"); status != http.StatusOK {
		t.Fatalf("->CERTIFIED: got %d: %v", status, body)
	}
	uploadDoc("insurance")
	if status, body = transition("INSURANCE_ACTIVE", "policy confirmed", "policy-xyz"); status != http.StatusOK {
		t.Fatalf("->INSURANCE_ACTIVE: got %d: %v", status, body)
	}
	if status, body = transition("ACTIVE", "all controls valid", ""); status != http.StatusOK {
		t.Fatalf("->ACTIVE: got %d: %v", status, body)
	}

	// --- Public visibility (§10.2) -------------------------------------------
	status, body = env.get("/api/v1/guides/"+guideID, nil)
	if status != http.StatusOK {
		t.Fatalf("public guide detail: got %d: %v", status, body)
	}
	g, _ := body["guide"].(map[string]any)
	if g["public_name"] != "Ama Pipeline" {
		t.Fatalf("public detail: %v", g)
	}

	// Suspend: hidden from the public catalog.
	if status, body = transition("SUSPENDED", "safety investigation", ""); status != http.StatusOK {
		t.Fatalf("->SUSPENDED: got %d: %v", status, body)
	}
	status, _ = env.get("/api/v1/guides/"+guideID, nil)
	if status != http.StatusNotFound {
		t.Fatalf("suspended guide public detail: got %d, want 404", status)
	}

	// Reactivate: back to ACTIVE, history preserved.
	if status, body = transition("ACTIVE", "investigation cleared", ""); status != http.StatusOK {
		t.Fatalf("reactivate: got %d: %v", status, body)
	}
	status, _ = env.get("/api/v1/guides/"+guideID, nil)
	if status != http.StatusOK {
		t.Fatalf("reactivated guide public detail: got %d, want 200", status)
	}
	status, body = env.get("/api/v1/me/guide/certification", bearer(guideAccess))
	if status != http.StatusOK {
		t.Fatalf("certification detail: got %d", status)
	}
	events, _ = body["events"].([]any)
	// 1 opening + 11 transitions (9 forward, suspend, reactivate).
	if len(events) != 12 {
		t.Fatalf("events after reactivation = %d, want 12 (history preserved)", len(events))
	}
	first, _ := events[0].(map[string]any)
	if first["from_status"] != nil || first["to_status"] != "APPLIED" {
		t.Fatalf("opening event malformed: %v", first)
	}
	last, _ := events[len(events)-1].(map[string]any)
	if last["from_status"] != "SUSPENDED" || last["to_status"] != "ACTIVE" {
		t.Fatalf("reactivation event malformed: %v", last)
	}

	// Every transition wrote an audit row.
	var auditCount int
	if err := env.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_logs
		WHERE action = 'certification.transition' AND entity_id = $1`, caseID).Scan(&auditCount); err != nil {
		t.Fatalf("audit lookup: %v", err)
	}
	if auditCount != 11 {
		t.Fatalf("transition audit rows = %d, want 11", auditCount)
	}

	// Unknown case id and unknown status are clean 404/400.
	status, _ = env.get("/api/v1/admin/certification/00000000-0000-0000-0000-000000000000", bearer(verifierAccess))
	if status != http.StatusNotFound {
		t.Fatalf("unknown case: got %d, want 404", status)
	}
	status, body = transition("NOPE", "bad", "")
	if status != http.StatusBadRequest {
		t.Fatalf("unknown to_status: got %d, want 400: %v", status, body)
	}
}

// TestPublicCatalog verifies the seeded reference data and effective pricing
// (spec §27) over HTTP.
func TestPublicCatalog(t *testing.T) {
	env := newIntegrationEnv(t)

	status, body := env.get("/api/v1/tour-packages", nil)
	if status != http.StatusOK {
		t.Fatalf("tour-packages: got %d: %v", status, body)
	}
	packages, _ := body["packages"].([]any)
	if len(packages) != 3 {
		t.Fatalf("packages = %d, want 3", len(packages))
	}
	want := map[string]string{
		"CITY_TOUR_4H":     "250.00",
		"HERITAGE_TOUR_8H": "450.00",
		"MULTI_REGION_24H": "900.00",
	}
	for _, p := range packages {
		pm, _ := p.(map[string]any)
		code, _ := pm["code"].(string)
		price, _ := pm["price"].(string)
		if want[code] == "" {
			t.Fatalf("unexpected package %s", code)
		}
		// NUMERIC may serialize as 250.00 or 250.0; compare trimmed.
		if !strings.HasPrefix(strings.TrimRight(price, "0"), strings.TrimRight(want[code], "0")) {
			t.Fatalf("%s price = %s, want %s", code, price, want[code])
		}
		if dur, _ := pm["duration_minutes"].(float64); dur <= 0 {
			t.Fatalf("%s duration = %v", code, pm["duration_minutes"])
		}
	}

	// Unauthenticated access is fine — this is reference data.
	status, body = env.get("/api/v1/regions", nil)
	if status != http.StatusOK {
		t.Fatalf("regions: got %d", status)
	}
	if regions, _ := body["regions"].([]any); len(regions) != 16 {
		t.Fatalf("regions = %d, want 16", len(regions))
	}
	status, body = env.get("/api/v1/specialties", nil)
	if status != http.StatusOK {
		t.Fatalf("specialties: got %d", status)
	}
	if specialties, _ := body["specialties"].([]any); len(specialties) != 13 {
		t.Fatalf("specialties = %d, want 13", len(specialties))
	}
}

// TestGuideVisibilityGates covers the negative paths of the §10.2 gate and
// self-service scoping without driving the full pipeline.
func TestGuideVisibilityGates(t *testing.T) {
	env := newIntegrationEnv(t)

	// Unknown guide id: 404.
	status, _ := env.get("/api/v1/guides/00000000-0000-0000-0000-000000000000", nil)
	if status != http.StatusNotFound {
		t.Fatalf("unknown guide: got %d, want 404", status)
	}

	// An applicant whose case has not reached ACTIVE is invisible.
	email := uniqueEmail(t)
	tokens := env.registerAndLogin(email)
	access, _ := tokens["access_token"].(string)
	status, body := env.post("/api/v1/guides/apply", map[string]any{"public_name": "Hidden Guide"}, bearer(access))
	if status != http.StatusCreated {
		t.Fatalf("apply: got %d: %v", status, body)
	}
	guideID, _ := body["guide_profile"].(map[string]any)["user_id"].(string)
	status, _ = env.get("/api/v1/guides/"+guideID, nil)
	if status != http.StatusNotFound {
		t.Fatalf("APPLIED guide public detail: got %d, want 404", status)
	}

	// me/guide requires auth and a profile.
	status, _ = env.get("/api/v1/me/guide", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("me/guide unauthenticated: got %d, want 401", status)
	}
	otherEmail := uniqueEmail(t)
	otherTokens := env.registerAndLogin(otherEmail)
	otherAccess, _ := otherTokens["access_token"].(string)
	status, body = env.get("/api/v1/me/guide", bearer(otherAccess))
	if status != http.StatusNotFound || errCode(body) != "NO_GUIDE_PROFILE" {
		t.Fatalf("me/guide without profile: got %d %v, want 404 NO_GUIDE_PROFILE", status, body)
	}
	status, body = env.get("/api/v1/me/guide/certification", bearer(otherAccess))
	if status != http.StatusNotFound || errCode(body) != "NO_CERTIFICATION_CASE" {
		t.Fatalf("me/guide/certification without case: got %d %v, want 404", status, body)
	}
}
