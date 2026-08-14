// Package incidents implements the operations incident workflow (spec §12,
// P6-03): listing, acknowledgement, notes, escalation, assignment,
// resolution and closure — every action timestamped and attributed in the
// append-only incident_events trail. It also owns the quality-flag queue
// (spec §4.4, P6-06).
package incidents

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Incident is an incidents row.
type Incident struct {
	ID         string    `json:"id"`
	BookingID  *string   `json:"booking_id"`
	Type       string    `json:"type"`
	Severity   string    `json:"severity"`
	Status     string    `json:"status"`
	ReportedBy *string   `json:"reported_by"`
	AssignedTo *string   `json:"assigned_to"`
	OccurredAt time.Time `json:"occurred_at"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Event is one append-only incident_events trail row.
type Event struct {
	ID         string    `json:"id"`
	IncidentID string    `json:"incident_id"`
	ActorID    *string   `json:"actor_id"`
	Kind       string    `json:"kind"`
	Body       *string   `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
}

// QualityFlag is one quality_flags row (spec §4.4).
type QualityFlag struct {
	ID              string     `json:"id"`
	GuideID         string     `json:"guide_id"`
	GuideName       *string    `json:"guide_name"`
	Kind            string     `json:"kind"`
	Status          string     `json:"status"`
	RatingAvgAtFlag float64    `json:"rating_avg_at_flag"`
	Detail          *string    `json:"detail"`
	CreatedAt       time.Time  `json:"created_at"`
	ResolvedAt      *time.Time `json:"resolved_at"`
	ResolvedBy      *string    `json:"resolved_by"`
	ResolutionNote  *string    `json:"resolution_note"`
}

// ListFilter narrows the incident list.
type ListFilter struct {
	Status   string
	Severity string
	Type     string
}

// Sentinel errors mapped by the handler.
var (
	// ErrNotFound — no such incident/flag.
	ErrNotFound = errors.New("incidents: not found")
	// ErrIllegalTransition — status move the workflow does not allow.
	ErrIllegalTransition = errors.New("incidents: illegal status transition")
	// ErrAlreadyResolved — quality flag already resolved.
	ErrAlreadyResolved = errors.New("incidents: quality flag already resolved")
)

// Repository is the incidents data layer.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository builds the repository.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

const incidentCols = `id, booking_id, type, severity, status, reported_by, assigned_to, occurred_at, created_at, updated_at`

// List returns incidents, offset-paginated (low-volume admin list, §14),
// most recent first.
func (r *Repository) List(ctx context.Context, f ListFilter, limit, offset int) ([]Incident, int, error) {
	where := "WHERE TRUE"
	args := []any{}
	add := func(clause string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(" AND %s$%d", clause, len(args))
	}
	if f.Status != "" {
		add("status = ", f.Status)
	}
	if f.Severity != "" {
		add("severity = ", f.Severity)
	}
	if f.Type != "" {
		add("type = ", f.Type)
	}

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM incidents `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("incidents: count: %w", err)
	}

	args = append(args, limit, offset)
	rows, err := r.pool.Query(ctx,
		`SELECT `+incidentCols+` FROM incidents `+where+
			fmt.Sprintf(" ORDER BY occurred_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("incidents: list: %w", err)
	}
	defer rows.Close()

	var out []Incident
	for rows.Next() {
		inc, err := scan(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, inc)
	}
	return out, total, rows.Err()
}

// GetByID loads one incident. ErrNotFound when absent.
func (r *Repository) GetByID(ctx context.Context, id string) (Incident, error) {
	inc, err := scan(r.pool.QueryRow(ctx,
		`SELECT `+incidentCols+` FROM incidents WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Incident{}, ErrNotFound
	}
	if err != nil {
		return Incident{}, fmt.Errorf("incidents: get: %w", err)
	}
	return inc, nil
}

