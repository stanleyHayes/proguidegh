package dispatch

import (
	"testing"
	"time"
)

func TestScoreWeightBehavior(t *testing.T) {
	w := DefaultWeights()
	sum := w.Distance + w.Rating + w.Specialty + w.Language + w.Workload + w.Reliability
	if sum < 0.99 || sum > 1.01 {
		t.Fatalf("default weights sum = %v, want ~1.0", sum)
	}

	near := 0.1
	far := 40.0
	base := Features{
		DistanceKm:            &near,
		RatingAvg:             5,
		RatingCount:           10,
		SpecialtyMatch:        1,
		LanguageMatch:         1,
		RecentWorkload:        0,
		AcceptanceReliability: 1,
		OffersSeen:            20,
		AcceptsSeen:           20,
	}

	// A perfect candidate scores at the top of the [0,1] range.
	if got := Score(w, base); got < 0.85 || got > 1.0 {
		t.Fatalf("perfect score = %v, want ~1.0", got)
	}

	// Distance: far away scores strictly less, and the distance weight
	// controls how much.
	farFeatures := base
	farFeatures.DistanceKm = &far
	closeScore, farScore := Score(w, base), Score(w, farFeatures)
	if farScore >= closeScore {
		t.Fatalf("far score %v not below close score %v", farScore, closeScore)
	}
	diff := closeScore - farScore
	if want := w.Distance * (1.0/1.1 - 1.0/41); diff < want-0.001 || diff > want+0.001 {
		t.Fatalf("distance diff = %v, want %v", diff, want)
	}

	// Rating: unrated guides are neutral (not zero), below a 5-star record.
	unrated := base
	unrated.RatingAvg, unrated.RatingCount = 0, 0
	if got := Score(w, unrated); got >= closeScore {
		t.Fatalf("unrated score %v, want below rated %v", got, closeScore)
	}
	if ratingScore(0, 0) != 0.5 {
		t.Fatalf("unrated sub-score = %v, want 0.5", ratingScore(0, 0))
	}

	// Workload fairness: busier guides score lower, smoothly.
	if workloadScore(0) <= workloadScore(5) || workloadScore(5) <= workloadScore(20) {
		t.Fatalf("workload sub-score not decreasing: %v %v %v",
			workloadScore(0), workloadScore(5), workloadScore(20))
	}

	// Unknown distance is neutral, between near and far.
	unknown := base
	unknown.DistanceKm = nil
	if got := Score(w, unknown); got >= closeScore || got <= farScore {
		t.Fatalf("unknown-distance score %v, want between %v and %v", got, farScore, closeScore)
	}

	// Determinism: same inputs, same output.
	if Score(w, base) != Score(w, base) {
		t.Fatal("Score is not deterministic")
	}
}

func TestScoreWeightsOverride(t *testing.T) {
	// Renormalizes a misconfigured row instead of inflating scores.
	w := ParseWeights(`{"distance": 2.0}`)
	sum := w.Distance + w.Rating + w.Specialty + w.Language + w.Workload + w.Reliability
	if sum < 0.99 || sum > 1.01 {
		t.Fatalf("renormalized sum = %v, want ~1.0", sum)
	}
	if w.Distance <= DefaultWeights().Distance {
		t.Fatalf("distance weight = %v, want boosted above default %v",
			w.Distance, DefaultWeights().Distance)
	}

	// Malformed/empty/negative rows fall back to defaults.
	for _, raw := range []string{"", "not json", `{"distance": -1}`, `{"distance": -0.5, "rating": -0.5}`} {
		if got := ParseWeights(raw); got != DefaultWeights() {
			t.Fatalf("ParseWeights(%q) = %+v, want defaults", raw, got)
		}
	}

	// A single zeroed weight is a legitimate override (that signal is
	// disabled); the rest renormalize.
	zeroed := ParseWeights(`{"distance": 0}`)
	if zeroed.Distance != 0 {
		t.Fatalf("zeroed distance = %v, want 0", zeroed.Distance)
	}
	zSum := zeroed.Rating + zeroed.Specialty + zeroed.Language + zeroed.Workload + zeroed.Reliability
	if zSum < 0.99 || zSum > 1.01 {
		t.Fatalf("zeroed-override sum = %v, want ~1.0", zSum)
	}

	// A distance-only weight map makes distance the whole score.
	near, far := 0.5, 30.0
	f := Features{DistanceKm: &near, RatingAvg: 1, RatingCount: 3, SpecialtyMatch: 0,
		LanguageMatch: 0, RecentWorkload: 9, AcceptanceReliability: 0, OffersSeen: 50}
	distOnly := ParseWeights(`{"distance": 1, "rating": 0, "specialty": 0, "language": 0, "workload": 0, "reliability": 0}`)
	f.DistanceKm = &far
	farScore := Score(distOnly, f)
	f.DistanceKm = &near
	nearScore := Score(distOnly, f)
	if farScore >= nearScore || nearScore != 1/(1+0.5) {
		t.Fatalf("distance-only scores near=%v far=%v", nearScore, farScore)
	}
}

func TestReliabilityWarmup(t *testing.T) {
	// Below the warmup threshold every guide is neutral.
	if got := Reliability(4, 4); got != 0.5 {
		t.Fatalf("Reliability(4,4) = %v, want 0.5 warmup", got)
	}
	if got := Reliability(0, 0); got != 0.5 {
		t.Fatalf("Reliability(0,0) = %v, want 0.5", got)
	}
	// At/above it the observed ratio applies.
	if got := Reliability(10, 8); got != 0.8 {
		t.Fatalf("Reliability(10,8) = %v, want 0.8", got)
	}
	if got := Reliability(ReliabilityWarmupOffers, 0); got != 0 {
		t.Fatalf("Reliability(5,0) = %v, want 0", got)
	}
}

func TestOfferIsExpired(t *testing.T) {
	now := time.Now()
	o := Offer{Status: OfferOffered, ExpiresAt: now.Add(30 * time.Second)}
	if o.IsExpired(now) {
		t.Fatal("fresh offer reported expired")
	}
	if !o.IsExpired(now.Add(31 * time.Second)) {
		t.Fatal("offer past deadline not reported expired")
	}
	// Exactly at the deadline the offer is gone.
	if !o.IsExpired(o.ExpiresAt) {
		t.Fatal("offer at exact deadline should be expired")
	}
}

func TestSpecialtyAndLanguageMatch(t *testing.T) {
	if got := specialtyMatch("HERITAGE_TOUR_8H", []string{"heritage_history", "city_tours"}); got != 1 {
		t.Fatalf("specialty hit = %v, want 1", got)
	}
	if got := specialtyMatch("HERITAGE_TOUR_8H", []string{"city_tours"}); got != 0 {
		t.Fatalf("specialty miss = %v, want 0", got)
	}
	if got := specialtyMatch("UNKNOWN_PACKAGE", []string{"city_tours"}); got != 0.5 {
		t.Fatalf("unmapped package = %v, want neutral 0.5", got)
	}
	if got := languageMatch("en", []string{"en", "tw"}); got != 1 {
		t.Fatalf("language hit = %v, want 1", got)
	}
	if got := languageMatch("fr", []string{"en"}); got != 0 {
		t.Fatalf("language miss = %v, want 0", got)
	}
	if got := languageMatch("", []string{"en"}); got != 0.5 {
		t.Fatalf("no preference = %v, want neutral 0.5", got)
	}
}
