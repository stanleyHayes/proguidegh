package certification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Case is a certification_cases row — the state machine root (spec §8.1).
type Case struct {
	ID          string     `json:"id"`
	GuideID     string     `json:"guide_id"`
	Status      string     `json:"status"`
	AssignedTo  *string    `json:"assigned_to"`
	OpenedAt    time.Time  `json:"opened_at"`
	CompletedAt *time.Time `json:"completed_at"`
}

// Event is an immutable certification_events history row.
type Event struct {
	ID          string    `json:"id"`
	CaseID      string    `json:"case_id"`
	FromStatus  *string   `json:"from_status"`
	ToStatus    string    `json:"to_status"`
	ActorID     *string   `json:"actor_id"`
	Reason      *string   `json:"reason"`
	EvidenceRef *string   `json:"evidence_ref"`
	CreatedAt   time.Time `json:"created_at"`
}

// QueueRow is one row of the admin certification queue: the case plus the
// guide identity a verifier needs to work it.
type QueueRow struct {
	Case
	PublicName  string `json:"public_name"`
	Email       string `json:"email"`
	GuideStatus string `json:"guide_status"`
}

// Document is a guide_documents metadata row (evidence for pipeline stages).
type Document struct {
	ID        string     `json:"id"`
	GuideID   string     `json:"guide_id"`
	Type      string     `json:"type"`
	ObjectKey string     `json:"object_key"`
	Status    string     `json:"status"`
	ExpiresAt *time.Time `json:"expires_at"`
	CreatedAt time.Time  `json:"created_at"`
}

// Sentinel errors mapped to HTTP statuses by the handler.
var (
	// ErrCaseNotFound — no case with the given id (or no case for the guide).
	ErrCaseNotFound = errors.New("certification: case not found")
	// ErrUnknownStatus — target status is not part of the state machine.
	ErrUnknownStatus = errors.New("certification: unknown status")
	// ErrIllegalTransition — from → to is not a legal state machine edge.
	ErrIllegalTransition = errors.New("certification: illegal transition")
	// ErrEvidenceRequired — target status requires evidence (spec §5 stage
	// table) and the reference/document check failed.
	ErrEvidenceRequired = errors.New("certification: evidence required")
)

const caseColumns = `id, guide_id, status, assigned_to, opened_at, completed_at`

// Repository owns certification persistence (explicit SQL, spec §7.2).
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository builds the repository.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func scanCase(row pgx.Row) (Case, error) {
	var c Case
	err := row.Scan(&c.ID, &c.GuideID, &c.Status, &c.AssignedTo, &c.OpenedAt, &c.CompletedAt)
	return c, err
}

