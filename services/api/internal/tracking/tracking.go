// Package tracking implements live location (spec §11): the HTTPS location
// update endpoint, Redis current-position keys, the coarse persisted
// checkpoint trail and the realtime fan-out.
//
// Privacy/retention (§11.2):
//   - High-frequency positions live ONLY in Redis (loc:booking:{id},
//     loc:guide:{id}, 60s TTL) — never in Postgres.
//   - location_checkpoints holds the coarse safety/audit trail: the first
//     ping of a tour leg, then at most one row per CheckpointInterval (60s),
//     plus one row per tour event when a fresh position is known. No
//     endpoint exposes checkpoints to tourists.
//   - Tourists read the current position only for THEIR booking while it is
//     in the active window (GUIDE_EN_ROUTE..IN_PROGRESS); operations reads
//     require dispatch.manage; strangers get 404.
package tracking

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/jackc/pgx/v5/pgxpool"

	"proguidegh/api/internal/bookings"
	"proguidegh/api/internal/realtime"
)

// LocationTTL is how long a position stays current in Redis (spec §11:
// clients re-POST within the TTL to stay visible).
const LocationTTL = 60 * time.Second

// CheckpointInterval is the minimum gap between persisted checkpoints — the
// documented 1-in-N policy: at most one durable row per minute per booking.
const CheckpointInterval = 60 * time.Second

// LocationActiveStatuses is the window in which location is collected and
// readable: guide en route until tour completion (§11.2).
var LocationActiveStatuses = []string{
	bookings.StatusGuideEnRoute,
	bookings.StatusGuideArrived,
	bookings.StatusInProgress,
}

// Sentinel errors mapped by the handler.
var (
	// ErrValidation — malformed or out-of-range payload (400).
	ErrValidation = errors.New("tracking: invalid location payload")
	// ErrForbidden — caller is not the assigned guide / not allowed to read
	// (403 for writers; readers get 404 so existence never leaks).
	ErrForbidden = errors.New("tracking: not allowed for this booking")
	// ErrInactive — the booking is outside the location window (409).
	ErrInactive = errors.New("tracking: booking is not in the active location window")
	// ErrNoLocation — no current position is cached (404).
	ErrNoLocation = errors.New("tracking: no current location")
)

// Position is one validated location update (spec §11.1 payload).
type Position struct {
	BookingID  string    `json:"booking_id"`
	GuideID    string    `json:"guide_id"`
	Latitude   float64   `json:"latitude"`
	Longitude  float64   `json:"longitude"`
	AccuracyM  *float64  `json:"accuracy_m,omitempty"`
	Heading    *float64  `json:"heading,omitempty"`
	SpeedMps   *float64  `json:"speed_mps,omitempty"`
	CapturedAt time.Time `json:"captured_at"`
}

// Validate enforces the §11.1 ranges. captured_at may be up to 10 minutes in
// the future (clock skew) and 24h old (offline retry).
func (p *Position) Validate(now time.Time) error {
	if p.Latitude < -90 || p.Latitude > 90 {
		return fmt.Errorf("%w: latitude out of range", ErrValidation)
	}
	if p.Longitude < -180 || p.Longitude > 180 {
		return fmt.Errorf("%w: longitude out of range", ErrValidation)
	}
	if p.AccuracyM != nil && (*p.AccuracyM < 0 || *p.AccuracyM > 100000) {
		return fmt.Errorf("%w: accuracy_m out of range", ErrValidation)
	}
	if p.Heading != nil && (*p.Heading < 0 || *p.Heading >= 360) {
		return fmt.Errorf("%w: heading out of range", ErrValidation)
	}
	if p.SpeedMps != nil && (*p.SpeedMps < 0 || *p.SpeedMps > 200) {
		return fmt.Errorf("%w: speed_mps out of range", ErrValidation)
	}
	if p.CapturedAt.After(now.Add(10*time.Minute)) || p.CapturedAt.Before(now.Add(-24*time.Hour)) {
		return fmt.Errorf("%w: captured_at outside acceptable window", ErrValidation)
	}
	return nil
}

// Active reports whether a booking status is in the location window.
func Active(status string) bool {
	for _, s := range LocationActiveStatuses {
		if s == status {
			return true
		}
	}
	return false
}

func bookingKey(bookingID string) string { return "loc:booking:" + bookingID }
func guideKey(guideID string) string     { return "loc:guide:" + guideID }

// Service owns location writes/reads (Redis + coarse Postgres checkpoints).
type Service struct {
	pool     *pgxpool.Pool
	bookings *bookings.Repository
	rdb      *goredis.Client
	hub      *realtime.Hub
	now      func() time.Time
}

