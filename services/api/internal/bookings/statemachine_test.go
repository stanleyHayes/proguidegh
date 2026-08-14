package bookings

import "testing"

// TestHappyPathChain walks the full §8.2 main line.
func TestHappyPathChain(t *testing.T) {
	chain := []string{
		StatusDraft, StatusPaymentPending, StatusConfirmed, StatusGuideEnRoute,
		StatusGuideArrived, StatusInProgress, StatusCompleted,
	}
	for i := 0; i+1 < len(chain); i++ {
		if !CanTransition(chain[i], chain[i+1]) {
			t.Fatalf("%s -> %s must be legal", chain[i], chain[i+1])
		}
	}
}

// TestTransitionMatrix pins the full legal/illegal edge set so the state
// machine cannot silently grow or lose edges.
func TestTransitionMatrix(t *testing.T) {
	want := map[string][]string{
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
	if len(transitions) != len(want) {
		t.Fatalf("state count = %d, want %d", len(transitions), len(want))
	}
	for from, tos := range want {
		for _, to := range tos {
			if !CanTransition(from, to) {
				t.Fatalf("%s -> %s must be legal", from, to)
			}
		}
	}
	// Every (from, to) pair not listed must be illegal.
	for from := range transitions {
		for _, to := range Statuses() {
			legal := false
			for _, w := range want[from] {
				if w == to {
					legal = true
				}
			}
			if CanTransition(from, to) != legal {
				t.Fatalf("%s -> %s legality = %v, want %v", from, to, !legal, legal)
			}
		}
	}
}

// TestTerminalAndGuardStates covers the edges most likely to be abused:
// terminal states and skipping the payment step.
func TestTerminalAndGuardStates(t *testing.T) {
	for _, to := range Statuses() {
		if CanTransition(StatusRefunded, to) {
			t.Fatalf("REFUNDED is terminal; %s reachable", to)
		}
	}
	if CanTransition(StatusDraft, StatusConfirmed) {
		t.Fatal("DRAFT -> CONFIRMED must skip through PAYMENT_PENDING")
	}
	if CanTransition(StatusPaymentPending, StatusInProgress) {
		t.Fatal("PAYMENT_PENDING -> IN_PROGRESS must not skip CONFIRMED")
	}
	if !ValidStatus(StatusCompleted) || ValidStatus("REFUNDING") {
		t.Fatal("ValidStatus out of sync with the machine")
	}
}

// TestActiveStatuses pins the on-calendar overlap set (must match migration
// 0004's exclusion constraint and idx_bookings_guide_active).
func TestActiveStatuses(t *testing.T) {
	want := map[string]bool{
		StatusConfirmed: true, StatusGuideEnRoute: true,
		StatusGuideArrived: true, StatusInProgress: true,
	}
	for _, s := range Statuses() {
		if Active(s) != want[s] {
			t.Fatalf("Active(%s) = %v, want %v", s, !want[s], want[s])
		}
	}
	if len(ActiveStatuses) != 4 {
		t.Fatalf("ActiveStatuses = %v, want exactly 4", ActiveStatuses)
	}
}
