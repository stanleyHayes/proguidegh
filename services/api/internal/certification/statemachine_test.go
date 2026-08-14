package certification

import (
	"testing"
	"time"
)

// legalTransitions is the full spec §5 edge set; the test asserts every one
// of them is accepted and every sampled illegal jump is rejected.
var legalTransitions = [][2]string{
	{StatusApplied, StatusIdentityPending},
	{StatusIdentityPending, StatusIdentityVerified},
	{StatusIdentityVerified, StatusBackgroundCheckPending},
	{StatusBackgroundCheckPending, StatusBackgroundVerified},
	{StatusBackgroundVerified, StatusTraining},
	{StatusTraining, StatusExamPending},
	{StatusExamPending, StatusCertified},
	{StatusCertified, StatusInsuranceActive},
	{StatusInsuranceActive, StatusActive},
	// Rejections during review.
	{StatusApplied, StatusRejected},
	{StatusExamPending, StatusRejected},
	// Exceptions and reactivation (history preserved via events).
	{StatusActive, StatusSuspended},
	{StatusActive, StatusExpired},
	{StatusActive, StatusRequiresRetraining},
	{StatusCertified, StatusSuspended},
	{StatusInsuranceActive, StatusExpired},
	{StatusSuspended, StatusActive},
	{StatusExpired, StatusActive},
	{StatusRequiresRetraining, StatusTraining},
}

func TestLegalTransitions(t *testing.T) {
	for _, tr := range legalTransitions {
		if !CanTransition(tr[0], tr[1]) {
			t.Errorf("expected legal transition %s -> %s", tr[0], tr[1])
		}
	}
}

func TestIllegalTransitions(t *testing.T) {
	illegal := [][2]string{
		// Skipping stages.
		{StatusApplied, StatusIdentityVerified},
		{StatusApplied, StatusActive},
		{StatusIdentityPending, StatusCertified},
		{StatusTraining, StatusCertified},
		{StatusExamPending, StatusActive},
		{StatusCertified, StatusActive}, // insurance stage cannot be skipped
		// Backwards moves.
		{StatusActive, StatusInsuranceActive},
		{StatusIdentityVerified, StatusApplied},
		// Terminal/exception misuse.
		{StatusRejected, StatusApplied},
		{StatusRejected, StatusActive},
		{StatusSuspended, StatusTraining},
		{StatusExpired, StatusCertified},
		{StatusRequiresRetraining, StatusActive},
		// Unknown statuses.
		{"", StatusActive},
		{StatusApplied, "NOPE"},
		{"submitted", StatusActive}, // Phase 0 placeholder set is gone
	}
	for _, tr := range illegal {
		if CanTransition(tr[0], tr[1]) {
			t.Errorf("expected illegal transition %s -> %s", tr[0], tr[1])
		}
	}
}

func TestEvidenceRequiredStages(t *testing.T) {
	required := []string{StatusIdentityVerified, StatusBackgroundVerified, StatusCertified, StatusInsuranceActive}
	for _, s := range required {
		if !EvidenceRequired(s) {
			t.Errorf("expected evidence required for %s", s)
		}
		if len(EvidenceDocTypes(s)) == 0 {
			t.Errorf("expected doc types for %s", s)
		}
	}
	free := []string{StatusApplied, StatusIdentityPending, StatusTraining, StatusExamPending,
		StatusActive, StatusRejected, StatusSuspended, StatusExpired, StatusRequiresRetraining}
	for _, s := range free {
		if EvidenceRequired(s) {
			t.Errorf("expected no evidence requirement for %s", s)
		}
	}
}

