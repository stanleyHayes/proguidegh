package guides

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"proguidegh/api/internal/availability"
)

// haversineKm is the great-circle distance formula used by the radius filter
// in Candidates' SQL. It exists in Go so the formula itself is unit-testable;
// the SQL expression in Candidates implements the identical math against
// guide_profiles.latitude/longitude (float8 trig — never money).
func haversineKm(lat1, lng1, lat2, lng2 float64) float64 {
	const earthKm = 6371.0
	rad := func(deg float64) float64 { return deg * math.Pi / 180 }
	dLat := rad(lat2 - lat1)
	dLng := rad(lng2 - lng1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(rad(lat1))*math.Cos(rad(lat2))*math.Sin(dLng/2)*math.Sin(dLng/2)
	return earthKm * 2 * math.Asin(math.Sqrt(a))
}

// SearchFilters is the validated §10.1 filter set. Zero values disable the
// filter.
type SearchFilters struct {
	RegionID       *string
	Lat, Lng       *float64
	RadiusKm       float64
	Language       string
	MinProficiency int // 0 = any; ordinal basic=1..native=4
	SpecialtyID    *string
	MinRating      *float64
	EliteOnly      bool

	// Date/time availability: when HasWindow is true, candidates must have a
	// weekly schedule covering [StartsAt, EndsAt), no intersecting time off
	// and no overlapping active booking (spec §10.2).
	HasWindow bool
	StartsAt  time.Time
	EndsAt    time.Time

	// AvailableNow restricts to guides holding a live Redis presence marker.
	AvailableNow bool

	Limit  int
	Offset int
}

// SearchRow is one search result: the public fields plus language/specialty
// codes. Guide coordinates are never exposed publicly — the radius filter
// uses them server-side only.
type SearchRow struct {
	UserID      string   `json:"user_id"`
	PublicName  string   `json:"public_name"`
	RatingAvg   string   `json:"rating_avg"`
	RatingCount int      `json:"rating_count"`
	EliteStatus bool     `json:"elite_status"`
	RegionID    *string  `json:"region_id"`
	RegionName  *string  `json:"region_name"`
	Languages   []string `json:"languages"`
	Specialties []string `json:"specialties"`
	Online      bool     `json:"online"`
}

// proficiencyOrdinal mirrors the guide_languages proficiency levels for
// min_proficiency comparisons; the SQL CASE expression must stay in sync.
var proficiencyOrdinal = map[string]int{
	"basic": 1, "conversational": 2, "fluent": 3, "native": 4,
}

// ProficiencyOrdinal returns the ordinal for a proficiency code (0 = unknown).
func ProficiencyOrdinal(code string) int { return proficiencyOrdinal[code] }

// SearchRepository runs the §10.1 search. Kept separate from Repository: the
// candidate pipeline crosses guide, certification, document, availability and
// booking tables, which the profile repository should not know about.
type SearchRepository struct {
	pool *pgxpool.Pool
}

// NewSearchRepository builds the search repository.
func NewSearchRepository(pool *pgxpool.Pool) *SearchRepository {
	return &SearchRepository{pool: pool}
}

// Candidates returns all §10.2-eligible guides matching the Postgres-side
// filters (region/radius/language/specialty/rating/elite plus, for a dated
// search, time-off and active-booking exclusion), ranked deterministically:
// rating_avg desc, rating_count desc, user_id (dispatch scoring is Phase 5).
// The weekly-schedule coverage and Redis presence checks run afterwards in
// Go (see availability.Covered and Presence).
//
// Pagination: offset, not cursor — the candidate set is bounded (eligible
// guides, not an append-only event stream) and the rank is a computed
// composite; offset keeps the contract simple per spec §14's allowance for
// bounded lists.
func (r *SearchRepository) Candidates(ctx context.Context, f SearchFilters) ([]SearchRow, error) {
	where := ""
	args := []any{}
	add := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	if f.RegionID != nil {
		where += ` AND gp.region_id = ` + add(*f.RegionID) + `::uuid`
	}
	if f.Lat != nil && f.Lng != nil {
		// Haversine in SQL against the guide's operating base (spec §10.1
		// coordinates/radius). Trig uses float8 — money is never involved.
		lat := add(*f.Lat)
		lng := add(*f.Lng)
		radius := add(f.RadiusKm)
		where += ` AND gp.latitude IS NOT NULL AND gp.longitude IS NOT NULL
			AND (6371 * 2 * ASIN(SQRT(
				POWER(SIN(RADIANS(gp.latitude::float8 - ` + lat + `) / 2), 2) +
				COS(RADIANS(` + lat + `)) * COS(RADIANS(gp.latitude::float8)) *
				POWER(SIN(RADIANS(gp.longitude::float8 - ` + lng + `) / 2), 2)
			))) <= ` + radius
	}
	if f.Language != "" {
		where += ` AND EXISTS (
			SELECT 1 FROM guide_languages gl
			WHERE gl.guide_id = gp.user_id AND gl.language_code = ` + add(f.Language) + `
			  AND CASE gl.proficiency
				WHEN 'basic' THEN 1 WHEN 'conversational' THEN 2
				WHEN 'fluent' THEN 3 WHEN 'native' THEN 4 END >= ` + add(f.MinProficiency) + `)`
	}
	if f.SpecialtyID != nil {
		where += ` AND EXISTS (
			SELECT 1 FROM guide_specialties gs
			WHERE gs.guide_id = gp.user_id AND gs.specialty_id = ` + add(*f.SpecialtyID) + `::uuid)`
	}
	if f.MinRating != nil {
		where += ` AND gp.rating_avg >= ` + add(*f.MinRating)
	}
	if f.EliteOnly {
		where += ` AND gp.elite_status`
	}
	if f.HasWindow {
		starts := add(f.StartsAt)
		ends := add(f.EndsAt)
		active := add(availability.BookingActiveStatuses)
		where += ` AND NOT EXISTS (
			SELECT 1 FROM guide_time_off t
			WHERE t.guide_id = gp.user_id
			  AND tstzrange(t.starts_at, t.ends_at, '[)') && tstzrange(` + starts + `, ` + ends + `, '[)'))
			AND NOT EXISTS (
			SELECT 1 FROM bookings b
			WHERE b.guide_id = gp.user_id
			  AND b.status = ANY(` + active + `::text[])
			  AND tstzrange(b.starts_at, b.ends_at, '[)') && tstzrange(` + starts + `, ` + ends + `, '[)'))`
	}

	// §10.2 eligibility gate in SQL — mirrors PubliclyVisible plus the
	// certification.MandatoryDocGroups document requirements (keep in sync).
	rows, err := r.pool.Query(ctx, `
		SELECT gp.user_id, gp.public_name, gp.rating_avg::text, gp.rating_count,
		       gp.elite_status, gp.region_id, rg.name,
		       COALESCE((SELECT array_agg(gl.language_code ORDER BY gl.language_code)
		                 FROM guide_languages gl WHERE gl.guide_id = gp.user_id), '{}'),
		       COALESCE((SELECT array_agg(s.code ORDER BY s.code)
		                 FROM guide_specialties gs JOIN specialties s ON s.id = gs.specialty_id
		                 WHERE gs.guide_id = gp.user_id), '{}')
		FROM guide_profiles gp
		JOIN users u ON u.id = gp.user_id AND u.status = 'active'
		JOIN certification_cases cc ON cc.guide_id = gp.user_id AND cc.status = 'ACTIVE'
		LEFT JOIN regions rg ON rg.id = gp.region_id
		WHERE gp.status NOT IN ('suspended', 'disabled')
		  AND EXISTS (SELECT 1 FROM guide_documents d WHERE d.guide_id = gp.user_id
		              AND d.type IN ('national_id', 'passport', 'drivers_license')
		              AND d.status NOT IN ('rejected', 'expired')
		              AND (d.expires_at IS NULL OR d.expires_at > now()))
		  AND EXISTS (SELECT 1 FROM guide_documents d WHERE d.guide_id = gp.user_id
		              AND d.type = 'background_check'
		              AND d.status NOT IN ('rejected', 'expired')
		              AND (d.expires_at IS NULL OR d.expires_at > now()))
		  AND EXISTS (SELECT 1 FROM guide_documents d WHERE d.guide_id = gp.user_id
		              AND d.type = 'certification'
		              AND d.status NOT IN ('rejected', 'expired')
		              AND (d.expires_at IS NULL OR d.expires_at > now()))
		  AND EXISTS (SELECT 1 FROM guide_documents d WHERE d.guide_id = gp.user_id
		              AND d.type = 'insurance'
		              AND d.status NOT IN ('rejected', 'expired')
		              AND (d.expires_at IS NULL OR d.expires_at > now()))
		`+where+`
		ORDER BY gp.rating_avg DESC, gp.rating_count DESC, gp.user_id`, args...)
	if err != nil {
		return nil, fmt.Errorf("guides: search candidates: %w", err)
	}
	defer rows.Close()

	out := []SearchRow{}
	for rows.Next() {
		var sr SearchRow
		if err := rows.Scan(&sr.UserID, &sr.PublicName, &sr.RatingAvg, &sr.RatingCount,
			&sr.EliteStatus, &sr.RegionID, &sr.RegionName, &sr.Languages, &sr.Specialties); err != nil {
			return nil, fmt.Errorf("guides: scan search row: %w", err)
		}
		out = append(out, sr)
	}
	return out, rows.Err()
}

// FilterBySchedule keeps candidates whose weekly schedule covers
// [start, end) (spec §10.1 date/time availability). One batched query for
// all candidate windows, then the pure Covered check per guide.
func (r *SearchRepository) FilterBySchedule(ctx context.Context, rows []SearchRow, start, end time.Time) ([]SearchRow, error) {
	if len(rows) == 0 {
		return rows, nil
	}
	ids := make([]string, 0, len(rows))
	for _, sr := range rows {
		ids = append(ids, sr.UserID)
	}
	wrows, err := r.pool.Query(ctx, `
		SELECT guide_id, weekday, start_time::text, end_time::text, timezone
		FROM guide_availability
		WHERE guide_id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, fmt.Errorf("guides: load availability windows: %w", err)
	}
	defer wrows.Close()

	byGuide := map[string][]availability.Window{}
	for wrows.Next() {
		var gid, startS, endS string
		var w availability.Window
		if err := wrows.Scan(&gid, &w.Weekday, &startS, &endS, &w.Timezone); err != nil {
			return nil, fmt.Errorf("guides: scan availability window: %w", err)
		}
		if w.StartMin, err = availability.ParseClock(startS); err != nil {
			return nil, err
		}
		if w.EndMin, err = availability.ParseClock(endS); err != nil {
			return nil, err
		}
		byGuide[gid] = append(byGuide[gid], w)
	}
	if err := wrows.Err(); err != nil {
		return nil, err
	}

	out := []SearchRow{}
	for _, sr := range rows {
		if availability.Covered(byGuide[sr.UserID], start, end) {
			out = append(out, sr)
		}
	}
	return out, nil
}
