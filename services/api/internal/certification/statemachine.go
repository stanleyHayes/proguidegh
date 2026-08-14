// Package certification implements the certification & trust pipeline
// (spec §5): an explicit state machine rooted at certification_cases, with
// immutable certification_events history and an audit row per transition.
// No arbitrary status writes exist outside this package's domain service.
package certification

import (
	"sort"
	"strings"
	"time"
)

// Pipeline statuses (spec §5). Stored uppercase; the state machine below is
// the only legal source of transitions.
const (
	StatusApplied                = "APPLIED"
	StatusIdentityPending        = "IDENTITY_PENDING"
	StatusIdentityVerified       = "IDENTITY_VERIFIED"
	StatusBackgroundCheckPending = "BACKGROUND_CHECK_PENDING"
	StatusBackgroundVerified     = "BACKGROUND_VERIFIED"
	StatusTraining               = "TRAINING"
	StatusExamPending            = "EXAM_PENDING"
	StatusCertified              = "CERTIFIED"
	StatusInsuranceActive        = "INSURANCE_ACTIVE"
	StatusActive                 = "ACTIVE"
	StatusRejected               = "REJECTED"
	StatusSuspended              = "SUSPENDED"
	StatusExpired                = "EXPIRED"
	StatusRequiresRetraining     = "REQUIRES_RETRAINING"
)

// transitions is the full legal transition set (spec §5). REJECTED is
// terminal — reapplication opens a new case, preserving history. Reactivation
// from SUSPENDED/EXPIRED/REQUIRES_RETRAINING appends new events to the same
// case; past rows are never updated.
var transitions = map[string][]string{
	StatusApplied:                {StatusIdentityPending, StatusRejected},
	StatusIdentityPending:        {StatusIdentityVerified, StatusRejected},
	StatusIdentityVerified:       {StatusBackgroundCheckPending, StatusRejected},
	StatusBackgroundCheckPending: {StatusBackgroundVerified, StatusRejected},
	StatusBackgroundVerified:     {StatusTraining, StatusRejected},
	StatusTraining:               {StatusExamPending, StatusRejected},
	StatusExamPending:            {StatusCertified, StatusRejected},
	StatusCertified:              {StatusInsuranceActive, StatusSuspended, StatusExpired},
	StatusInsuranceActive:        {StatusActive, StatusSuspended, StatusExpired},
	StatusActive:                 {StatusSuspended, StatusExpired, StatusRequiresRetraining},
	StatusSuspended:              {StatusActive},
	StatusExpired:                {StatusActive},
	StatusRequiresRetraining:     {StatusTraining},
	StatusRejected:               {},
}

// evidenceDocuments maps an evidence-gated target status (spec §5 stage
// table) to the guide_documents types that satisfy its evidence requirement.
// IDENTITY_VERIFIED accepts any government ID type.
var evidenceDocuments = map[string][]string{
	StatusIdentityVerified:   {"national_id", "passport", "drivers_license"},
	StatusBackgroundVerified: {"background_check"},
	StatusCertified:          {"certification"},
	StatusInsuranceActive:    {"insurance"},
}

// stageOrder ranks pipeline stages so "outstanding requirements" can be
// limited to stages at or ahead of the case's current position. Exception
// states map to the stage they fall back to on reactivation.
var stageOrder = map[string]int{
	StatusApplied:                0,
	StatusIdentityPending:        1,
	StatusIdentityVerified:       2,
	StatusBackgroundCheckPending: 3,
	StatusBackgroundVerified:     4,
	StatusTraining:               5,
	StatusExamPending:            6,
	StatusCertified:              7,
	StatusInsuranceActive:        8,
	StatusActive:                 9,
	StatusRejected:               99,
	StatusSuspended:              9,
	StatusExpired:                9,
	StatusRequiresRetraining:     5,
}

