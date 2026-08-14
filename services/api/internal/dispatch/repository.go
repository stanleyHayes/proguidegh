package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"proguidegh/api/internal/availability"
	"proguidegh/api/internal/bookings"
)

// Offer statuses (migration 0006 CHECK constraint).
const (
	OfferOffered    = "OFFERED"
	OfferAccepted   = "ACCEPTED"
	OfferDeclined   = "DECLINED"
	OfferExpired    = "EXPIRED"
	OfferSuperseded = "SUPERSEDED"
)

// Sentinel errors mapped to HTTP statuses by the handler.
var (
	// ErrOfferNotFound — unknown offer id, or offer belongs to another guide
	// (404 either way; offer existence never leaks across guides).
	ErrOfferNotFound = errors.New("dispatch: offer not found")
	// ErrOfferExpired — the offer deadline passed (410). dispatch_offers.
	// expires_at is the source of truth; the Redis TTL key is only a cache.
	ErrOfferExpired = errors.New("dispatch: offer has expired")
	// ErrOfferClosed — the offer was already resolved, or another guide
	// already won the booking (409; first valid acceptance wins, §10.3).
	ErrOfferClosed = errors.New("dispatch: offer is no longer open")
	// ErrNotDispatchable — the booking is not awaiting a guide (409): wrong
	// status, or a guide is already assigned (direct bookings skip dispatch).
	ErrNotDispatchable = errors.New("dispatch: booking is not awaiting dispatch")
)

// Offer is a dispatch_offers row.
type Offer struct {
	ID          string          `json:"id"`
	BookingID   string          `json:"booking_id"`
	GuideID     string          `json:"guide_id"`
	BatchSeq    int             `json:"batch_seq"`
	Score       string          `json:"score"`
	Features    json.RawMessage `json:"features"`
	Status      string          `json:"status"`
	OfferedAt   time.Time       `json:"offered_at"`
	ExpiresAt   time.Time       `json:"expires_at"`
	RespondedAt *time.Time      `json:"responded_at"`
}

// IsExpired reports whether the offer deadline has passed (pure, unit-tested
// expiry logic — §31.27).
func (o Offer) IsExpired(now time.Time) bool { return !now.Before(o.ExpiresAt) }

const offerColumns = `id, booking_id, guide_id, batch_seq, score::text, features,
	status, offered_at, expires_at, responded_at`

func scanOffer(row pgx.Row) (Offer, error) {
	var o Offer
	err := row.Scan(&o.ID, &o.BookingID, &o.GuideID, &o.BatchSeq, &o.Score, &o.Features,
		&o.Status, &o.OfferedAt, &o.ExpiresAt, &o.RespondedAt)
	return o, err
}

// OfferView is the guide-inbox view: the offer plus the booking summary a
// guide needs to decide (§13.4 GET /me/guide/offers).
type OfferView struct {
	Offer
	BookingReference string     `json:"booking_reference"`
	PackageName      string     `json:"package_name"`
	DurationMinutes  int        `json:"duration_minutes"`
	StartsAt         time.Time  `json:"starts_at"`
	EndsAt           *time.Time `json:"ends_at"`
	MeetingPoint     *string    `json:"meeting_point"`
	NumGuests        int        `json:"num_guests"`
}

// Candidate is one dispatch-eligible guide with its scoring signals.
type Candidate struct {
	UserID      string
	RatingAvg   float64
	RatingCount int
	Latitude    *float64
	Longitude   *float64
	Languages   []string
	Specialties []string // codes
	Workload    int      // bookings in trailing 30 days
	OffersSeen  int
	AcceptsSeen int
}

// Repository owns dispatch persistence (explicit SQL, spec §7.2).
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository builds the repository.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// settingText reads a system_settings scalar; missing keys yield "".
func (r *Repository) settingText(ctx context.Context, key string) (string, error) {
	var v string
	err := r.pool.QueryRow(ctx,
		`SELECT value_json #>> '{}' FROM system_settings WHERE key = $1`, key).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("dispatch: read setting %q: %w", key, err)
	}
	return v, nil
}

