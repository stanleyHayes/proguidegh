// Package bookings implements the booking aggregate (spec §8.2): the state
// machine rooted at the bookings table, immutable booking_status_events
// history, server-authoritative pricing snapshots (§14, §27), idempotent
// creation (§14) and the double-booking guard (§10.2).
//
// Status writes exist only inside this package (repository.Transition and
// the creation path) — controllers and admin forms never write bookings.status
// directly (AGENTS.md §3). Money is integer minor units (pesewas); floats are
// never used (§9).
package bookings
