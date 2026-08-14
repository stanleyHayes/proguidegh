package bookings

import (
	"regexp"
	"testing"
	"time"
)

func TestReferenceFormat(t *testing.T) {
	re := regexp.MustCompile(`^PGH-[A-HJ-NP-Z2-9]{5}$`)
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		ref, err := newReference()
		if err != nil {
			t.Fatalf("newReference: %v", err)
		}
		if !re.MatchString(ref) {
			t.Fatalf("reference %q does not match PGH-XXXXX (unambiguous alphabet)", ref)
		}
		seen[ref] = true
	}
	if len(seen) < 150 {
		t.Fatalf("references look non-random: %d distinct of 200", len(seen))
	}
}

func TestCursorRoundTrip(t *testing.T) {
	at := time.Date(2026, 6, 1, 12, 30, 45, 123456789, time.UTC)
	id := "3f6d3f78-2c4d-4b3f-9a1c-1e0f9a1b2c3d"
	cur := encodeCursor(at, id)
	gotAt, gotID, err := decodeCursor(cur)
	if err != nil {
		t.Fatalf("decodeCursor: %v", err)
	}
	if !gotAt.Equal(at) || gotID != id {
		t.Fatalf("round trip = (%v, %s), want (%v, %s)", gotAt, gotID, at, id)
	}
}

func TestDecodeCursorRejects(t *testing.T) {
	for _, cur := range []string{"", "!!!", "bm90LXN0cnVjdHVyZWQ", "MjAyNi0wMS0wMXww"} {
		if _, _, err := decodeCursor(cur); err == nil {
			t.Fatalf("decodeCursor(%q) must fail", cur)
		}
	}
}
