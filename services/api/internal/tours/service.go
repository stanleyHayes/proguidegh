// Package tours implements Phase 5 tour operations: the guide-driven
// §8.2 edges GUIDE_EN_ROUTE -> GUIDE_ARRIVED -> IN_PROGRESS -> COMPLETED
// plus the operations override transition. Every move goes through
// bookings.Transition/TransitionTx — the single validated write path —
// never a raw status UPDATE.
package tours

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"proguidegh/api/internal/bookings"
	"proguidegh/api/internal/ledger"
	"proguidegh/api/internal/realtime"
	"proguidegh/api/internal/tracking"
)

// Sentinel errors mapped by the handler.
var (
	// ErrNotAssigned — the caller is not the booking's assigned guide (404;
	// assignment never leaks).
	ErrNotAssigned = errors.New("tours: booking not assigned to this guide")
	// ErrReasonRequired — the operations override requires a reason (400).
	ErrReasonRequired = errors.New("tours: reason is required")
)

// step pairs an endpoint action with its §8.2 target status.
type step struct {
	Action string
	To     string
}

// Steps are the legal guide-driven tour operations edges.
var (
	StepEnRoute  = step{Action: "tour.en_route", To: bookings.StatusGuideEnRoute}
	StepArrived  = step{Action: "tour.arrived", To: bookings.StatusGuideArrived}
	StepStart    = step{Action: "tour.start", To: bookings.StatusInProgress}
	StepComplete = step{Action: "tour.complete", To: bookings.StatusCompleted}
)

// Service is the tour-operations domain service.
type Service struct {
	pool     *pgxpool.Pool
	bookings *bookings.Repository
	ledger   *ledger.Service
	tracking *tracking.Service
	hub      *realtime.Hub
}

// NewService builds the service.
func NewService(pool *pgxpool.Pool, bookingsRepo *bookings.Repository,
	ledgerSvc *ledger.Service, trackingSvc *tracking.Service, hub *realtime.Hub) *Service {
	return &Service{pool: pool, bookings: bookingsRepo, ledger: ledgerSvc,
		tracking: trackingSvc, hub: hub}
}

// GuideStep applies one guide-driven edge (en-route/arrived/start). The
// caller must be the assigned guide; legality comes from the §8.2 state
// machine, so a wrong-order call surfaces ErrIllegalTransition (409).
func (s *Service) GuideStep(ctx context.Context, bookingID, guideID string, st step) (bookings.Booking, error) {
	b, err := s.bookings.GetByID(ctx, bookingID)
	if errors.Is(err, bookings.ErrNotFound) {
		return bookings.Booking{}, err
	}
	if err != nil {
		return bookings.Booking{}, fmt.Errorf("tours: load booking: %w", err)
	}
	if b.GuideID == nil || *b.GuideID != guideID {
		return bookings.Booking{}, ErrNotAssigned
	}
	meta, _ := json.Marshal(map[string]string{"action": st.Action})
	b, _, err = s.bookings.Transition(ctx, bookingID, guideID, st.To, json.RawMessage(meta))
	if err != nil {
		return bookings.Booking{}, err
	}
	s.afterStep(ctx, b)
	return b, nil
}

