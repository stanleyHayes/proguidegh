package bookings

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Money is handled as integer minor units (pesewas, 1/100 GHS) — never
// floats (spec §9, AGENTS.md §3). Database values arrive as NUMERIC(14,2)
// text and are parsed to minor units; percentages from system_settings are
// parsed by the same function, yielding centi-percent (15 -> 1500).

// errInvalidDecimal rejects malformed or over-precise decimal text.
var errInvalidDecimal = errors.New("bookings: invalid decimal")

// ParseDecimal parses a non-negative decimal string with at most two
// fractional digits into integer hundredths. Used both for money
// ("450.00" -> 45000 pesewas) and for percentages ("15" -> 1500
// centi-percent, "4.0" -> 400).
func ParseDecimal(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errInvalidDecimal
	}
	intPart, frac := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, frac = s[:i], s[i+1:]
	}
	if intPart == "" && frac == "" {
		return 0, errInvalidDecimal
	}
	if len(frac) > 2 {
		return 0, errInvalidDecimal
	}
	if intPart == "" {
		intPart = "0"
	}
	for _, c := range intPart + frac {
		if c < '0' || c > '9' {
			return 0, errInvalidDecimal
		}
	}
	switch len(frac) {
	case 0:
		frac = "00"
	case 1:
		frac += "0"
	}
	iv, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil {
		return 0, errInvalidDecimal
	}
	fv, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, errInvalidDecimal
	}
	return iv*100 + fv, nil
}

// FormatMinor renders integer minor units as a two-decimal string
// (45000 -> "450.00", matching the NUMERIC(14,2) storage format).
func FormatMinor(v int64) string {
	if v < 0 {
		return "-" + FormatMinor(-v)
	}
	return fmt.Sprintf("%d.%02d", v/100, v%100)
}

// PctOf returns amount * pct / 100 rounded half away from zero, where amount
// is minor units and pct is centi-percent (1500 = 15%). Pure integer math:
// amount * pct / 10_000 with a +5_000 bias before division.
func PctOf(amount, pct int64) int64 {
	p := amount * pct
	if p >= 0 {
		return (p + 5000) / 10000
	}
	return (p - 5000) / 10000
}

// PriceBreakdown is the server-authoritative quote (spec §14, §27): the
// package amount plus the platform fee and tourism levy splits, and the
// guide's estimated gross payable (before gateway fees, which Phase 4 records
// separately per spec §9.1).
type PriceBreakdown struct {
	Amount               string `json:"amount"`
	Currency             string `json:"currency"`
	PlatformFee          string `json:"platform_fee"`
	TourismLevy          string `json:"tourism_levy"`
	GuidePayableEstimate string `json:"guide_payable_estimate"`
	PlatformFeePct       string `json:"platform_fee_pct"`
	TourismLevyPct       string `json:"tourism_levy_pct"`
}

// ComputeBreakdown splits a NUMERIC(14,2) amount string by the configured
// percentages. Example (spec §9.1): GHS 450.00 at 15%/3% -> fee 67.50,
// levy 13.50, payable 369.00. payable = amount - fee - levy, so the three
// always sum exactly to the amount (no rounding drift).
func ComputeBreakdown(amount, currency, feePct, levyPct string) (PriceBreakdown, error) {
	a, err := ParseDecimal(amount)
	if err != nil {
		return PriceBreakdown{}, fmt.Errorf("bookings: bad amount %q: %w", amount, err)
	}
	fp, err := ParseDecimal(feePct)
	if err != nil {
		return PriceBreakdown{}, fmt.Errorf("bookings: bad platform_fee_pct %q: %w", feePct, err)
	}
	lp, err := ParseDecimal(levyPct)
	if err != nil {
		return PriceBreakdown{}, fmt.Errorf("bookings: bad tourism_levy_pct %q: %w", levyPct, err)
	}
	fee := PctOf(a, fp)
	levy := PctOf(a, lp)
	return PriceBreakdown{
		Amount:               FormatMinor(a),
		Currency:             currency,
		PlatformFee:          FormatMinor(fee),
		TourismLevy:          FormatMinor(levy),
		GuidePayableEstimate: FormatMinor(a - fee - levy),
		PlatformFeePct:       feePct,
		TourismLevyPct:       levyPct,
	}, nil
}
