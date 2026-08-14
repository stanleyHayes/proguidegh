package dispatch

import (
	"encoding/json"
	"math"
)

// Weights is the §10.3 step 2 scoring weight map. The defaults below are the
// V1 tuning; system_settings.dispatch_weights (JSON, same keys) overrides
// them at runtime. Weights should sum to 1.0; ParseWeights renormalizes when
// they do not so a misconfigured row cannot inflate scores.
type Weights struct {
	Distance    float64 `json:"distance"`
	Rating      float64 `json:"rating"`
	Specialty   float64 `json:"specialty"`
	Language    float64 `json:"language"`
	Workload    float64 `json:"workload"`
	Reliability float64 `json:"reliability"`
}

// DefaultWeights returns the V1 tuning (spec §10.3): distance/ETA dominates,
// then rating; specialty/language are smaller nudges; workload fairness and
// acceptance reliability break ties toward guides who get (and honour) fewer
// offers.
func DefaultWeights() Weights {
	return Weights{
		Distance:    0.30,
		Rating:      0.25,
		Specialty:   0.15,
		Language:    0.10,
		Workload:    0.10,
		Reliability: 0.10,
	}
}

// ParseWeights decodes the system_settings.dispatch_weights JSON value,
// falling back to DefaultWeights on any malformed or missing key. Unknown
// keys are ignored; a sum far from 1.0 is renormalized; an all-zero or
// negative row yields the defaults.
func ParseWeights(raw string) Weights {
	def := DefaultWeights()
	if raw == "" {
		return def
	}
	w := def
	if err := json.Unmarshal([]byte(raw), &w); err != nil {
		return def
	}
	sum := w.Distance + w.Rating + w.Specialty + w.Language + w.Workload + w.Reliability
	if sum <= 0 {
		return def
	}
	if math.Abs(sum-1) > 0.01 {
		w.Distance /= sum
		w.Rating /= sum
		w.Specialty /= sum
		w.Language /= sum
		w.Workload /= sum
		w.Reliability /= sum
	}
	return w
}

// Features is the exact input set one offer was scored from (spec §10.3:
// "Persist scoring features/outcomes so a future model can be evaluated
// offline"). It is stored verbatim on dispatch_offers.features.
type Features struct {
	// DistanceKm from the guide's operating base to the booking meeting
	// point (haversine, float — never money). Nil when either side lacks
	// coordinates: the distance sub-score is neutral 0.5 then.
	DistanceKm *float64 `json:"distance_km,omitempty"`
	// RatingAvg/RatingCount are the guide's public rating snapshot.
	// Guides with no ratings score neutral 0.5 so new ACTIVE guides are
	// dispatchable.
	RatingAvg   float64 `json:"rating_avg"`
	RatingCount int     `json:"rating_count"`
	// SpecialtyMatch is 1 when the guide holds the specialty mapped to the
	// booking's package, 0 when not, 0.5 when the package has no specialty
	// mapping (PackageSpecialty, V1 static table).
	SpecialtyMatch float64 `json:"specialty_match"`
	// LanguageMatch is 1 when the guide lists the tourist's
	// preferred_language, 0 when not, 0.5 when the tourist set none.
	LanguageMatch float64 `json:"language_match"`
	// RecentWorkload is the guide's booking count over the trailing 30 days
	// (fairness: busier guides score lower, 1/(1+n)).
	RecentWorkload int `json:"recent_workload"`
	// AcceptanceReliability is accepts/offers from guide_dispatch_stats;
	// neutral 0.5 until the guide has seen ReliabilityWarmupOffers offers.
	AcceptanceReliability float64 `json:"acceptance_reliability"`
	// OffersSeen backs AcceptanceReliability (persisted for offline ML).
	OffersSeen int `json:"offers_seen"`
	// AcceptsSeen backs AcceptanceReliability (persisted for offline ML).
	AcceptsSeen int `json:"accepts_seen"`
}

// ReliabilityWarmupOffers is how many offers a guide must see before the
// observed accept ratio replaces the neutral 0.5.
const ReliabilityWarmupOffers = 5

// Reliability computes the acceptance-reliability feature from raw counters.
func Reliability(offers, accepts int) float64 {
	if offers < ReliabilityWarmupOffers {
		return 0.5
	}
	return float64(accepts) / float64(offers)
}

// distanceScore maps kilometres to (0,1]: 1/(1+km) — close guides approach
// 1, far guides decay smoothly. Unknown distance is neutral 0.5.
func distanceScore(km *float64) float64 {
	if km == nil || *km < 0 {
		return 0.5
	}
	return 1 / (1 + *km)
}

// ratingScore normalizes rating_avg to [0,1]; unrated guides are neutral.
func ratingScore(avg float64, count int) float64 {
	if count == 0 {
		return 0.5
	}
	return math.Max(0, math.Min(1, avg/5))
}

// workloadScore decays with the trailing-30-day booking count.
func workloadScore(recentBookings int) float64 {
	if recentBookings < 0 {
		recentBookings = 0
	}
	return 1 / (1 + float64(recentBookings))
}

// Score is the pure §10.3 step 2 composite: the weighted sum of the six
// sub-scores, each in [0,1], so the result is in [0,1] for normalized
// weights. Deterministic — same features and weights, same score.
func Score(w Weights, f Features) float64 {
	return w.Distance*distanceScore(f.DistanceKm) +
		w.Rating*ratingScore(f.RatingAvg, f.RatingCount) +
		w.Specialty*clamp01(f.SpecialtyMatch) +
		w.Language*clamp01(f.LanguageMatch) +
		w.Workload*workloadScore(f.RecentWorkload) +
		w.Reliability*clamp01(f.AcceptanceReliability)
}

func clamp01(v float64) float64 {
	return math.Max(0, math.Min(1, v))
}

// PackageSpecialty maps a tour package code to the specialty whose holders
// should score a specialty match (V1 static table — packages have no
// specialty link in the schema; extend the table when packages are added).
var PackageSpecialty = map[string]string{
	"CITY_TOUR_4H":     "city_tours",
	"HERITAGE_TOUR_8H": "heritage_history",
	"MULTI_REGION_24H": "multi_region_tours",
}