// Candidates returns §10.2-eligible guides available for the booking window,
// excluding guides who DECLINED an earlier batch for this booking, with their
// scoring signals. Mirrors the guides.SearchRepository gate (keep in sync):
// ACTIVE certification, unsuspended account/profile, valid mandatory
// documents, no intersecting time off, no overlapping active booking. Weekly
// schedule coverage runs in Go via availability.Covered; Redis presence is
// applied by the service when the booking is "available now".
func (r *Repository) Candidates(ctx context.Context, b bookings.Booking, radiusKm float64) ([]Candidate, error) {
	args := []any{b.StartsAt, b.EndsAt, availability.BookingActiveStatuses, b.ID}
	where := `
		AND NOT EXISTS (
			SELECT 1 FROM guide_time_off t
			WHERE t.guide_id = gp.user_id
			  AND tstzrange(t.starts_at, t.ends_at, '[)') && tstzrange($1, $2, '[)'))
		AND NOT EXISTS (
			SELECT 1 FROM bookings ob
			WHERE ob.guide_id = gp.user_id
			  AND ob.status = ANY($3::text[])
			  AND tstzrange(ob.starts_at, ob.ends_at, '[)') && tstzrange($1, $2, '[)'))
		AND NOT EXISTS (
			SELECT 1 FROM dispatch_offers d
			WHERE d.guide_id = gp.user_id AND d.booking_id = $4 AND d.status = 'DECLINED')`

	// Distance filter only when the booking carries meeting coordinates; the
	// haversine expression matches guides.SearchRepository (float8 trig).
	distanceSelect := "NULL"
	if b.MeetingLatitude != nil && b.MeetingLongitude != nil {
		args = append(args, *b.MeetingLatitude, *b.MeetingLongitude, radiusKm)
		lat, lng, radius := "$5", "$6", "$7"
		distanceSelect = `6371 * 2 * ASIN(SQRT(
			POWER(SIN(RADIANS(gp.latitude::float8 - ` + lat + `::float8) / 2), 2) +
			COS(RADIANS(` + lat + `::float8)) * COS(RADIANS(gp.latitude::float8)) *
			POWER(SIN(RADIANS(gp.longitude::float8 - ` + lng + `::float8) / 2), 2)))`
		where += ` AND gp.latitude IS NOT NULL AND gp.longitude IS NOT NULL
			AND ` + distanceSelect + ` <= ` + radius
	}

	rows, err := r.pool.Query(ctx, `
		SELECT gp.user_id, gp.rating_avg::float8, gp.rating_count,
		       gp.latitude::float8, gp.longitude::float8,
		       COALESCE((SELECT array_agg(gl.language_code) FROM guide_languages gl
		                 WHERE gl.guide_id = gp.user_id), '{}'),
		       COALESCE((SELECT array_agg(s.code) FROM guide_specialties gs
		                 JOIN specialties s ON s.id = gs.specialty_id
		                 WHERE gs.guide_id = gp.user_id), '{}'),
		       (SELECT count(*) FROM bookings wb
		        WHERE wb.guide_id = gp.user_id
		          AND wb.created_at > now() - interval '30 days'),
		       COALESCE(st.offers, 0), COALESCE(st.accepts, 0),
		       `+distanceSelect+`
		FROM guide_profiles gp
		JOIN users u ON u.id = gp.user_id AND u.status = 'active'
		JOIN certification_cases cc ON cc.guide_id = gp.user_id AND cc.status = 'ACTIVE'
		LEFT JOIN guide_dispatch_stats st ON st.guide_id = gp.user_id
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
		`+where, args...)
	if err != nil {
		return nil, fmt.Errorf("dispatch: candidates: %w", err)
	}
	defer rows.Close()

	out := []Candidate{}
	for rows.Next() {
		var c Candidate
		var distance *float64
		if err := rows.Scan(&c.UserID, &c.RatingAvg, &c.RatingCount,
			&c.Latitude, &c.Longitude, &c.Languages, &c.Specialties,
			&c.Workload, &c.OffersSeen, &c.AcceptsSeen, &distance); err != nil {
			return nil, fmt.Errorf("dispatch: scan candidate: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// FilterBySchedule keeps candidates whose weekly schedule covers
// [start, end) — the same availability.Covered check search applies.
func (r *Repository) FilterBySchedule(ctx context.Context, cands []Candidate, start, end time.Time) ([]Candidate, error) {
	if len(cands) == 0 {
		return cands, nil
	}
	ids := make([]string, 0, len(cands))
	for _, c := range cands {
		ids = append(ids, c.UserID)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT guide_id, weekday, start_time::text, end_time::text, timezone
		FROM guide_availability
		WHERE guide_id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, fmt.Errorf("dispatch: load availability windows: %w", err)
	}
	defer rows.Close()

	byGuide := map[string][]availability.Window{}
	for rows.Next() {
		var gid, startS, endS string
		var w availability.Window
		if err := rows.Scan(&gid, &w.Weekday, &startS, &endS, &w.Timezone); err != nil {
			return nil, fmt.Errorf("dispatch: scan availability window: %w", err)
		}
		if w.StartMin, err = availability.ParseClock(startS); err != nil {
			return nil, err
		}
		if w.EndMin, err = availability.ParseClock(endS); err != nil {
			return nil, err
		}
		byGuide[gid] = append(byGuide[gid], w)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := []Candidate{}
	for _, c := range cands {
		if availability.Covered(byGuide[c.UserID], start, end) {
			out = append(out, c)
		}
	}
	return out, nil
}

// touristLanguage returns the booking tourist's preferred_language ("" when
// unset — language match is neutral then).
func (r *Repository) touristLanguage(ctx context.Context, touristID string) (string, error) {
	var lang *string
	err := r.pool.QueryRow(ctx,
		`SELECT preferred_language FROM tourist_profiles WHERE user_id = $1`, touristID).Scan(&lang)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("dispatch: tourist language: %w", err)
	}
	if lang == nil {
		return "", nil
	}
	return *lang, nil
}

// packageCode returns the booking's package code (specialty-match lookup).
func (r *Repository) packageCode(ctx context.Context, packageID string) (string, error) {
	var code string
	err := r.pool.QueryRow(ctx,
		`SELECT code FROM tour_packages WHERE id = $1`, packageID).Scan(&code)
	if err != nil {
		return "", fmt.Errorf("dispatch: package code: %w", err)
	}
	return code, nil
}

// NextBatchSeq returns max(batch_seq)+1 for the booking (1 when never
// dispatched).
func (r *Repository) NextBatchSeq(ctx context.Context, bookingID string) (int, error) {
	var seq int
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(batch_seq), 0) + 1 FROM dispatch_offers
		WHERE booking_id = $1`, bookingID).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("dispatch: next batch seq: %w", err)
	}
	return seq, nil
}

// LiveOffers returns the booking's OFFERED rows that have not yet expired.
func (r *Repository) LiveOffers(ctx context.Context, bookingID string) ([]Offer, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+offerColumns+`
		FROM dispatch_offers
		WHERE booking_id = $1 AND status = 'OFFERED' AND expires_at > now()
		ORDER BY score DESC, guide_id`, bookingID)
	if err != nil {
		return nil, fmt.Errorf("dispatch: live offers: %w", err)
	}
	defer rows.Close()
	out := []Offer{}
	for rows.Next() {
		o, err := scanOffer(rows)
		if err != nil {
			return nil, fmt.Errorf("dispatch: scan live offer: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// ListForGuide returns one guide's offer inbox view, OFFERED-unexpired first
// then recent history (bounded — a guide's offer list is low-volume).
func (r *Repository) ListForGuide(ctx context.Context, guideID string, liveOnly bool, limit int) ([]OfferView, error) {
	where := `WHERE o.guide_id = $1`
	if liveOnly {
		where += ` AND o.status = 'OFFERED' AND o.expires_at > now()`
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+offerColumnsPrefixed("o")+`,
		       b.reference, tp.name, tp.duration_minutes, b.starts_at, b.ends_at,
		       b.meeting_point_text, b.num_guests
		FROM dispatch_offers o
		JOIN bookings b ON b.id = o.booking_id
		JOIN tour_packages tp ON tp.id = b.package_id
		`+where+`
		ORDER BY (o.status = 'OFFERED' AND o.expires_at > now()) DESC,
		         o.offered_at DESC
		LIMIT $2`, guideID, limit)
	if err != nil {
		return nil, fmt.Errorf("dispatch: list guide offers: %w", err)
	}
	defer rows.Close()
	out := []OfferView{}
	for rows.Next() {
		v, err := scanOfferView(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ListForBooking returns every offer for a booking, newest batches first —
// the operations "why unmatched" view (spec §30.2).
func (r *Repository) ListForBooking(ctx context.Context, bookingID string) ([]Offer, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+offerColumns+`
		FROM dispatch_offers
		WHERE booking_id = $1
		ORDER BY batch_seq DESC, score DESC, guide_id`, bookingID)
	if err != nil {
		return nil, fmt.Errorf("dispatch: list booking offers: %w", err)
	}
	defer rows.Close()
	out := []Offer{}
	for rows.Next() {
		o, err := scanOffer(rows)
		if err != nil {
			return nil, fmt.Errorf("dispatch: scan booking offer: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// offerColumnsPrefixed is offerColumns with a table alias (for joins).
func offerColumnsPrefixed(p string) string {
	cols := []string{
		"id", "booking_id", "guide_id", "batch_seq", "score::text",
		"features", "status", "offered_at", "expires_at", "responded_at",
	}
	for i, c := range cols {
		cols[i] = p + "." + c
	}
	return cols[0] + ", " + cols[1] + ", " + cols[2] + ", " + cols[3] + ", " +
		cols[4] + ", " + cols[5] + ", " + cols[6] + ", " + cols[7] + ", " +
		cols[8] + ", " + cols[9]
}

func scanOfferView(row pgx.Row) (OfferView, error) {
	var v OfferView
	err := row.Scan(&v.ID, &v.BookingID, &v.GuideID, &v.BatchSeq, &v.Score, &v.Features,
		&v.Status, &v.OfferedAt, &v.ExpiresAt, &v.RespondedAt,
		&v.BookingReference, &v.PackageName, &v.DurationMinutes, &v.StartsAt, &v.EndsAt,
		&v.MeetingPoint, &v.NumGuests)
	return v, err
}

// InsertOffer writes one scored offer row and bumps the guide's offers
// counter in the same transaction.
func (r *Repository) InsertOffer(ctx context.Context, tx pgx.Tx, o Offer, features Features) (Offer, error) {
	raw, err := json.Marshal(features)
	if err != nil {
		return Offer{}, fmt.Errorf("dispatch: marshal features: %w", err)
	}
	ins, err := scanOffer(tx.QueryRow(ctx, `
		INSERT INTO dispatch_offers (booking_id, guide_id, batch_seq, score, features, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+offerColumns,
		o.BookingID, o.GuideID, o.BatchSeq, o.Score, raw, o.ExpiresAt))
	if err != nil {
		return Offer{}, fmt.Errorf("dispatch: insert offer: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO guide_dispatch_stats (guide_id, offers)
		VALUES ($1, 1)
		ON CONFLICT (guide_id)
		DO UPDATE SET offers = guide_dispatch_stats.offers + 1, updated_at = now()`, o.GuideID); err != nil {
		return Offer{}, fmt.Errorf("dispatch: bump offers stat: %w", err)
	}
	return ins, nil
}

// GetOfferForUpdate locks and returns one offer inside tx.
func (r *Repository) GetOfferForUpdate(ctx context.Context, tx pgx.Tx, offerID string) (Offer, error) {
	o, err := scanOffer(tx.QueryRow(ctx, `
		SELECT `+offerColumns+`
		FROM dispatch_offers WHERE id = $1 FOR UPDATE`, offerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Offer{}, ErrOfferNotFound
	}
	if err != nil {
		return Offer{}, fmt.Errorf("dispatch: lock offer: %w", err)
	}
	return o, nil
}

// BookingForUpdate locks and returns the booking row inside tx (assignment
// decision reads: status + guide_id + schedule).
func (r *Repository) BookingForUpdate(ctx context.Context, tx pgx.Tx, bookingID string) (bookings.Booking, error) {
	var b bookings.Booking
	err := tx.QueryRow(ctx, `
		SELECT id, reference, tourist_id, guide_id, package_id, starts_at, ends_at,
		       status, meeting_point_text, meeting_latitude::text, meeting_longitude::text,
		       num_guests, notes, amount::text, currency, created_at, updated_at
		FROM bookings WHERE id = $1 FOR UPDATE`, bookingID).
		Scan(&b.ID, &b.Reference, &b.TouristID, &b.GuideID, &b.PackageID,
			&b.StartsAt, &b.EndsAt, &b.Status, &b.MeetingPointText, &b.MeetingLatitude,
			&b.MeetingLongitude, &b.NumGuests, &b.Notes, &b.Amount, &b.Currency,
			&b.CreatedAt, &b.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return bookings.Booking{}, bookings.ErrNotFound
	}
	if err != nil {
		return bookings.Booking{}, fmt.Errorf("dispatch: lock booking: %w", err)
	}
	return b, nil
}

// AssignGuide sets the booking's guide (acceptance) inside tx. The
// bookings_no_guide_overlap exclusion constraint is the race backstop: a
// conflicting assignment surfaces as ErrOfferClosed's overlap variant.
func (r *Repository) AssignGuide(ctx context.Context, tx pgx.Tx, bookingID, guideID string) error {
	if _, err := tx.Exec(ctx, `
		UPDATE bookings SET guide_id = $2, updated_at = now()
		WHERE id = $1`, bookingID, guideID); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23P01" {
			return bookings.ErrOverlap
		}
		return fmt.Errorf("dispatch: assign guide: %w", err)
	}
	return nil
}

// SetOfferStatus updates an offer's terminal status inside tx and bumps the
// matching reliability counter (accepts/declines/expiries).
func (r *Repository) SetOfferStatus(ctx context.Context, tx pgx.Tx, offerID, guideID, status string) error {
	if _, err := tx.Exec(ctx, `
		UPDATE dispatch_offers SET status = $2, responded_at = now()
		WHERE id = $1`, offerID, status); err != nil {
		return fmt.Errorf("dispatch: set offer %s: %w", status, err)
	}
	column := map[string]string{
		OfferAccepted: "accepts",
		OfferDeclined: "declines",
		OfferExpired:  "expiries",
	}[status]
	if column == "" {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO guide_dispatch_stats (guide_id, `+column+`)
		VALUES ($1, 1)
		ON CONFLICT (guide_id)
		DO UPDATE SET `+column+` = guide_dispatch_stats.`+column+` + 1, updated_at = now()`, guideID); err != nil {
		return fmt.Errorf("dispatch: bump %s stat: %w", column, err)
	}
	return nil
}

// SupersedeOthers marks every other live offer on the booking SUPERSEDED
// (the acceptance loser set) inside tx, returning the losing offer ids.
func (r *Repository) SupersedeOthers(ctx context.Context, tx pgx.Tx, bookingID, winnerOfferID string) ([]Offer, error) {
	rows, err := tx.Query(ctx, `
		UPDATE dispatch_offers SET status = 'SUPERSEDED', responded_at = now()
		WHERE booking_id = $1 AND id <> $2 AND status = 'OFFERED'
		RETURNING `+offerColumns, bookingID, winnerOfferID)
	if err != nil {
		return nil, fmt.Errorf("dispatch: supersede others: %w", err)
	}
	defer rows.Close()
	out := []Offer{}
	for rows.Next() {
		o, err := scanOffer(rows)
		if err != nil {
			return nil, fmt.Errorf("dispatch: scan superseded: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// ExpireStale marks every live offer past its deadline EXPIRED, bumping the
// guides' expiries counters, and returns the expired rows. The sweeper
// (Service.ExpireOffers) and lazy read paths share it.
func (r *Repository) ExpireStale(ctx context.Context) ([]Offer, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("dispatch: begin expire: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after Commit

	rows, err := tx.Query(ctx, `
		UPDATE dispatch_offers SET status = 'EXPIRED', responded_at = now()
		WHERE status = 'OFFERED' AND expires_at <= now()
		RETURNING `+offerColumns)
	if err != nil {
		return nil, fmt.Errorf("dispatch: expire stale: %w", err)
	}
	expired := []Offer{}
	for rows.Next() {
		o, err := scanOffer(rows)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("dispatch: scan expired: %w", err)
		}
		expired = append(expired, o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, o := range expired {
		if _, err := tx.Exec(ctx, `
			INSERT INTO guide_dispatch_stats (guide_id, expiries)
			VALUES ($1, 1)
			ON CONFLICT (guide_id)
			DO UPDATE SET expiries = guide_dispatch_stats.expiries + 1, updated_at = now()`, o.GuideID); err != nil {
			return nil, fmt.Errorf("dispatch: bump expiries stat: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("dispatch: commit expire: %w", err)
	}
	return expired, nil
}
