package catalog

import (
	"testing"
	"time"
)

func rule(amount string, from time.Time, to *time.Time, region *string) PricingRule {
	return PricingRule{PackageID: "pkg", Amount: amount, EffectiveFrom: from, EffectiveTo: to, RegionID: region}
}

func TestEffectivePrice(t *testing.T) {
	jan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	jun := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	jul := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	nextYear := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	regionA := "region-a"
	regionB := "region-b"

	t.Run("no rules", func(t *testing.T) {
		if _, ok := EffectivePrice(nil, nil, jun); ok {
			t.Fatal("expected no rule to apply")
		}
	})

	t.Run("single open-ended rule", func(t *testing.T) {
		got, ok := EffectivePrice([]PricingRule{rule("250.00", jan, nil, nil)}, nil, jun)
		if !ok || got.Amount != "250.00" {
			t.Fatalf("got %v %v", got, ok)
		}
	})

	t.Run("latest effective_from wins", func(t *testing.T) {
		rules := []PricingRule{rule("250.00", jan, nil, nil), rule("275.00", jun, nil, nil)}
		got, ok := EffectivePrice(rules, nil, jul)
		if !ok || got.Amount != "275.00" {
			t.Fatalf("got %v %v, want 275.00", got, ok)
		}
		// Before the newer rule kicks in, the old price still applies.
		got, ok = EffectivePrice(rules, nil, jan.Add(time.Hour))
		if !ok || got.Amount != "250.00" {
			t.Fatalf("got %v %v, want 250.00", got, ok)
		}
	})

	t.Run("future rule ignored", func(t *testing.T) {
		rules := []PricingRule{rule("999.00", nextYear, nil, nil)}
		if _, ok := EffectivePrice(rules, nil, jun); ok {
			t.Fatal("future rule must not apply")
		}
	})

	t.Run("expired rule ignored", func(t *testing.T) {
		rules := []PricingRule{
			rule("200.00", jan, &jun, nil), // ended 2026-06-01
			rule("250.00", jun, nil, nil),  // takes over
		}
		got, ok := EffectivePrice(rules, nil, jun)
		if !ok || got.Amount != "250.00" {
			t.Fatalf("got %v %v, want 250.00 after expiry", got, ok)
		}
		if _, ok := EffectivePrice([]PricingRule{rule("200.00", jan, &jun, nil)}, nil, jul); ok {
			t.Fatal("expired rule with no successor must not apply")
		}
	})

	t.Run("region-specific beats global", func(t *testing.T) {
		rules := []PricingRule{
			rule("250.00", jan, nil, nil),
			rule("300.00", jan, nil, &regionA),
		}
		got, ok := EffectivePrice(rules, &regionA, jun)
		if !ok || got.Amount != "300.00" {
			t.Fatalf("region A: got %v %v, want 300.00", got, ok)
		}
		// Another region falls back to the global rule.
		got, ok = EffectivePrice(rules, &regionB, jun)
		if !ok || got.Amount != "250.00" {
			t.Fatalf("region B: got %v %v, want 250.00", got, ok)
		}
		// Nil region request: only global rules.
		got, ok = EffectivePrice(rules, nil, jun)
		if !ok || got.Amount != "250.00" {
			t.Fatalf("nil region: got %v %v, want 250.00", got, ok)
		}
	})

	t.Run("newer global supersedes older regional", func(t *testing.T) {
		rules := []PricingRule{
			rule("300.00", jan, nil, &regionA),
			rule("350.00", jun, nil, nil),
		}
		got, ok := EffectivePrice(rules, &regionA, jul)
		if !ok || got.Amount != "350.00" {
			t.Fatalf("got %v %v, want 350.00", got, ok)
		}
	})
}