// ListEvents returns the incident's audit trail, oldest first.
func (r *Repository) ListEvents(ctx context.Context, incidentID string) ([]Event, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, incident_id, actor_id, kind, body, created_at
		 FROM incident_events WHERE incident_id = $1 ORDER BY created_at`, incidentID)
	if err != nil {
		return nil, fmt.Errorf("incidents: events: %w", err)
	}
	defer rows.Close()

	var out []Event
	for rows.Next() {
		var ev Event
		if err := rows.Scan(&ev.ID, &ev.IncidentID, &ev.ActorID, &ev.Kind, &ev.Body, &ev.CreatedAt); err != nil {
			return nil, fmt.Errorf("incidents: scan event: %w", err)
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

// Apply performs one workflow mutation — status/severity/assignment change
// plus its trail event — atomically. Nil fields stay unchanged.
func (r *Repository) Apply(ctx context.Context, id, actorID string, status, severity, assignedTo *string, kind string, body *string) (Incident, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Incident{}, fmt.Errorf("incidents: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	inc, err := scan(tx.QueryRow(ctx,
		`UPDATE incidents SET
		   status      = COALESCE($2, status),
		   severity    = COALESCE($3, severity),
		   assigned_to = COALESCE($4, assigned_to),
		   updated_at  = now()
		 WHERE id = $1
		 RETURNING `+incidentCols, id, status, severity, assignedTo))
	if errors.Is(err, pgx.ErrNoRows) {
		return Incident{}, ErrNotFound
	}
	if err != nil {
		return Incident{}, fmt.Errorf("incidents: update: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO incident_events (incident_id, actor_id, kind, body) VALUES ($1, $2, $3, $4)`,
		id, actorID, kind, body); err != nil {
		return Incident{}, fmt.Errorf("incidents: insert event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Incident{}, fmt.Errorf("incidents: commit: %w", err)
	}
	return inc, nil
}

// ListFlags returns quality flags, open first, offset-paginated.
func (r *Repository) ListFlags(ctx context.Context, status string, limit, offset int) ([]QualityFlag, error) {
	where := "WHERE TRUE"
	args := []any{}
	if status != "" {
		args = append(args, status)
		where = "WHERE q.status = $1"
	}
	args = append(args, limit, offset)
	rows, err := r.pool.Query(ctx,
		fmt.Sprintf(
			`SELECT q.id, q.guide_id, gp.public_name, q.kind, q.status,
			        q.rating_avg_at_flag::float8, q.detail, q.created_at,
			        q.resolved_at, q.resolved_by, q.resolution_note
			 FROM quality_flags q
			 JOIN guide_profiles gp ON gp.user_id = q.guide_id
			 %s
			 ORDER BY q.status = 'open' DESC, q.created_at DESC
			 LIMIT $%d OFFSET $%d`, where, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("incidents: list flags: %w", err)
	}
	defer rows.Close()

	var out []QualityFlag
	for rows.Next() {
		var f QualityFlag
		if err := rows.Scan(&f.ID, &f.GuideID, &f.GuideName, &f.Kind, &f.Status,
			&f.RatingAvgAtFlag, &f.Detail, &f.CreatedAt,
			&f.ResolvedAt, &f.ResolvedBy, &f.ResolutionNote); err != nil {
			return nil, fmt.Errorf("incidents: scan flag: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ResolveFlag closes one open quality flag with a resolution note.
func (r *Repository) ResolveFlag(ctx context.Context, id, actorID, note string) (QualityFlag, error) {
	var f QualityFlag
	err := r.pool.QueryRow(ctx,
		`UPDATE quality_flags SET status = 'resolved', resolved_at = now(),
		   resolved_by = $2, resolution_note = $3
		 WHERE id = $1 AND status = 'open'
		 RETURNING id, guide_id, NULL, kind, status, rating_avg_at_flag::float8,
		           detail, created_at, resolved_at, resolved_by, resolution_note`,
		id, actorID, note).
		Scan(&f.ID, &f.GuideID, &f.GuideName, &f.Kind, &f.Status,
			&f.RatingAvgAtFlag, &f.Detail, &f.CreatedAt,
			&f.ResolvedAt, &f.ResolvedBy, &f.ResolutionNote)
	if errors.Is(err, pgx.ErrNoRows) {
		return QualityFlag{}, ErrAlreadyResolved
	}
	if err != nil {
		return QualityFlag{}, fmt.Errorf("incidents: resolve flag: %w", err)
	}
	return f, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scan(row scanner) (Incident, error) {
	var inc Incident
	err := row.Scan(&inc.ID, &inc.BookingID, &inc.Type, &inc.Severity, &inc.Status,
		&inc.ReportedBy, &inc.AssignedTo, &inc.OccurredAt, &inc.CreatedAt, &inc.UpdatedAt)
	return inc, err
}
