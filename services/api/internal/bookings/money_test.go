package bookings

import "testing"

func TestParseDecimal(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"450.00", 45000},
		{"450", 45000},
		{"0.01", 1},
		{"250.5", 25050},
		{"15", 1500},  // platform_fee_pct as centi-percent
		{"4.0", 400},  // percentage with one decimal
		{"2.75", 275}, // two-decimal percentage
		{"0", 0},
		{" 67.50 ", 6750},
	}
	for _, c := range cases {
		got, err := ParseDecimal(c.in)
		if err != nil {
			t.Fatalf("ParseDecimal(%q): %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("ParseDecimal(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestParseDecimalRejects(t *testing.T) {
	for _, in := range []string{"", "abc", "1.234", "-5.00", "12,50", ".", "1.2.3", "NaN"} {
		if _, err := ParseDecimal(in); err == nil {
			t.Fatalf("ParseDecimal(%q) must fail", in)
		}
	}
}

func TestFormatMinor(t *testing.T) {
	cases := map[int64]string{
		45000: "450.00",
		1:     "0.01",
		0:     "0.00",
		6750:  "67.50",
		-250:  "-2.50",
	}
	for in, want := range cases {
		if got := FormatMinor(in); got != want {
			t.Fatalf("FormatMinor(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestPctOfRounding(t *testing.T) {
	cases := []struct {
		amount, pct int64
		want        int64
	}{
		{45000, 1500, 6750}, // GHS 450 @ 15% = 67.50
		{45000, 300, 1350},  // GHS 450 @ 3% = 13.50
		{25000, 1500, 3750}, // GHS 250 @ 15% = 37.50
		{90000, 1500, 13500},
		{101, 1500, 15}, // 15.15 pesewas -> 15
		{110, 1500, 17}, // 16.50 pesewas -> 17 (half away from zero)
		{103, 1500, 15}, // 15.45 -> 15
		{1, 50, 0},      // 0.005 pesewa -> 0 (below half)
		{1, 100, 0},     // 0.01 pesewa -> 0
		{100, 50, 1},    // 0.50 pesewa -> 1 (exactly half rounds away from zero)
		{0, 1500, 0},
	}
	for _, c := range cases {
		if got := PctOf(c.amount, c.pct); got != c.want {
			t.Fatalf("PctOf(%d, %d) = %d, want %d", c.amount, c.pct, got, c.want)
		}
	}
}

// TestComputeBreakdownSpecExample pins the spec §9.1 allocation example and
// the exact-sum invariant (fee + levy + payable == amount).
func TestComputeBreakdownSpecExample(t *testing.T) {
	b, err := ComputeBreakdown("450.00", "GHS", "15", "3")
	if err != nil {
		t.Fatalf("ComputeBreakdown: %v", err)
	}
	if b.Amount != "450.00" || b.PlatformFee != "67.50" || b.TourismLevy != "13.50" ||
		b.GuidePayableEstimate != "369.00" {
		t.Fatalf("breakdown = %+v, want 450.00/67.50/13.50/369.00 (spec §9.1)", b)
	}

	a, _ := ParseDecimal(b.Amount)
	fee, _ := ParseDecimal(b.PlatformFee)
	levy, _ := ParseDecimal(b.TourismLevy)
	payable, _ := ParseDecimal(b.GuidePayableEstimate)
	if fee+levy+payable != a {
		t.Fatalf("fee+levy+payable = %d, want %d (no rounding drift)", fee+levy+payable, a)
	}
}

func TestComputeBreakdownRejectsBadConfig(t *testing.T) {
	if _, err := ComputeBreakdown("450.00", "GHS", "fifteen", "3"); err == nil {
		t.Fatal("bad fee pct must fail")
	}
	if _, err := ComputeBreakdown("not-a-price", "GHS", "15", "3"); err == nil {
		t.Fatal("bad amount must fail")
	}
}