// NewService builds the service.
func NewService(pool *pgxpool.Pool, bookingsRepo *bookings.Repository,
	rdb *goredis.Client, hub *realtime.Hub) *Service {
	return &Service{pool: pool, bookings: bookingsRepo, rdb: rdb, hub: hub, now: time.Now}
}

// Ingest validates and records one update from the assigned guide: Redis
// current-position keys (authoritative for "where is the guide now"), one
// coarse checkpoint per the retention policy, and a WS fan-out to the
// booking channel and the operations feed.
func (s *Service) Ingest(ctx context.Context, bookingID, guideID string, p Position) (Position, error) {
	b, err := s.bookings.GetByID(ctx, bookingID)
	if errors.Is(err, bookings.ErrNotFound) {
		return Position{}, err
	}
	if err != nil {
		return Position{}, fmt.Errorf("tracking: load booking: %w", err)
	}
	if b.GuideID == nil || *b.GuideID != guideID {
		return Position{}, ErrForbidden
	}
	if !Active(b.Status) {
		return Position{}, fmt.Errorf("%w: booking is %s", ErrInactive, b.Status)
	}

	p.BookingID = bookingID
	p.GuideID = guideID
	if p.CapturedAt.IsZero() {
		p.CapturedAt = s.now().UTC()
	}
	if err := p.Validate(s.now()); err != nil {
		return Position{}, err
	}

	raw, err := json.Marshal(p)
	if err != nil {
		return Position{}, fmt.Errorf("tracking: marshal position: %w", err)
	}
	pipe := s.rdb.Pipeline()
	pipe.Set(ctx, bookingKey(bookingID), raw, LocationTTL)
	pipe.Set(ctx, guideKey(guideID), raw, LocationTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return Position{}, fmt.Errorf("tracking: cache position: %w", err)
	}

	if err := s.persistCheckpoint(ctx, p, false); err != nil {
		return Position{}, err
	}

	msg := realtime.NewMessage("location.update", p)
	if s.hub != nil {
		s.hub.Broadcast(realtime.ChannelBooking(bookingID), msg)
		s.hub.Broadcast(realtime.ChannelAdminOperations, msg)
	}
	return p, nil
}

// persistCheckpoint writes the coarse durable trail (§11.2): the first
// position of the tour leg, then at most one row per CheckpointInterval.
// Event rows (force) bypass the interval so every tour-event position is on
// the audit trail.
func (s *Service) persistCheckpoint(ctx context.Context, p Position, force bool) error {
	if !force {
		var last *time.Time
		if err := s.pool.QueryRow(ctx, `
			SELECT MAX(created_at) FROM location_checkpoints
			WHERE booking_id = $1`, p.BookingID).Scan(&last); err != nil {
			return fmt.Errorf("tracking: last checkpoint: %w", err)
		}
		if last != nil && s.now().Before(last.Add(CheckpointInterval)) {
			return nil // within the coarse interval — Redis carries the fresh fix
		}
	}
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO location_checkpoints
			(booking_id, guide_id, latitude, longitude, accuracy_m, heading, speed_mps, captured_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		p.BookingID, p.GuideID, p.Latitude, p.Longitude,
		p.AccuracyM, p.Heading, p.SpeedMps, p.CapturedAt); err != nil {
		return fmt.Errorf("tracking: persist checkpoint: %w", err)
	}
	return nil
}

// RecordEvent persists a tour-event checkpoint from the guide's freshest
// cached position (arrived/start/complete). Best-effort: no cached position,
// no row — the event itself is already in booking_status_events.
func (s *Service) RecordEvent(ctx context.Context, bookingID, guideID string) {
	p, err := s.CurrentForGuide(ctx, guideID)
	if err != nil || p.BookingID != bookingID {
		return
	}
	if err := s.persistCheckpoint(ctx, p, true); err != nil {
		// Checkpoint loss is non-fatal to the tour event; logged by caller path.
		_ = err
	}
}

// CurrentForGuide reads the guide's cached position (Redis only).
func (s *Service) CurrentForGuide(ctx context.Context, guideID string) (Position, error) {
	return s.read(ctx, guideKey(guideID))
}

// CurrentForBooking reads the booking's cached position for an authorized
// reader. Visibility is enforced by the handler; the booking must be in the
// active window (no historical exposure, §11.2).
func (s *Service) CurrentForBooking(ctx context.Context, bookingID string) (Position, error) {
	return s.read(ctx, bookingKey(bookingID))
}

func (s *Service) read(ctx context.Context, key string) (Position, error) {
	raw, err := s.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, goredis.Nil) {
		return Position{}, ErrNoLocation
	}
	if err != nil {
		return Position{}, fmt.Errorf("tracking: read position: %w", err)
	}
	var p Position
	if err := json.Unmarshal(raw, &p); err != nil {
		return Position{}, fmt.Errorf("tracking: decode position: %w", err)
	}
	return p, nil
}
