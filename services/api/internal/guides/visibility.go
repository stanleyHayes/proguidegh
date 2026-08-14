package guides

import "proguidegh/api/internal/certification"

// VisibilityInput carries the §10.2 public-visibility signals for a guide.
type VisibilityInput struct {
	// CaseStatus is the guide's current certification case status ("" when
	// the guide has never applied).
	CaseStatus string
	// UserStatus is the account status (users.status).
	UserStatus string
	// GuideStatus is the guide_profiles status.
	GuideStatus string
	// DocumentsValid reports whether all mandatory document groups are
	// satisfied by usable, unexpired documents.
	DocumentsValid bool
}

// PubliclyVisible applies the §10.2 availability gate: a guide is publicly
// visible (and searchable) only with ACTIVE certification, an unsuspended
// account and valid mandatory documents.
func PubliclyVisible(v VisibilityInput) bool {
	if v.CaseStatus != certification.StatusActive {
		return false
	}
	if v.UserStatus != "active" {
		return false
	}
	if v.GuideStatus == "suspended" || v.GuideStatus == "disabled" {
		return false
	}
	return v.DocumentsValid
}
