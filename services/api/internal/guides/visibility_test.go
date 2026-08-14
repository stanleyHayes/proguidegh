package guides

import (
	"testing"

	"proguidegh/api/internal/certification"
)

// TestPubliclyVisible covers the §10.2 gate truth table: ACTIVE certification
// + unsuspended account + valid mandatory documents, all three required.
func TestPubliclyVisible(t *testing.T) {
	visible := VisibilityInput{
		CaseStatus:     certification.StatusActive,
		UserStatus:     "active",
		GuideStatus:    "certified",
		DocumentsValid: true,
	}
	if !PubliclyVisible(visible) {
		t.Fatal("fully eligible guide must be visible")
	}

	cases := []struct {
		name string
		mut  func(v *VisibilityInput)
	}{
		{"no case", func(v *VisibilityInput) { v.CaseStatus = "" }},
		{"case in progress", func(v *VisibilityInput) { v.CaseStatus = certification.StatusTraining }},
		{"case certified but not active", func(v *VisibilityInput) { v.CaseStatus = certification.StatusCertified }},
		{"case suspended", func(v *VisibilityInput) { v.CaseStatus = certification.StatusSuspended }},
		{"case expired", func(v *VisibilityInput) { v.CaseStatus = certification.StatusExpired }},
		{"case rejected", func(v *VisibilityInput) { v.CaseStatus = certification.StatusRejected }},
		{"requires retraining", func(v *VisibilityInput) { v.CaseStatus = certification.StatusRequiresRetraining }},
		{"account suspended", func(v *VisibilityInput) { v.UserStatus = "suspended" }},
		{"account deactivated", func(v *VisibilityInput) { v.UserStatus = "deactivated" }},
		{"guide profile suspended", func(v *VisibilityInput) { v.GuideStatus = "suspended" }},
		{"guide profile disabled", func(v *VisibilityInput) { v.GuideStatus = "disabled" }},
		{"mandatory docs expired", func(v *VisibilityInput) { v.DocumentsValid = false }},
	}
	for _, tc := range cases {
		v := visible
		tc.mut(&v)
		if PubliclyVisible(v) {
			t.Errorf("%s: must not be visible", tc.name)
		}
	}
}
