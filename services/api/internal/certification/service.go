package certification

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Service is the single domain service allowed to move a certification case
// through the state machine (spec §5, AGENTS.md §3): every status write goes
// through Service.Transition.
type Service struct {
	repo *Repository
	now  func() time.Time // injectable for tests
}

// NewService builds the service.
func NewService(repo *Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

// TransitionParams is one requested state machine move.
type TransitionParams struct {
	CaseID      string
	ActorID     string
	ToStatus    string
	Reason      string
	EvidenceRef string
	IP          string
}

// TransitionResult pairs the updated case with its immutable event row.
type TransitionResult struct {
	Case  Case  `json:"case"`
	Event Event `json:"event"`
}

// Transition validates and applies a state machine move. Errors:
// ErrUnknownStatus (400), ErrCaseNotFound (404), ErrEvidenceRequired (422),
// ErrIllegalTransition (409).
func (s *Service) Transition(ctx context.Context, p TransitionParams) (TransitionResult, error) {
	p.ToStatus = NormalizeStatus(p.ToStatus)
	p.Reason = strings.TrimSpace(p.Reason)
	p.EvidenceRef = strings.TrimSpace(p.EvidenceRef)

	if !ValidStatus(p.ToStatus) {
		return TransitionResult{}, ErrUnknownStatus
	}
	if p.Reason == "" {
		return TransitionResult{}, ErrReasonRequired
	}
	if EvidenceRequired(p.ToStatus) && p.EvidenceRef == "" {
		return TransitionResult{}, ErrEvidenceRequired
	}

	c, e, err := s.repo.Transition(ctx, transitionInput{
		CaseID:      p.CaseID,
		ActorID:     p.ActorID,
		ToStatus:    p.ToStatus,
		Reason:      p.Reason,
		EvidenceRef: p.EvidenceRef,
		IP:          p.IP,
	})
	if err != nil {
		return TransitionResult{}, err
	}
	return TransitionResult{Case: c, Event: e}, nil
}

// OpenCase opens (or returns) the guide's certification case in APPLIED.
func (s *Service) OpenCase(ctx context.Context, guideID, actorID string) (Case, error) {
	return s.repo.OpenCase(ctx, guideID, actorID)
}

// CurrentCase returns the guide's latest case (ErrCaseNotFound when none).
func (s *Service) CurrentCase(ctx context.Context, guideID string) (Case, error) {
	return s.repo.CurrentCase(ctx, guideID)
}

// Events returns a case's immutable event history, oldest first.
func (s *Service) Events(ctx context.Context, caseID string) ([]Event, error) {
	return s.repo.ListEvents(ctx, caseID)
}

// Documents returns a guide's document metadata, newest first.
func (s *Service) Documents(ctx context.Context, guideID string) ([]Document, error) {
	return s.repo.ListDocuments(ctx, guideID)
}

// DocumentsValid reports whether every mandatory document group (spec §10.2)
// is satisfied by a usable, unexpired document. The second return lists the
// missing groups — used both by the §10.2 visibility gate and by pipeline
// evidence checks.
func (s *Service) DocumentsValid(ctx context.Context, guideID string) (bool, []string, error) {
	docs, err := s.repo.ListDocuments(ctx, guideID)
	if err != nil {
		return false, nil, err
	}
	inputs := make([]DocInput, 0, len(docs))
	for _, d := range docs {
		inputs = append(inputs, DocInput{Type: d.Type, Status: d.Status, ExpiresAt: d.ExpiresAt})
	}
	missing := MissingMandatoryDocs(inputs, s.now())
	return len(missing) == 0, missing, nil
}

// Outstanding lists the guide's outstanding requirements given its current
// case status (spec §4.2: track pipeline stage and outstanding requirements).
func (s *Service) Outstanding(ctx context.Context, guideID, caseStatus string) ([]string, error) {
	docs, err := s.repo.ListDocuments(ctx, guideID)
	if err != nil {
		return nil, err
	}
	inputs := make([]DocInput, 0, len(docs))
	for _, d := range docs {
		inputs = append(inputs, DocInput{Type: d.Type, Status: d.Status, ExpiresAt: d.ExpiresAt})
	}
	return OutstandingRequirements(caseStatus, inputs, s.now()), nil
}

// ErrReasonRequired is returned when a transition carries no reason.
var ErrReasonRequired = errors.New("certification: reason is required")
