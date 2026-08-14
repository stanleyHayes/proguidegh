package guides

import (
	"math"
	"testing"
)

// TestHaversineKm pins the radius-filter formula against known Ghanaian
// city-pair distances; the Candidates SQL implements the identical math.
func TestHaversineKm(t *testing.T) {
	// Accra (5.6037, -0.1870), Kumasi (6.6885, -1.6244), Cape Coast
	// (5.1053, -1.2466) — published great-circle distances are ~200 km and
	// ~125 km respectively.
	cases := []struct {
		name          string
		lat1, lng1    float64
		lat2, lng2    float64
		wantKm, tolKm float64
	}{
		{"accra-kumasi", 5.6037, -0.1870, 6.6885, -1.6244, 202, 8},
		{"accra-capecoast", 5.6037, -0.1870, 5.1053, -1.2466, 127, 8},
	}
	for _, c := range cases {
		got := haversineKm(c.lat1, c.lng1, c.lat2, c.lng2)
		if math.Abs(got-c.wantKm) > c.tolKm {
			t.Fatalf("%s = %.1f km, want %.0f ± %.0f", c.name, got, c.wantKm, c.tolKm)
		}
	}

	// Zero distance and antipodal sanity.
	if d := haversineKm(5.6037, -0.1870, 5.6037, -0.1870); d != 0 {
		t.Fatalf("same point = %v km, want 0", d)
	}
	if d := haversineKm(5.6037, -0.1870, 5.7, -0.3); d > 20 {
		t.Fatalf("nearby point = %.1f km, want within a 20 km radius", d)
	}
	if d := haversineKm(5.6037, -0.1870, 6.6885, -1.6244); d < 25 {
		t.Fatalf("Kumasi must fall outside a 25 km Accra radius, got %.1f", d)
	}
}

func TestProficiencyOrdinal(t *testing.T) {
	order := []string{"basic", "conversational", "fluent", "native"}
	for i, code := range order {
		if got := ProficiencyOrdinal(code); got != i+1 {
			t.Fatalf("ProficiencyOrdinal(%q) = %d, want %d", code, got, i+1)
		}
	}
	if ProficiencyOrdinal("expert") != 0 {
		t.Fatal("unknown proficiency must map to 0")
	}
}