func TestEvidenceSatisfied(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	cases := []struct {
		name   string
		status string
		docs   []DocInput
		want   bool
	}{
		{"id satisfied", StatusIdentityVerified,
			[]DocInput{{Type: "national_id", Status: "uploaded"}}, true},
		{"id accepts passport", StatusIdentityVerified,
			[]DocInput{{Type: "passport", Status: "approved"}}, true},
		{"wrong doc type", StatusIdentityVerified,
			[]DocInput{{Type: "insurance", Status: "uploaded"}}, false},
		{"no docs", StatusBackgroundVerified, nil, false},
		{"rejected doc does not count", StatusBackgroundVerified,
			[]DocInput{{Type: "background_check", Status: "rejected"}}, false},
		{"expired status does not count", StatusCertified,
			[]DocInput{{Type: "certification", Status: "expired"}}, false},
		{"past expires_at does not count", StatusInsuranceActive,
			[]DocInput{{Type: "insurance", Status: "uploaded", ExpiresAt: &past}}, false},
		{"future expires_at counts", StatusInsuranceActive,
			[]DocInput{{Type: "insurance", Status: "uploaded", ExpiresAt: &future}}, true},
		{"non-evidence stage always satisfied", StatusTraining, nil, true},
	}
	for _, tc := range cases {
		if got := EvidenceSatisfied(tc.status, tc.docs, now); got != tc.want {
			t.Errorf("%s: EvidenceSatisfied(%s) = %v, want %v", tc.name, tc.status, got, tc.want)
		}
	}
}

func TestMissingMandatoryDocs(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Hour)

	all := []DocInput{
		{Type: "national_id", Status: "approved"},
		{Type: "background_check", Status: "uploaded"},
		{Type: "certification", Status: "approved"},
		{Type: "insurance", Status: "uploaded"},
	}
	if got := MissingMandatoryDocs(all, now); len(got) != 0 {
		t.Fatalf("complete docs: got missing %v", got)
	}

	if got := MissingMandatoryDocs(nil, now); len(got) != 4 {
		t.Fatalf("no docs: got %v, want 4 missing groups", got)
	}

	expiredInsurance := []DocInput{
		{Type: "national_id", Status: "approved"},
		{Type: "background_check", Status: "uploaded"},
		{Type: "certification", Status: "approved"},
		{Type: "insurance", Status: "uploaded", ExpiresAt: &past},
	}
	got := MissingMandatoryDocs(expiredInsurance, now)
	if len(got) != 1 || got[0] != "insurance" {
		t.Fatalf("expired insurance: got %v, want [insurance]", got)
	}
}

func TestOutstandingRequirements(t *testing.T) {
	now := time.Now()

	// Fresh applicant with no documents: every evidence stage is outstanding.
	got := OutstandingRequirements(StatusApplied, nil, now)
	if len(got) != 4 {
		t.Fatalf("APPLIED no docs: got %v, want 4 requirements", got)
	}

	// After identity verification with ID + background docs present, only
	// certification and insurance remain.
	docs := []DocInput{
		{Type: "national_id", Status: "approved"},
		{Type: "background_check", Status: "uploaded"},
	}
	got = OutstandingRequirements(StatusBackgroundVerified, docs, now)
	if len(got) != 2 {
		t.Fatalf("BACKGROUND_VERIFIED: got %v, want 2 requirements", got)
	}

	// Terminal and exception states report no document requirements.
	for _, s := range []string{StatusActive, StatusRejected, StatusSuspended, StatusExpired, StatusRequiresRetraining} {
		if got := OutstandingRequirements(s, nil, now); len(got) != 0 {
			t.Errorf("%s: got %v, want no requirements", s, got)
		}
	}
}

func TestNormalizeAndValidateStatus(t *testing.T) {
	if got := NormalizeStatus(" active "); got != StatusActive {
		t.Fatalf("NormalizeStatus: got %q", got)
	}
	if !ValidStatus(StatusActive) || ValidStatus("submitted") || ValidStatus("") {
		t.Fatal("ValidStatus misclassified")
	}
	if len(Statuses()) != 14 {
		t.Fatalf("Statuses: got %d, want 14", len(Statuses()))
	}
}
