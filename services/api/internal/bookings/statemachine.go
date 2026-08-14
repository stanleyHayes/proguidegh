package bookings

import (
	"sort"

	"proguidegh/api/internal/availability"
)

// Booking statuses (spec §8.2). Stored uppercase; the transitions map below
// is the only legal source of moves.
const (
	StatusDraft              = "DRAFT"
	StatusPaymentPending     = "PAYMENT_PENDING"
	StatusConfirmed          = "CONFIRMED"
	StatusGuideEnRoute       = "GUIDE_EN_ROUTE"
	StatusGuideArrived       = "GUIDE_ARRIVED"
	StatusInProgress         = "IN_PROGRESS"
	StatusCompleted          = "COMPLETED"
	StatusPaymentFailed      = "PAYMENT_FAILED"
	StatusCancelledByTourist = "CANCELLED_BY_TOURIST"
	StatusCancelledByGuide   = "CANCELLED_BY_GUIDE"
	StatusCancelledByAdmin   = "CANCELLED_BY_ADMIN"
	StatusNoShow             = "NO_SHOW"
	StatusRefundPending      = "REFUND_PENDING"
	StatusRefunded           = "REFUNDED"
)

// transitions is the full legal transition set (spec §8.2). The happy path is
// DRAFT -> PAYMENT_PENDING -> CONFIRMED -> GUIDE_EN_ROUTE -> GUIDE_ARRIVED ->
// IN_PROGRESS -> COMPLETED. Cancellation edges land on REFUND_PENDING when a
// payment may need unwinding (Phase 4 decides the refund itself); PAYMENT_FAILED
// may retry back to PAYMENT_PENDING. REFUNDED is terminal.
var transitions = map[string][]string{
	StatusDraft:              {StatusPaymentPending, StatusCancelledByTourist},
	StatusPaymentPending:     {StatusConfirmed, StatusPaymentFailed, StatusCancelledByTourist, StatusCancelledByAdmin},
	StatusPaymentFailed:      {StatusPaymentPending, StatusCancelledByTourist},
	StatusConfirmed:          {StatusGuideEnRoute, StatusNoShow, StatusCancelledByTourist, StatusCancelledByGuide, StatusCancelledByAdmin},
	StatusGuideEnRoute:       {StatusGuideArrived, StatusNoShow, StatusCancelledByGuide, StatusCancelledByAdmin},
	StatusGuideArrived:       {StatusInProgress, StatusNoShow, StatusCancelledByAdmin},
	StatusInProgress:         {StatusCompleted},
	StatusCompleted:          {StatusRefundPending},
	StatusNoShow:             {StatusRefundPending},
	StatusCancelledByTourist: {StatusRefundPending},
	StatusCancelledByGuide:   {StatusRefundPending},
	StatusCancelledByAdmin:   {StatusRefundPending},
	StatusRefundPending:      {StatusRefunded},
	StatusRefunded:           {},
}

// ActiveStatuses are the on-calendar statuses that block a guide's schedule:
// the overlap guard (spec §10.2) only considers bookings in one of these.
// Single definition lives in availability (shared with search); migration
// 0004 (bookings_no_guide_overlap, idx_bookings_guide_active) must stay in
// sync.
var ActiveStatuses = availability.BookingActiveStatuses

// CanTransition reports whether from -> to is a legal state machine edge.
func CanTransition(from, to string) bool {
	for _, t := range transitions[from] {
		if t == to {
			return true
		}
	}
	return false
}

// ValidStatus reports whether s is a known booking status.
func ValidStatus(s string) bool {
	_, ok := transitions[s]
	return ok
}

// Statuses returns every known status, sorted.
func Statuses() []string {
	out := make([]string, 0, len(transitions))
	for s := range transitions {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// Active reports whether a status is on the guide's calendar (overlap set).
func Active(status string) bool {
	for _, s := range ActiveStatuses {
		if s == status {
			return true
		}
	}
	return false
}
