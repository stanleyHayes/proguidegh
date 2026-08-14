package availability

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier is satisfied by both *pgxpool.Pool and pgx.Tx so the same queries
// can run standalone or inside a caller's transaction (booking creation
// re-checks time-off inside its own tx via this package).
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// ErrTimeOffNotFound is returned when deleting a time-off row that does not
// exist (or belongs to another guide).
var ErrTimeOffNotFound = errors.New("availability: time off not found")

// TimeOff is a guide_time_off row.
type TimeOff struct {
	ID        string    `json:"id"`
	GuideID   string    `json:"guide_id"`
	StartsAt  time.Time `json:"starts_at"`
	EndsAt    time.Time `json:"ends_at"`
	Reason    *string   `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

// WindowInput is one validated weekly window for ReplaceSchedule.
type WindowInput struct {
	Weekday  int    `json:"weekday"`
	StartMin int    `json:"-"`
	EndMin   int    `json:"-"`
	Timezone string `json:"timezone"`
}

// WindowJSON renders a Window for API responses.
type WindowJSON struct {
	Weekday   int    `json:"weekday"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	Timezone  string `json:"timezone"`
}

// Repository owns guide_availability / guide_time_off persistence (explicit
// SQL, spec §7.2).
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository builds the repository.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// ListWindows returns the guide's weekly schedule as parsed windows.
func (r *Repository) ListWindows(ctx context.Context, guideID string) ([]Window, error) {
	return listWindows(ctx, r.pool, guideID)
}

func listWindows(ctx context.Context, q Querier, guideID string) ([]Window, error) {
	rows, err := q.Query(ctx, `
		SELECT weekday, start_time::text, end_time::text, timezone
		FROM guide_availability
		WHERE guide_id = $1
		ORDER BY weekday, start_time`, guideID)
	if err != nil {
		return nil, fmt.Errorf("availability: list windows: %w", err)
	}
	defer rows.Close()

	out := []Window{}
	for rows.Next() {
		var w Window
		var startS, endS string
		if err := rows.Scan(&w.Weekday, &startS, &endS, &w.Timezone); err != nil {
			return nil, fmt.Errorf("availability: scan window: %w", err)
		}
		if w.StartMin, err = ParseClock(startS); err != nil {
			return nil, err
		}
		if w.EndMin, err = ParseClock(endS); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// ListWindowsJSON returns the schedule in API-response shape.
func (r *Repository) ListWindowsJSON(ctx context.Context, guideID string) ([]WindowJSON, error) {
	ws, err := r.ListWindows(ctx, guideID)
	if err != nil {
		return nil, err
	}
	out := make([]WindowJSON, 0, len(ws))
	for _, w := range ws {
		out = append(out, WindowJSON{
			Weekday:   w.Weekday,
			StartTime: Clock(w.StartMin),
			EndTime:   Clock(w.EndMin),
			Timezone:  w.Timezone,
		})
	}
	return out, nil
}

// ReplaceSchedule atomically replaces the guide's weekly windows (DELETE +
// INSERT in one transaction, spec §4.2's "set availability" semantics).
// Inputs must be pre-validated by the caller (weekday 0-6, end > start,
// loadable timezone).
func (r *Repository) ReplaceSchedule(ctx context.Context, guideID string, windows []WindowInput) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("availability: begin replace schedule: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after Commit

	if _, err := tx.Exec(ctx, `DELETE FROM guide_availability WHERE guide_id = $1`, guideID); err != nil {
		return fmt.Errorf("availability: clear schedule: %w", err)
	}
	for _, w := range windows {
		if _, err := tx.Exec(ctx, `
			INSERT INTO guide_availability (guide_id, weekday, start_time, end_time, timezone)
			VALUES ($1, $2, $3, $4, $5)`,
			guideID, w.Weekday, Clock(w.StartMin), Clock(w.EndMin), w.Timezone); err != nil {
			return fmt.Errorf("availability: insert window: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("availability: commit replace schedule: %w", err)
	}
	return nil
}

// AddTimeOff records a one-off unavailability block.
func (r *Repository) AddTimeOff(ctx context.Context, guideID string, startsAt, endsAt time.Time, reason *string) (TimeOff, error) {
	var t TimeOff
	err := r.pool.QueryRow(ctx, `
		INSERT INTO guide_time_off (guide_id, starts_at, ends_at, reason)
		VALUES ($1, $2, $3, $4)
		RETURNING id, guide_id, starts_at, ends_at, reason, created_at`,
		guideID, startsAt, endsAt, reason).
		Scan(&t.ID, &t.GuideID, &t.StartsAt, &t.EndsAt, &t.Reason, &t.CreatedAt)
	if err != nil {
		return TimeOff{}, fmt.Errorf("availability: add time off: %w", err)
	}
	return t, nil
}

// DeleteTimeOff removes a time-off row owned by the guide.
func (r *Repository) DeleteTimeOff(ctx context.Context, guideID, id string) error {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM guide_time_off WHERE id = $1 AND guide_id = $2`, id, guideID)
	if err != nil {
		return fmt.Errorf("availability: delete time off: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrTimeOffNotFound
	}
	return nil
}

// HasTimeOffOverlap reports whether any of the guide's time-off rows
// intersects [start, end). Runs on any Querier, so booking creation can call
// it inside its own transaction.
func HasTimeOffOverlap(ctx context.Context, q Querier, guideID string, start, end time.Time) (bool, error) {
	var overlap bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM guide_time_off
			WHERE guide_id = $1
			  AND tstzrange(starts_at, ends_at, '[)') && tstzrange($2, $3, '[)'))`,
		guideID, start, end).Scan(&overlap)
	if err != nil {
		return false, fmt.Errorf("availability: time off overlap: %w", err)
	}
	return overlap, nil
}

// AvailableAt is the full Postgres-side availability verdict for a guide at
// [start, end): the weekly schedule must cover the interval and no time-off
// row may intersect it. Used by booking creation; search applies the same
// rules in its candidate pipeline.
func (r *Repository) AvailableAt(ctx context.Context, guideID string, start, end time.Time) (bool, error) {
	windows, err := r.ListWindows(ctx, guideID)
	if err != nil {
		return false, err
	}
	if !Covered(windows, start, end) {
		return false, nil
	}
	off, err := HasTimeOffOverlap(ctx, r.pool, guideID, start, end)
	if err != nil {
		return false, err
	}
	return !off, nil
}
