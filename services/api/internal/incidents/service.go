package incidents

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"proguidegh/api/internal/platform/audit"
	"proguidegh/api/internal/realtime"
)

// severities in escalation order (spec §12: HIGH/CRITICAL for SOS).
var severityOrder = []string{"low", "medium", "high", "critical"}

// allowedTransitions is the incident workflow state machine. Reopening a
// resolved incident is permitted; closed is terminal.
var allowedTransitions = map[string][]string{
	"open":         {"acknowledged", "in_progress", "resolved", "closed"},
	"acknowledged": {"in_progress", "resolved", "closed"},
	"in_progress":  {"resolved", "closed"},
	"resolved":     {"open", "closed"},
	"closed":       {},
}

// Service is the incident workflow application service.
type Service struct {
	repo   *Repository
	safety safetyAcknowledger
	hub    *realtime.Hub
	audit  *audit.Recorder
}

// safetyAcknowledger is the safety package's SOS acknowledgement hook,
// kept behind an interface so incidents does not import safety (and tests
// can stub it).
type safetyAcknowledger interface {
	AcknowledgeBooking(ctx context.Context, bookingID string) error
}

// NewService builds the service. hub/audit/safety may be nil in tests.
func NewService(repo *Repository, safety safetyAcknowledger, hub *realtime.Hub, auditor *audit.Recorder) *Service {
	return &Service{repo: repo, safety: safety, hub: hub, audit: auditor}
}

// List returns the filtered incident page.
func (s *Service) List(ctx context.Context, f ListFilter, limit, offset int) ([]Incident, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.List(ctx, f, limit, offset)
}

// Detail returns one incident with its full audit trail.
func (s *Service) Detail(ctx context.Context, id string) (Incident, []Event, error) {
	inc, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return Incident{}, nil, err
	}
	events, err := s.repo.ListEvents(ctx, id)
	if err != nil {
		return Incident{}, nil, err
	}
	return inc, events, nil
}

func (s *Service) transition(ctx context.Context, id, actorID, toStatus, kind string, body *string) (Incident, error) {
	inc, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return Incident{}, err
	}
	allowed := false
	for _, s := range allowedTransitions[inc.Status] {
		if s == toStatus {
			allowed = true
			break
		}
	}
	if !allowed {
		return Incident{}, fmt.Errorf("%w: %s -> %s", ErrIllegalTransition, inc.Status, toStatus)
	}
	return s.apply(ctx, inc, actorID, &toStatus, nil, nil, kind, body)
}

// Acknowledge moves open -> acknowledged and, for SOS incidents, marks the
// underlying SOS events acknowledged (§12 step 9).
func (s *Service) Acknowledge(ctx context.Context, id, actorID string) (Incident, error) {
	inc, err := s.transition(ctx, id, actorID, "acknowledged", "acknowledged", nil)
	if err != nil {
		return Incident{}, err
	}
	if inc.Type == "sos" && inc.BookingID != nil && s.safety != nil {
		if err := s.safety.AcknowledgeBooking(ctx, *inc.BookingID); err != nil {
			return Incident{}, fmt.Errorf("incidents: acknowledge sos events: %w", err)
		}
	}
	return inc, nil
}

// StartWork moves to in_progress.
func (s *Service) StartWork(ctx context.Context, id, actorID string) (Incident, error) {
	return s.transition(ctx, id, actorID, "in_progress", "note", strPtr("work started"))
}

// AddNote appends a timestamped note without changing status (§12 step 11).
func (s *Service) AddNote(ctx context.Context, id, actorID, body string) (Incident, error) {
	if strings.TrimSpace(body) == "" {
		return Incident{}, fmt.Errorf("%w: note body is required", ErrValidation)
	}
	inc, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return Incident{}, err
	}
	return s.apply(ctx, inc, actorID, nil, nil, nil, "note", &body)
}

