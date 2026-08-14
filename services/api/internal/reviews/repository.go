// Package reviews implements the verified review flow (spec §4.4, §13.5):
// one review per completed booking, Appendix B tags, rating aggregation and
// the quality thresholds that open low-rating (retraining) and Elite
// qualification flags.
package reviews

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Review is a reviews row joined with its Appendix B tags. The public view
// deliberately carries no tourist identity (spec §14 — expose only what is
// needed).
type Review struct {
	ID        string    `json:"id"`
	BookingID string    `json:"-"`
	GuideID   string    `json:"-"`
	Rating    int       `json:"rating"`
	Body      *string   `json:"body"`
	Tags      []string  `json:"tags"`
	CreatedAt time.Time `json:"created_at"`
}

// Repository is the reviews data layer.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository builds the repository.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// Create inserts one review plus its tags in a single transaction. The
// UNIQUE(booking_id) constraint enforces one-review-per-booking; a conflict
// maps to ErrAlreadyReviewed.
func (r *Repository) Create(ctx context.Context, bookingID, touristID, guideID string, rating int, body *string, tags []string) (Review, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Review{}, fmt.Errorf("reviews: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var rev Review
	err = tx.QueryRow(ctx,
		`INSERT INTO reviews (booking_id, tourist_id, guide_id, rating, body)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, booking_id, guide_id, rating, body, created_at`,
		bookingID, touristID, guideID, rating, body).
		Scan(&rev.ID, &rev.BookingID, &rev.GuideID, &rev.Rating, &rev.Body, &rev.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Review{}, ErrAlreadyReviewed
		}
		return Review{}, fmt.Errorf("reviews: insert: %w", err)
	}

	for _, tag := range tags {
		if _, err := tx.Exec(ctx,
			`INSERT INTO review_tags (review_id, tag) VALUES ($1, $2)`, rev.ID, tag); err != nil {
			return Review{}, fmt.Errorf("reviews: insert tag: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Review{}, fmt.Errorf("reviews: commit: %w", err)
	}
	rev.Tags = tags
	return rev, nil
}

// ListByGuide returns a guide's reviews, newest first, offset-paginated
// (per-guide volume is low — spec §14).
func (r *Repository) ListByGuide(ctx context.Context, guideID string, limit, offset int) ([]Review, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT r.id, r.booking_id, r.guide_id, r.rating, r.body, r.created_at,
		        COALESCE(array_agg(t.tag) FILTER (WHERE t.tag IS NOT NULL), '{}') AS tags
		 FROM reviews r
		 LEFT JOIN review_tags t ON t.review_id = r.id
		 WHERE r.guide_id = $1
		 GROUP BY r.id
		 ORDER BY r.created_at DESC
		 LIMIT $2 OFFSET $3`, guideID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("reviews: list: %w", err)
	}
	defer rows.Close()

	var out []Review
	for rows.Next() {
		var rev Review
		if err := rows.Scan(&rev.ID, &rev.BookingID, &rev.GuideID, &rev.Rating, &rev.Body, &rev.CreatedAt, &rev.Tags); err != nil {
			return nil, fmt.Errorf("reviews: scan: %w", err)
		}
		out = append(out, rev)
	}
	return out, rows.Err()
}

// Aggregate recomputes a guide's rating average and count from the reviews
// table — the table is authoritative, guide_profiles.rating_* is the cache.
func (r *Repository) Aggregate(ctx context.Context, guideID string) (avg float64, count int, err error) {
	err = r.pool.QueryRow(ctx,
		`SELECT COALESCE(AVG(rating), 0)::float8, COUNT(*)::int
		 FROM reviews WHERE guide_id = $1`, guideID).Scan(&avg, &count)
	if err != nil {
		return 0, 0, fmt.Errorf("reviews: aggregate: %w", err)
	}
	return avg, count, nil
}

// RefreshGuideRating rewrites the cached aggregate on guide_profiles.
func (r *Repository) RefreshGuideRating(ctx context.Context, guideID string, avg float64, count int) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE guide_profiles SET rating_avg = $2, rating_count = $3, updated_at = now()
		 WHERE user_id = $1`, guideID, avg, count)
	if err != nil {
		return fmt.Errorf("reviews: refresh guide rating: %w", err)
	}
	return nil
}

// CompletedTours counts a guide's COMPLETED bookings (Elite qualification
// input, spec §4.4).
func (r *Repository) CompletedTours(ctx context.Context, guideID string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*)::int FROM bookings WHERE guide_id = $1 AND status = 'COMPLETED'`, guideID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("reviews: completed tours: %w", err)
	}
	return n, nil
}

// OpenQualityFlag opens a quality flag unless one is already open for
// (guide, kind); the partial unique index is the race guard. Returns true
// when a flag was created.
func (r *Repository) OpenQualityFlag(ctx context.Context, guideID, kind string, avg float64, detail string) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`INSERT INTO quality_flags (guide_id, kind, rating_avg_at_flag, detail)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (guide_id, kind) WHERE status = 'open' DO NOTHING`,
		guideID, kind, avg, detail)
	if err != nil {
		return false, fmt.Errorf("reviews: open quality flag: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// SettingText returns the raw scalar text of a system_settings JSON value
// (same convention as bookings). Missing key → empty string, no error: every
// quality setting has a documented default.
func (r *Repository) SettingText(ctx context.Context, key string) (string, error) {
	var v string
	err := r.pool.QueryRow(ctx,
		`SELECT value_json #>> '{}' FROM system_settings WHERE key = $1`, key).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reviews: read setting %q: %w", key, err)
	}
	return v, nil
}