// NormalizeStatus uppercases and trims a client-supplied status.
func NormalizeStatus(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

// ValidStatus reports whether s is a known pipeline status.
func ValidStatus(s string) bool {
	_, ok := transitions[s]
	return ok
}

// Statuses returns every known status, sorted (for API documentation and
// filter validation).
func Statuses() []string {
	out := make([]string, 0, len(transitions))
	for s := range transitions {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// CanTransition reports whether from → to is a legal transition.
func CanTransition(from, to string) bool {
	for _, t := range transitions[from] {
		if t == to {
			return true
		}
	}
	return false
}

// EvidenceRequired reports whether transitioning into status demands evidence
// (spec §5 stage table): a non-empty evidence_ref plus a valid document of
// the mapped type.
func EvidenceRequired(status string) bool {
	_, ok := evidenceDocuments[status]
	return ok
}

// EvidenceDocTypes returns the document types that satisfy the evidence
// requirement for status (nil when the stage needs no document).
func EvidenceDocTypes(status string) []string { return evidenceDocuments[status] }

// DocInput is the document metadata the evidence checks run against.
type DocInput struct {
	Type      string
	Status    string
	ExpiresAt *time.Time
}

// docUsable reports whether a document counts as evidence: present, not
// rejected/expired by status, and not past its expires_at.
func docUsable(d DocInput, at time.Time) bool {
	if d.Status == "rejected" || d.Status == "expired" {
		return false
	}
	return d.ExpiresAt == nil || d.ExpiresAt.After(at)
}

// hasEvidenceDoc reports whether docs contain a usable document of any of the
// given types.
func hasEvidenceDoc(docs []DocInput, types []string, at time.Time) bool {
	for _, d := range docs {
		if !docUsable(d, at) {
			continue
		}
		for _, t := range types {
			if d.Type == t {
				return true
			}
		}
	}
	return false
}

// EvidenceSatisfied reports whether the evidence requirement for a target
// status is met by the guide's documents.
func EvidenceSatisfied(status string, docs []DocInput, at time.Time) bool {
	types, ok := evidenceDocuments[status]
	if !ok {
		return true
	}
	return hasEvidenceDoc(docs, types, at)
}

// MandatoryDocGroups are the document groups a guide must hold, usable and
// unexpired, to satisfy the §10.2 mandatory-document rule. Each group is one
// requirement: any of its types satisfies it.
var MandatoryDocGroups = [][]string{
	{"national_id", "passport", "drivers_license"},
	{"background_check"},
	{"certification"},
	{"insurance"},
}

// MissingMandatoryDocs returns the requirement groups (as "type1|type2"
// strings) with no usable document at the given time. An empty result means
// the guide's mandatory documents are valid.
func MissingMandatoryDocs(docs []DocInput, at time.Time) []string {
	var missing []string
	for _, group := range MandatoryDocGroups {
		if !hasEvidenceDoc(docs, group, at) {
			missing = append(missing, strings.Join(group, "|"))
		}
	}
	return missing
}

// requirementLabels turns an evidence doc group into a human-facing
// outstanding-requirement string.
var requirementLabels = map[string]string{
	"national_id|passport|drivers_license": "upload a government ID document (national_id, passport or drivers_license)",
	"background_check":                     "upload a background check document",
	"certification":                        "upload a certification document",
	"insurance":                            "upload an insurance policy document",
}

// OutstandingRequirements lists what the guide still must supply, limited to
// stages at or ahead of the case's current status. Terminal/inactive states
// return no document requirements (the case status itself is the blocker).
func OutstandingRequirements(caseStatus string, docs []DocInput, at time.Time) []string {
	switch caseStatus {
	case StatusRejected, StatusSuspended, StatusExpired, StatusRequiresRetraining, StatusActive:
		return []string{}
	}
	current, ok := stageOrder[caseStatus]
	if !ok {
		return []string{}
	}
	var out []string
	for status, types := range evidenceDocuments {
		if stageOrder[status] < current {
			continue
		}
		if hasEvidenceDoc(docs, types, at) {
			continue
		}
		out = append(out, requirementLabels[strings.Join(types, "|")])
	}
	sort.Strings(out)
	return out
}