// Escalate bumps severity one step (capped at critical) and trails it.
func (s *Service) Escalate(ctx context.Context, id, actorID string) (Incident, error) {
	inc, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return Incident{}, err
	}
	next := inc.Severity
	for i, sev := range severityOrder {
		if sev == inc.Severity && i+1 < len(severityOrder) {
			next = severityOrder[i+1]
		}
	}
	if next == inc.Severity {
		return Incident{}, fmt.Errorf("%w: already at maximum severity", ErrValidation)
	}
	body := fmt.Sprintf("severity %s -> %s", inc.Severity, next)
	return s.apply(ctx, inc, actorID, nil, &next, nil, "escalated", &body)
}

// Assign routes the incident to an operations user.
func (s *Service) Assign(ctx context.Context, id, actorID, assigneeID string) (Incident, error) {
	if strings.TrimSpace(assigneeID) == "" {
		return Incident{}, fmt.Errorf("%w: user_id is required", ErrValidation)
	}
	inc, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return Incident{}, err
	}
	body := "assigned to " + assigneeID
	return s.apply(ctx, inc, actorID, nil, nil, &assigneeID, "assigned", &body)
}

// Resolve closes the incident's work with a mandatory resolution note.
func (s *Service) Resolve(ctx context.Context, id, actorID, note string) (Incident, error) {
	if strings.TrimSpace(note) == "" {
		return Incident{}, fmt.Errorf("%w: resolution note is required", ErrValidation)
	}
	return s.transition(ctx, id, actorID, "resolved", "resolved", &note)
}

// Close marks the incident fully closed (terminal).
func (s *Service) Close(ctx context.Context, id, actorID string, note *string) (Incident, error) {
	return s.transition(ctx, id, actorID, "closed", "closed", note)
}

// ErrValidation marks malformed workflow input.
var ErrValidation = errors.New("incidents: validation failed")

func (s *Service) apply(ctx context.Context, before Incident, actorID string, status, severity, assignedTo *string, kind string, body *string) (Incident, error) {
	inc, err := s.repo.Apply(ctx, before.ID, actorID, status, severity, assignedTo, kind, body)
	if err != nil {
		return Incident{}, err
	}
	if s.hub != nil {
		s.hub.Broadcast(realtime.ChannelAdminOperations, realtime.NewMessage("incident.updated", map[string]any{
			"incident_id": inc.ID,
			"kind":        kind,
			"status":      inc.Status,
			"severity":    inc.Severity,
		}))
	}
	if s.audit != nil {
		_ = s.audit.Record(ctx, audit.Entry{
			ActorID:    actorID,
			Action:     "incidents." + kind,
			EntityType: "incident",
			EntityID:   inc.ID,
			Before:     map[string]any{"status": before.Status, "severity": before.Severity},
			After:      map[string]any{"status": inc.Status, "severity": inc.Severity, "note": body},
		})
	}
	return inc, nil
}

// ListFlags returns the quality/retraining queue (spec §4.4).
func (s *Service) ListFlags(ctx context.Context, status string, limit, offset int) ([]QualityFlag, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.ListFlags(ctx, status, limit, offset)
}

// ResolveFlag closes one quality flag with a mandatory note and audits it.
func (s *Service) ResolveFlag(ctx context.Context, id, actorID, note string) (QualityFlag, error) {
	if strings.TrimSpace(note) == "" {
		return QualityFlag{}, fmt.Errorf("%w: resolution note is required", ErrValidation)
	}
	f, err := s.repo.ResolveFlag(ctx, id, actorID, note)
	if err != nil {
		return QualityFlag{}, err
	}
	if s.audit != nil {
		_ = s.audit.Record(ctx, audit.Entry{
			ActorID:    actorID,
			Action:     "quality_flag.resolved",
			EntityType: "quality_flag",
			EntityID:   f.ID,
			After:      map[string]any{"guide_id": f.GuideID, "kind": f.Kind, "note": note},
		})
	}
	return f, nil
}

func strPtr(s string) *string { return &s }