// Complete drives IN_PROGRESS -> COMPLETED and, in the same transaction,
// sets ends_at to the actual completion time and moves the guide payable
// from pending to eligible in the ledger (§9.2: "Booking completion moves
// guide payable from pending to eligible"). The ledger move is a balanced
// two-entry posting whose reference (ELIGIBLE:<booking>) makes it
// idempotent by uniqueness. payout_delay_days is a payout-time hold applied
// by the payouts phase, not a ledger concept — documented for Phase 7.
func (s *Service) Complete(ctx context.Context, bookingID, guideID string) (bookings.Booking, error) {
	b, err := s.bookings.GetByID(ctx, bookingID)
	if errors.Is(err, bookings.ErrNotFound) {
		return bookings.Booking{}, err
	}
	if err != nil {
		return bookings.Booking{}, fmt.Errorf("tours: load booking: %w", err)
	}
	if b.GuideID == nil || *b.GuideID != guideID {
		return bookings.Booking{}, ErrNotAssigned
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return bookings.Booking{}, fmt.Errorf("tours: begin complete: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after Commit

	meta, _ := json.Marshal(map[string]string{"action": StepComplete.Action})
	b, _, err = s.bookings.TransitionTx(ctx, tx, bookingID, guideID, bookings.StatusCompleted, json.RawMessage(meta))
	if err != nil {
		return bookings.Booking{}, err
	}

	// Actual end time replaces the scheduled estimate on completion. Clamped
	// to stays after starts_at (schema CHECK) — in production completion
	// always happens after the start, so the clamp is a no-op there; it only
	// binds for tests/dev data with future bookings.
	if _, err := tx.Exec(ctx, `
		UPDATE bookings SET ends_at = GREATEST(now(), starts_at + interval '1 minute'),
		                    updated_at = now()
		WHERE id = $1`, bookingID); err != nil {
		return bookings.Booking{}, fmt.Errorf("tours: set ends_at: %w", err)
	}

	// Move the guide payable pending -> eligible when an allocation exists.
	// Unpaid/dev bookings carry no PAYMENT posting; the move is skipped then.
	pending, err := s.ledger.AccountID(ctx, tx, "platform", nil, "guide_payable_pending")
	if err != nil {
		return bookings.Booking{}, err
	}
	eligible, err := s.ledger.AccountID(ctx, tx, "platform", nil, "guide_payable_eligible")
	if err != nil {
		return bookings.Booking{}, err
	}
	var amountMinor *int64
	if err := tx.QueryRow(ctx, `
		SELECT ROUND(le.amount * 100)::bigint
		FROM ledger_entries le
		JOIN ledger_transactions lt ON lt.id = le.transaction_id
		WHERE lt.booking_id = $1 AND lt.type = 'PAYMENT'
		  AND le.account_id = $2 AND le.direction = 'credit'
		LIMIT 1`, bookingID, pending).Scan(&amountMinor); err != nil &&
		!errors.Is(err, pgx.ErrNoRows) {
		return bookings.Booking{}, fmt.Errorf("tours: load payable allocation: %w", err)
	}
	if amountMinor != nil && *amountMinor > 0 {
		if _, err := s.ledger.Post(ctx, tx, ledger.Transaction{
			Reference: "ELIGIBLE:" + bookingID,
			Type:      "PAYOUT_ELIGIBILITY",
			BookingID: bookingID,
			Entries: []ledger.Entry{
				{AccountID: pending, Direction: ledger.Debit, AmountMinor: *amountMinor},
				{AccountID: eligible, Direction: ledger.Credit, AmountMinor: *amountMinor},
			},
		}); err != nil {
			var pgErr *pgconn.PgError
			if !(errors.As(err, &pgErr) && pgErr.Code == "23505") {
				return bookings.Booking{}, fmt.Errorf("tours: eligible move: %w", err)
			}
			// Reference already posted: an earlier completion did the move
			// (retry-safe); the state machine rejects true double completes.
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return bookings.Booking{}, fmt.Errorf("tours: commit complete: %w", err)
	}
	now := b.UpdatedAt
	b.EndsAt = &now
	s.afterStep(ctx, b)
	return b, nil
}

// AdminTransition is the operations override (permission dispatch.manage at
// the router): a validated §8.2 transition with a mandatory reason. The
// state machine is still the only legality source — operations can move a
// stuck booking but cannot invent edges.
func (s *Service) AdminTransition(ctx context.Context, bookingID, actorID, to, reason string) (bookings.Booking, bookings.Event, error) {
	if strings.TrimSpace(reason) == "" {
		return bookings.Booking{}, bookings.Event{}, ErrReasonRequired
	}
	if !bookings.ValidStatus(to) {
		return bookings.Booking{}, bookings.Event{},
			fmt.Errorf("%w: unknown status %q", bookings.ErrIllegalTransition, to)
	}
	meta, _ := json.Marshal(map[string]string{"action": "admin.bookings.transition", "reason": reason})
	b, e, err := s.bookings.Transition(ctx, bookingID, actorID, to, json.RawMessage(meta))
	if err != nil {
		return bookings.Booking{}, bookings.Event{}, err
	}
	s.afterStep(ctx, b)
	return b, e, nil
}

// afterStep pushes the new state to the booking and operations channels and
// records a tour-event location checkpoint from the guide's freshest fix.
func (s *Service) afterStep(ctx context.Context, b bookings.Booking) {
	if s.hub != nil {
		msg := realtime.NewMessage("booking.updated", b)
		s.hub.Broadcast(realtime.ChannelBooking(b.ID), msg)
		s.hub.Broadcast(realtime.ChannelAdminOperations, msg)
	}
	if s.tracking != nil && b.GuideID != nil {
		s.tracking.RecordEvent(ctx, b.ID, *b.GuideID)
	}
}