// CurrentCase returns the guide's latest case (by opened_at).
func (r *Repository) CurrentCase(ctx context.Context, guideID string) (Case, error) {
	c, err := scanCase(r.pool.QueryRow(ctx, `
		SELECT `+caseColumns+`
		FROM certification_cases
		WHERE guide_id = $1
		ORDER BY opened_at DESC
		LIMIT 1`, guideID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Case{}, ErrCaseNotFound
	}
	if err != nil {
		return Case{}, fmt.Errorf("certification: current case: %w", err)
	}
	return c, nil
}

// GetCase returns a case by id.
func (r *Repository) GetCase(ctx context.Context, caseID string) (Case, error) {
	c, err := scanCase(r.pool.QueryRow(ctx, `
		SELECT `+caseColumns+`
		FROM certification_cases WHERE id = $1`, caseID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Case{}, ErrCaseNotFound
	}
	if err != nil {
		return Case{}, fmt.Errorf("certification: get case: %w", err)
	}
	return c, nil
}

// OpenCase creates a case in APPLIED with its opening event when the guide
// has no case yet; repeats are no-ops (idempotent application, spec §1.2).
// Returns the current case either way.
func (r *Repository) OpenCase(ctx context.Context, guideID, actorID string) (Case, error) {
	if c, err := r.CurrentCase(ctx, guideID); err == nil {
		return c, nil
	} else if !errors.Is(err, ErrCaseNotFound) {
		return Case{}, err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Case{}, fmt.Errorf("certification: begin open case: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after Commit

	c, err := scanCase(tx.QueryRow(ctx, `
		INSERT INTO certification_cases (guide_id, status)
		VALUES ($1, '`+StatusApplied+`')
		ON CONFLICT DO NOTHING
		RETURNING `+caseColumns, guideID))
	if errors.Is(err, pgx.ErrNoRows) {
		// Partial unique index race: another request opened the case first.
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return Case{}, fmt.Errorf("certification: rollback open case race: %w", rbErr)
		}
		return r.CurrentCase(ctx, guideID)
	}
	if err != nil {
		return Case{}, fmt.Errorf("certification: insert case: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO certification_events (case_id, from_status, to_status, actor_id, reason)
		VALUES ($1, NULL, $2, $3, 'application submitted')`,
		c.ID, StatusApplied, actorID); err != nil {
		return Case{}, fmt.Errorf("certification: opening event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Case{}, fmt.Errorf("certification: commit open case: %w", err)
	}
	return c, nil
}

// ListQueue returns one page of cases, newest first, with guide identity.
// status empty = all statuses.
func (r *Repository) ListQueue(ctx context.Context, status string, limit, offset int) ([]QueueRow, int, error) {
	where := ""
	countWhere := ""
	args := []any{limit, offset}
	if status != "" {
		where = "WHERE cc.status = $3"
		countWhere = "WHERE cc.status = $1"
		args = append(args, status)
	}

	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM certification_cases cc `+countWhere, args[2:]...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("certification: count queue: %w", err)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT cc.id, cc.guide_id, cc.status, cc.assigned_to, cc.opened_at, cc.completed_at,
		       gp.public_name, u.email, gp.status
		FROM certification_cases cc
		JOIN guide_profiles gp ON gp.user_id = cc.guide_id
		JOIN users u ON u.id = cc.guide_id
		`+where+`
		ORDER BY cc.opened_at DESC
		LIMIT $1 OFFSET $2`, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("certification: list queue: %w", err)
	}
	defer rows.Close()

	out := []QueueRow{}
	for rows.Next() {
		var q QueueRow
		if err := rows.Scan(&q.ID, &q.GuideID, &q.Status, &q.AssignedTo, &q.OpenedAt,
			&q.CompletedAt, &q.PublicName, &q.Email, &q.GuideStatus); err != nil {
			return nil, 0, fmt.Errorf("certification: scan queue row: %w", err)
		}
		out = append(out, q)
	}
	return out, total, rows.Err()
}

// ListEvents returns a case's immutable history, oldest first.
func (r *Repository) ListEvents(ctx context.Context, caseID string) ([]Event, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, case_id, from_status, to_status, actor_id, reason, evidence_ref, created_at
		FROM certification_events
		WHERE case_id = $1
		ORDER BY created_at, id`, caseID)
	if err != nil {
		return nil, fmt.Errorf("certification: list events: %w", err)
	}
	defer rows.Close()

	out := []Event{}
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.CaseID, &e.FromStatus, &e.ToStatus,
			&e.ActorID, &e.Reason, &e.EvidenceRef, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("certification: scan event: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListDocuments returns a guide's document metadata (newest first).
func (r *Repository) ListDocuments(ctx context.Context, guideID string) ([]Document, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, guide_id, type, object_key, status, expires_at, created_at
		FROM guide_documents
		WHERE guide_id = $1
		ORDER BY created_at DESC`, guideID)
	if err != nil {
		return nil, fmt.Errorf("certification: list documents: %w", err)
	}
	defer rows.Close()

	out := []Document{}
	for rows.Next() {
		var d Document
		if err := rows.Scan(&d.ID, &d.GuideID, &d.Type, &d.ObjectKey,
			&d.Status, &d.ExpiresAt, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("certification: scan document: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// transitionInput carries one validated state machine move.
type transitionInput struct {
	CaseID      string
	ActorID     string
	ToStatus    string
	Reason      string
	EvidenceRef string
	IP          string
}

// Transition applies one validated transition atomically: row lock, legality
// + evidence re-check inside the transaction, status update, immutable event
// row and audit_logs row. Returns the updated case and the event.
func (r *Repository) Transition(ctx context.Context, in transitionInput) (Case, Event, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Case{}, Event{}, fmt.Errorf("certification: begin transition: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after Commit

	c, err := scanCase(tx.QueryRow(ctx, `
		SELECT `+caseColumns+`
		FROM certification_cases WHERE id = $1 FOR UPDATE`, in.CaseID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Case{}, Event{}, ErrCaseNotFound
	}
	if err != nil {
		return Case{}, Event{}, fmt.Errorf("certification: lock case: %w", err)
	}

	if !CanTransition(c.Status, in.ToStatus) {
		return Case{}, Event{}, fmt.Errorf("%w: %s -> %s", ErrIllegalTransition, c.Status, in.ToStatus)
	}

	if EvidenceRequired(in.ToStatus) {
		docs, err := documentsForTx(ctx, tx, c.GuideID)
		if err != nil {
			return Case{}, Event{}, err
		}
		if !EvidenceSatisfied(in.ToStatus, docs, time.Now()) {
			return Case{}, Event{}, fmt.Errorf("%w: %s needs a valid %v document",
				ErrEvidenceRequired, in.ToStatus, EvidenceDocTypes(in.ToStatus))
		}
	}

	// completed_at marks reaching the end states; reactivation (SUSPENDED/
	// EXPIRED -> ACTIVE) refreshes it without touching past events.
	var completedAt any
	if in.ToStatus == StatusActive || in.ToStatus == StatusRejected {
		completedAt = time.Now()
	} else {
		completedAt = c.CompletedAt
	}
	if _, err := tx.Exec(ctx, `
		UPDATE certification_cases
		SET status = $2, completed_at = $3, updated_at = now()
		WHERE id = $1`, c.ID, in.ToStatus, completedAt); err != nil {
		return Case{}, Event{}, fmt.Errorf("certification: update case status: %w", err)
	}

	var evidence any
	if in.EvidenceRef != "" {
		evidence = in.EvidenceRef
	}
	var e Event
	err = tx.QueryRow(ctx, `
		INSERT INTO certification_events (case_id, from_status, to_status, actor_id, reason, evidence_ref)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, case_id, from_status, to_status, actor_id, reason, evidence_ref, created_at`,
		c.ID, c.Status, in.ToStatus, in.ActorID, in.Reason, evidence).
		Scan(&e.ID, &e.CaseID, &e.FromStatus, &e.ToStatus, &e.ActorID,
			&e.Reason, &e.EvidenceRef, &e.CreatedAt)
	if err != nil {
		return Case{}, Event{}, fmt.Errorf("certification: insert event: %w", err)
	}

	// Keep guide_profiles.status in step so the admin guide directory stays
	// meaningful; certification_cases remains the state machine root.
	var profileStatus string
	switch in.ToStatus {
	case StatusActive:
		profileStatus = "certified"
	case StatusSuspended:
		profileStatus = "suspended"
	}
	if profileStatus != "" {
		if _, err := tx.Exec(ctx, `
			UPDATE guide_profiles SET status = $2, updated_at = now()
			WHERE user_id = $1`, c.GuideID, profileStatus); err != nil {
			return Case{}, Event{}, fmt.Errorf("certification: sync guide status: %w", err)
		}
	}
	if in.ToStatus == StatusActive {
		if _, err := tx.Exec(ctx, `
			INSERT INTO user_roles (user_id, role_id)
			SELECT $1, id FROM roles WHERE code = 'guide'
			ON CONFLICT DO NOTHING`, c.GuideID); err != nil {
			return Case{}, Event{}, fmt.Errorf("certification: grant guide role: %w", err)
		}
	}

	// Audit row inside the same transaction (spec §1.2 decision 8): a
	// transition without its audit trail must not commit.
	before, _ := json.Marshal(map[string]any{"status": c.Status})
	after, _ := json.Marshal(map[string]any{
		"status": in.ToStatus, "reason": in.Reason, "evidence_ref": in.EvidenceRef,
	})
	var ip any
	if parsed := net.ParseIP(in.IP); parsed != nil {
		ip = parsed.String()
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_logs (actor_id, action, entity_type, entity_id, before_json, after_json, ip)
		VALUES ($1, 'certification.transition', 'certification_case', $2, $3, $4, $5)`,
		in.ActorID, c.ID, before, after, ip); err != nil {
		return Case{}, Event{}, fmt.Errorf("certification: audit transition: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Case{}, Event{}, fmt.Errorf("certification: commit transition: %w", err)
	}

	c.Status = in.ToStatus
	if t, ok := completedAt.(time.Time); ok {
		c.CompletedAt = &t
	}
	return c, e, nil
}

func documentsForTx(ctx context.Context, tx pgx.Tx, guideID string) ([]DocInput, error) {
	rows, err := tx.Query(ctx, `
		SELECT type, status, expires_at FROM guide_documents WHERE guide_id = $1`, guideID)
	if err != nil {
		return nil, fmt.Errorf("certification: load evidence documents: %w", err)
	}
	defer rows.Close()

	out := []DocInput{}
	for rows.Next() {
		var d DocInput
		if err := rows.Scan(&d.Type, &d.Status, &d.ExpiresAt); err != nil {
			return nil, fmt.Errorf("certification: scan evidence document: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
