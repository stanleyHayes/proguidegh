package payments

import (
	"proguidegh/api/internal/bookings"
)

// Allocation is the §9.1 split of a collected payment in integer pesewas.
// Fee and levy are rounded to the nearest pesewa; the guide payable takes
// the remainder, so the three shares ALWAYS sum exactly to the gross (no
// rounding drift, launch-blocking invariant).
type Allocation struct {
	Gross        int64
	PlatformFee  int64
	TourismLevy  int64
	GuidePayable int64
}

// Allocate splits grossMinor (pesewas) by the configured percentages
// (centi-percent: 1500 = 15%), using the same integer math as the Phase 3
// quote (bookings.PctOf) so the ledger allocation matches the quoted
// breakdown pesewa-for-pesewa.
func Allocate(grossMinor, feePctCenti, levyPctCenti int64) Allocation {
	fee := bookings.PctOf(grossMinor, feePctCenti)
	levy := bookings.PctOf(grossMinor, levyPctCenti)
	return Allocation{
		Gross:        grossMinor,
		PlatformFee:  fee,
		TourismLevy:  levy,
		GuidePayable: grossMinor - fee - levy,
	}
}
