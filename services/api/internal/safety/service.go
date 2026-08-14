package safety

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"proguidegh/api/internal/bookings"
	"proguidegh/api/internal/platform/audit"
	"proguidegh/api/internal/platform/ratelimit"
	"proguidegh/api/internal/realtime"
)

// Sentinel errors mapped to HTTP statuses by the handler.
var (
	// ErrNotFound — no such booking (also used for callers who may not see
	// it, so existence never leaks).
	ErrNotFound = errors.New("safety: booking not found")
	// ErrForbidden — caller is neither the booking's tourist nor its guide.
	ErrForbidden = errors.New("safety: not a participant of this booking")
	// ErrInactive — SOS only makes sense on an active booking (§12 step 6).
	ErrInactive = errors.New("safety: booking is not active")
	// ErrValidation — malformed coordinates.
	ErrValidation = errors.New("safety: validation failed")
	// ErrRateLimited — SOS abuse guard (spec §15.2).
	ErrRateLimited = errors.New("safety: too many SOS triggers")
)

// SOSLimit caps SOS triggers per user — generous enough for a real
// emergency, tight enough to blunt abuse (spec §15.2).
var SOSLimit = ratelimit.Limit{Bucket: "sos", Max: 5, Window: time.Hour}

// activeStatuses are the booking states from which an SOS can be raised
// (matches the §8.2 state machine: confirmed through in-progress).
var activeStatuses = map[string]bool{
	"CONFIRMED":      true,
	"GUIDE_EN_ROUTE": true,
	"GUIDE_ARRIVED":  true,
	"IN_PROGRESS":    true,
}

// Service is the SOS application service.
type Service struct {
	repo     *Repository
	bookings *bookings.Repository
	hub      *realtime.Hub
	limiter  *ratelimit.Limiter
	audit    *audit.Recorder
}

// NewService builds the service. hub/audit may be nil in tests.
func NewService(repo *Repository, bookingsRepo *bookings.Repository, hub *realtime.Hub, limiter *ratelimit.Limiter, auditor *audit.Recorder) *Service {
	return &Service{repo: repo, bookings: bookingsRepo, hub: hub, limiter: limiter, audit: auditor}
}

// Trigger creates the immutable SOS event and its critical incident, alerts
// operations in realtime and queues the responder fallback notifications
// (spec §12 steps 8–10).
func (s *Service) Trigger(ctx context.Context, bookingID, userID string, lat, lng float64, accuracy *float64) (SOSEvent, Incident, error) {
	if lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		return SOSEvent{}, Incident{}, fmt.Errorf("%w: coordinates out of range", ErrValidation)
	}
	if ok, err := ratelimit.Keyed(s.limiter, SOSLimit, userID)(ctx); err != nil {
		slog.ErrorContext(ctx, "safety: rate limit check failed open", "error", err)
	} else if !ok {
		return SOSEvent{}, Incident{}, ErrRateLimited
	}

	b, err := s.bookings.GetByID(ctx, bookingID)
	if errors.Is(err, bookings.ErrNotFound) {
		return SOSEvent{}, Incident{}, ErrNotFound
	}
	if err != nil {
		return SOSEvent{}, Incident{}, fmt.Errorf("safety: load booking: %w", err)
	}
	isTourist := b.TouristID == userID
	isGuide := b.GuideID != nil && *b.GuideID == userID
	if !isTourist && !isGuide {
		return SOSEvent{}, Incident{}, ErrForbidden
	}
	if !activeStatuses[b.Status] {
		return SOSEvent{}, Incident{}, ErrInactive
	}

	ev, inc, err := s.repo.CreateSOS(ctx, bookingID, userID, lat, lng, accuracy, "critical")
	if err != nil {
		return SOSEvent{}, Incident{}, err
	}

	// Realtime operations alert (§12 step 9) — both the operations feed and
	// the booking channel so the counterpart participant sees it too.
	if s.hub != nil {
		msg := realtime.NewMessage("sos.triggered", map[string]any{
			"incident_id": inc.ID,
			"booking_id":  bookingID,
			"user_id":     userID,
			"role":        map[bool]string{true: "tourist", false: "guide"}[isTourist],
			"latitude":    lat,
			"longitude":   lng,
			"severity":    inc.Severity,
		})
		s.hub.Broadcast(realtime.ChannelAdminOperations, msg)
		s.hub.Broadcast(realtime.ChannelBooking(bookingID), msg)
	}

	// Fallback notifications for the operations roster (§12 step 10). Without
	// SMS/push provider credentials the queued in_app row is the auditable
	// fallback; providers attach downstream of this queue.
	responders, err := s.repo.ResponderIDs(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "safety: responder lookup failed", "error", err)
	}
	for _, rid := range responders {
		if rid == userID {
			continue
		}
		if err := s.repo.QueueNotification(ctx, rid, "in_app", "sos_alert"); err != nil {
			slog.ErrorContext(ctx, "safety: queue responder notification failed",
				"responder_id", rid, "error", err)
		}
	}

	if s.audit != nil {
		_ = s.audit.Record(ctx, audit.Entry{
			ActorID:    userID,
			Action:     "sos.triggered",
			EntityType: "incident",
			EntityID:   inc.ID,
			After:      map[string]any{"booking_id": bookingID, "severity": inc.Severity, "sos_event_id": ev.ID},
		})
	}
	return ev, inc, nil
}
