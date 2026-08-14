package bookings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Booking is a bookings row (the aggregate root, spec §8.1).
type Booking struct {
	ID               string     `json:"id"`
	Reference        string     `json:"reference"`
	TouristID        string     `json:"tourist_id"`
	GuideID          *string    `json:"guide_id"`
	PackageID        string     `json:"package_id"`
	StartsAt         time.Time  `json:"starts_at"`
	EndsAt           *time.Time `json:"ends_at"`
	Status           string     `json:"status"`
	MeetingPointText *string    `json:"meeting_point"`
	MeetingLatitude  *string    `json:"meeting_latitude"`
	MeetingLongitude *string    `json:"meeting_longitude"`
	NumGuests        int        `json:"num_guests"`
	Notes            *string    `json:"notes"`
	Amount           *string    `json:"amount"`
	Currency         string     `json:"currency"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// Event is an immutable booking_status_events history row.
type Event struct {
	ID         string          `json:"id"`
	BookingID  string          `json:"booking_id"`
	FromStatus *string         `json:"from_status"`
	ToStatus   string          `json:"to_status"`
	ActorID    *string         `json:"actor_id"`
	Metadata   json.RawMessage `json:"metadata"`
	CreatedAt  time.Time       `json:"created_at"`
}

// Sentinel errors mapped to HTTP statuses by the handler.
var (
	// ErrNotFound — no booking with the given id.
	ErrNotFound = errors.New("bookings: not found")
	// ErrIllegalTransition — from -> to is not a legal state machine edge.
	ErrIllegalTransition = errors.New("bookings: illegal transition")
	// ErrOverlap — the guide already holds an active (CONFIRMED..IN_PROGRESS)
	// booking intersecting the requested interval (spec §10.2).
	ErrOverlap = errors.New("bookings: guide has an overlapping active booking")
	// ErrIdempotencyConflict — same Idempotency-Key, different payload.
	ErrIdempotencyConflict = errors.New("bookings: idempotency key reused with a different payload")
	// ErrIdempotencyInProgress — the original request with this key has not
	// completed; the client should retry shortly.
	ErrIdempotencyInProgress = errors.New("bookings: request with this idempotency key is still in progress")
)

const bookingColumns = `id, reference, tourist_id, guide_id, package_id, starts_at, ends_at,
	status, meeting_point_text, meeting_latitude::text, meeting_longitude::text,
	num_guests, notes, amount::text, currency, created_at, updated_at`

// Repository owns booking persistence (explicit SQL, spec §7.2).
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository builds the repository.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func scanBooking(row pgx.Row) (Booking, error) {
	var b Booking
	err := row.Scan(&b.ID, &b.Reference, &b.TouristID, &b.GuideID, &b.PackageID,
		&b.StartsAt, &b.EndsAt, &b.Status, &b.MeetingPointText, &b.MeetingLatitude,
		&b.MeetingLongitude, &b.NumGuests, &b.Notes, &b.Amount, &b.Currency,
		&b.CreatedAt, &b.UpdatedAt)
	return b, err
}

// GetByID returns a booking by id.
func (r *Repository) GetByID(ctx context.Context, id string) (Booking, error) {
	b, err := scanBooking(r.pool.QueryRow(ctx, `SELECT `+bookingColumns+`
		FROM bookings WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Booking{}, ErrNotFound
	}
	if err != nil {
		return Booking{}, fmt.Errorf("bookings: get by id: %w", err)
	}
	return b, nil
}

// ListByTourist returns one page of the tourist's bookings, newest first,
// keyset-paginated by (created_at, id). cursor zero-value means first page.
// limit+1 rows are fetched by the caller to detect a next page.
func (r *Repository) ListByTourist(ctx context.Context, touristID string, cursorAt time.Time, cursorID string, limit int) ([]Booking, error) {
	where := `WHERE tourist_id = $1`
	args := []any{touristID, limit}
	if !cursorAt.IsZero() {
		where += ` AND (created_at, id) < ($3, $4)`
		args = append(args, cursorAt, cursorID)
	}
	rows, err := r.pool.Query(ctx, `SELECT `+bookingColumns+`
		FROM bookings `+where+`
		ORDER BY created_at DESC, id DESC
		LIMIT $2`, args...)
	if err != nil {
		return nil, fmt.Errorf("bookings: list by tourist: %w", err)
	}
	defer rows.Close()

	out := []Booking{}
	for rows.Next() {
		var b Booking
		if err := rows.Scan(&b.ID, &b.Reference, &b.TouristID, &b.GuideID, &b.PackageID,
			&b.StartsAt, &b.EndsAt, &b.Status, &b.MeetingPointText, &b.MeetingLatitude,
			&b.MeetingLongitude, &b.NumGuests, &b.Notes, &b.Amount, &b.Currency,
			&b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, fmt.Errorf("bookings: scan booking: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// GuideBooking is one row of the guide's assignment list
// (GET /api/v1/me/guide/bookings): the booking plus the package and tourist
// display names, without history or internal fields.
type GuideBooking struct {
	ID           string     `json:"id"`
	Reference    string     `json:"reference"`
	Status       string     `json:"status"`
	PackageName  string     `json:"package_name"`
	StartsAt     time.Time  `json:"starts_at"`
	EndsAt       *time.Time `json:"ends_at"`
	MeetingPoint *string    `json:"meeting_point"`
	NumGuests    int        `json:"num_guests"`
	Amount       *string    `json:"amount"`
	Currency     string     `json:"currency"`
	TouristName  string     `json:"tourist_name"`
}

// ListByGuide returns every booking assigned to the guide, ordered for the
// guide app: upcoming tours first (starts_at ascending), then past tours
// (starts_at descending).
func (r *Repository) ListByGuide(ctx context.Context, guideID string) ([]GuideBooking, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT b.id, b.reference, b.status, p.name, b.starts_at, b.ends_at,
		       b.meeting_point_text, b.num_guests, b.amount::text, b.currency,
		       COALESCE(tp.full_name, '')
		FROM bookings b
		JOIN tour_packages p ON p.id = b.package_id
		LEFT JOIN tourist_profiles tp ON tp.user_id = b.tourist_id
		WHERE b.guide_id = $1
		ORDER BY (b.starts_at < now()),
		         CASE WHEN b.starts_at >= now() THEN b.starts_at END ASC,
		         CASE WHEN b.starts_at <  now() THEN b.starts_at END DESC,
		         b.id`, guideID)
	if err != nil {
		return nil, fmt.Errorf("bookings: list by guide: %w", err)
	}
	defer rows.Close()

	out := []GuideBooking{}
	for rows.Next() {
		var g GuideBooking
		if err := rows.Scan(&g.ID, &g.Reference, &g.Status, &g.PackageName,
			&g.StartsAt, &g.EndsAt, &g.MeetingPoint, &g.NumGuests, &g.Amount,
			&g.Currency, &g.TouristName); err != nil {
			return nil, fmt.Errorf("bookings: scan guide booking: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// ListEvents returns a booking's immutable status history, oldest first.
func (r *Repository) ListEvents(ctx context.Context, bookingID string) ([]Event, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, booking_id, from_status, to_status, actor_id, metadata, created_at
		FROM booking_status_events
		WHERE booking_id = $1
		ORDER BY created_at, id`, bookingID)
	if err != nil {
		return nil, fmt.Errorf("bookings: list events: %w", err)
	}
	defer rows.Close()

	out := []Event{}
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.BookingID, &e.FromStatus, &e.ToStatus,
			&e.ActorID, &e.Metadata, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("bookings: scan event: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// SettingText returns the raw scalar text of a system_settings JSON value
// (e.g. platform_fee_pct -> "15"). ErrNotFound when the key is missing.
func (r *Repository) SettingText(ctx context.Context, key string) (string, error) {
	var v string
	err := r.pool.QueryRow(ctx,
		`SELECT value_json #>> '{}' FROM system_settings WHERE key = $1`, key).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("bookings: system setting %q: %w", key, ErrNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("bookings: read setting %q: %w", key, err)
	}
	return v, nil
}

// CreateInput is one validated booking creation. GuideID empty means a
// marketplace booking: no guide is assigned until dispatch acceptance
// (Phase 5) and no calendar slot is held at creation.
type CreateInput struct {
	TouristID    string
	GuideID      string
	PackageID    string
	StartsAt     time.Time
	EndsAt       time.Time
	MeetingPoint *string
	MeetingLat   *string
	MeetingLng   *string
	NumGuests    int
	Notes        *string
	Amount       string // NUMERIC(14,2) text, server-computed
	Currency     string

	// Idempotency (spec §14): key is the client-supplied Idempotency-Key,
	// scope isolates it per tourist, PayloadHash is the sha256 hex of the
	// canonical request payload.
	IdempotencyKey string
	IdemScope      string
	PayloadHash    string
}

// CreateResult pairs the booking with whether the call was an idempotent
// replay of an earlier creation.
type CreateResult struct {
	Booking  Booking
	Replayed bool
}

// Create inserts the booking atomically with its idempotency claim and the
// opening state machine moves (DRAFT -> PAYMENT_PENDING), each with an
// immutable event row.
//
// Overlap guard (spec §10.2, decision documented in migration 0004): inside
// the transaction we SELECT ... FOR UPDATE the guide's overlapping ACTIVE
// (CONFIRMED..IN_PROGRESS) rows and fail with ErrOverlap when any exist. The
// bookings_no_guide_overlap exclusion constraint is the database backstop for
// races at confirm time; DRAFT/PAYMENT_PENDING holds intentionally may
// coexist until payment lands (Phase 4).
//
// Idempotency: the key is claimed inside the same transaction (INSERT ... ON
// CONFLICT DO NOTHING). A conflicting claim means the original request
// committed already — its payload hash decides replay (same booking returned)
// vs ErrIdempotencyConflict. Concurrent same-key requests serialize on the
// primary key, so exactly one of them creates the booking.
func (r *Repository) Create(ctx context.Context, in CreateInput) (CreateResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return CreateResult{}, fmt.Errorf("bookings: begin create: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after Commit

	// Claim the idempotency key first; on conflict, read the committed row.
	var claimed string
	err = tx.QueryRow(ctx, `
		INSERT INTO idempotency_keys (key, scope, response_code, response_body_hash, expires_at)
		VALUES ($1, $2, NULL, $3, now() + interval '24 hours')
		ON CONFLICT (key, scope) DO NOTHING
		RETURNING key`, in.IdempotencyKey, in.IdemScope, in.PayloadHash).Scan(&claimed)
	if errors.Is(err, pgx.ErrNoRows) {
		res, replayErr := r.replay(ctx, tx, in)
		if replayErr != nil {
			return CreateResult{}, replayErr
		}
		return res, nil
	}
	if err != nil {
		return CreateResult{}, fmt.Errorf("bookings: claim idempotency key: %w", err)
	}

	// Overlap guard (direct flow): lock the guide's overlapping active rows
	// (if any) and refuse to stack another booking on the calendar slot.
	// Marketplace bookings carry no guide yet and hold no calendar slot;
	// dispatch's acceptance path takes the overlap decision then.
	if in.GuideID != "" {
		var hasOverlap bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM bookings
				WHERE guide_id = $1
				  AND status = ANY($2::text[])
				  AND tstzrange(starts_at, ends_at, '[)') && tstzrange($3, $4, '[)')
				FOR UPDATE)`,
			in.GuideID, ActiveStatuses, in.StartsAt, in.EndsAt).Scan(&hasOverlap); err != nil {
			return CreateResult{}, fmt.Errorf("bookings: overlap check: %w", err)
		}
		if hasOverlap {
			return CreateResult{}, ErrOverlap
		}
	}

	b, err := r.insertWithEvents(ctx, tx, in)
	if err != nil {
		return CreateResult{}, err
	}

	// Complete the idempotency claim in the same transaction so the booking
	// and its replay mapping commit or roll back together.
	if _, err := tx.Exec(ctx, `
		UPDATE idempotency_keys SET entity_id = $3, response_code = 201
		WHERE key = $1 AND scope = $2`, in.IdempotencyKey, in.IdemScope, b.ID); err != nil {
		return CreateResult{}, fmt.Errorf("bookings: complete idempotency key: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return CreateResult{}, fmt.Errorf("bookings: commit create: %w", err)
	}
	return CreateResult{Booking: b}, nil
}

// replay resolves a conflicting idempotency claim: same payload hash replays
// the original booking; a different hash is a conflict; a claim without an
// entity means the original request is mid-flight.
func (r *Repository) replay(ctx context.Context, tx pgx.Tx, in CreateInput) (CreateResult, error) {
	var entityID *string
	var hash *string
	err := tx.QueryRow(ctx, `
		SELECT entity_id, response_body_hash FROM idempotency_keys
		WHERE key = $1 AND scope = $2`, in.IdempotencyKey, in.IdemScope).Scan(&entityID, &hash)
	if err != nil {
		return CreateResult{}, fmt.Errorf("bookings: read idempotency key: %w", err)
	}
	if hash == nil || *hash != in.PayloadHash {
		return CreateResult{}, ErrIdempotencyConflict
	}
	if entityID == nil {
		return CreateResult{}, ErrIdempotencyInProgress
	}
	// Roll back this transaction's work (none beyond the failed claim) and
	// read the original booking fresh from the pool.
	if rbErr := tx.Rollback(ctx); rbErr != nil {
		return CreateResult{}, fmt.Errorf("bookings: rollback replay: %w", rbErr)
	}
	b, err := r.GetByID(ctx, *entityID)
	if err != nil {
		return CreateResult{}, fmt.Errorf("bookings: replay lookup: %w", err)
	}
	return CreateResult{Booking: b, Replayed: true}, nil
}

// insertWithEvents writes the booking row plus the two opening transitions
// (NULL -> DRAFT -> PAYMENT_PENDING) with their immutable events. Reference
// collisions retry with fresh entropy.
func (r *Repository) insertWithEvents(ctx context.Context, tx pgx.Tx, in CreateInput) (Booking, error) {
	var b Booking
	for attempt := 0; attempt < 5; attempt++ {
		ref, err := newReference()
		if err != nil {
			return Booking{}, err
		}
		b, err = scanBooking(tx.QueryRow(ctx, `
			INSERT INTO bookings (reference, tourist_id, guide_id, package_id, starts_at, ends_at,
			                      status, meeting_point_text, meeting_latitude, meeting_longitude,
			                      num_guests, notes, amount, currency)
			VALUES ($1, $2, $3, $4, $5, $6, '`+StatusDraft+`', $7, $8, $9, $10, $11, $12, $13)
			RETURNING `+bookingColumns,
			ref, in.TouristID, nullableUUID(in.GuideID), in.PackageID, in.StartsAt, in.EndsAt,
			in.MeetingPoint, in.MeetingLat, in.MeetingLng,
			in.NumGuests, in.Notes, in.Amount, in.Currency))
		if isUniqueViolation(err, "reference") {
			continue
		}
		if err != nil {
			return Booking{}, fmt.Errorf("bookings: insert booking: %w", err)
		}
		break
	}
	if b.ID == "" {
		return Booking{}, fmt.Errorf("bookings: could not allocate a unique reference")
	}

	// NULL -> DRAFT opening event.
	if _, err := tx.Exec(ctx, `
		INSERT INTO booking_status_events (booking_id, from_status, to_status, actor_id, metadata)
		VALUES ($1, NULL, $2, $3, $4)`,
		b.ID, StatusDraft, in.TouristID,
		json.RawMessage(`{"action":"booking.created"}`)); err != nil {
		return Booking{}, fmt.Errorf("bookings: opening event: %w", err)
	}

	// DRAFT -> PAYMENT_PENDING: the booking awaits provider payment
	// confirmation (Phase 4). Validated through the state machine like any
	// other move.
	if _, _, err := transitionTx(ctx, tx, b.ID, in.TouristID, StatusPaymentPending,
		json.RawMessage(`{"action":"booking.payment_pending"}`)); err != nil {
		return Booking{}, err
	}
	b.Status = StatusPaymentPending
	return b, nil
}

// Transition applies one validated state machine move to a booking,
// atomically: row lock, legality check, overlap re-check when moving into
// CONFIRMED (the winner-takes-the-slot rule for competing PAYMENT_PENDING
// holds, spec §10.2), status update and immutable event row. This is the only
// write path for bookings.status outside creation; later phases (payment
// confirmation, dispatch, tour operations, cancellation) drive it.
func (r *Repository) Transition(ctx context.Context, bookingID, actorID, to string, metadata json.RawMessage) (Booking, Event, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Booking{}, Event{}, fmt.Errorf("bookings: begin transition: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after Commit

	b, e, err := transitionTx(ctx, tx, bookingID, actorID, to, metadata)
	if err != nil {
		return Booking{}, Event{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		if isExclusionViolation(err) {
			return Booking{}, Event{}, ErrOverlap
		}
		return Booking{}, Event{}, fmt.Errorf("bookings: commit transition: %w", err)
	}
	return b, e, nil
}

// TransitionTx is Transition inside the caller's transaction, for flows that
// must commit the booking move atomically with other writes (Phase 4:
// payment confirmation + ledger allocation + receipt in ONE transaction).
// It remains the only write path for bookings.status — the same legality and
// overlap checks apply.
func (r *Repository) TransitionTx(ctx context.Context, tx pgx.Tx, bookingID, actorID, to string, metadata json.RawMessage) (Booking, Event, error) {
	return transitionTx(ctx, tx, bookingID, actorID, to, metadata)
}

func transitionTx(ctx context.Context, tx pgx.Tx, bookingID, actorID, to string, metadata json.RawMessage) (Booking, Event, error) {
	b, err := scanBooking(tx.QueryRow(ctx, `SELECT `+bookingColumns+`
		FROM bookings WHERE id = $1 FOR UPDATE`, bookingID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Booking{}, Event{}, ErrNotFound
	}
	if err != nil {
		return Booking{}, Event{}, fmt.Errorf("bookings: lock booking: %w", err)
	}

	if !CanTransition(b.Status, to) {
		return Booking{}, Event{}, fmt.Errorf("%w: %s -> %s", ErrIllegalTransition, b.Status, to)
	}

	// Confirming takes the calendar slot: re-check overlap against active
	// rows (excluding self) inside the lock. The exclusion constraint is the
	// backstop for the concurrent-confirm race.
	if to == StatusConfirmed && b.GuideID != nil && b.EndsAt != nil {
		var hasOverlap bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM bookings
				WHERE guide_id = $1
				  AND id <> $2
				  AND status = ANY($3::text[])
				  AND tstzrange(starts_at, ends_at, '[)') && tstzrange($4, $5, '[)')
				FOR UPDATE)`,
			*b.GuideID, b.ID, ActiveStatuses, b.StartsAt, *b.EndsAt).Scan(&hasOverlap); err != nil {
			return Booking{}, Event{}, fmt.Errorf("bookings: confirm overlap check: %w", err)
		}
		if hasOverlap {
			return Booking{}, Event{}, ErrOverlap
		}
	}

	var actor any
	if actorID != "" {
		actor = actorID
	}
	if _, err := tx.Exec(ctx, `
		UPDATE bookings SET status = $2, updated_at = now()
		WHERE id = $1`, b.ID, to); err != nil {
		if isExclusionViolation(err) {
			return Booking{}, Event{}, ErrOverlap
		}
		return Booking{}, Event{}, fmt.Errorf("bookings: update status: %w", err)
	}

	var meta any
	if len(metadata) > 0 {
		meta = metadata
	}
	var e Event
	err = tx.QueryRow(ctx, `
		INSERT INTO booking_status_events (booking_id, from_status, to_status, actor_id, metadata)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, booking_id, from_status, to_status, actor_id, metadata, created_at`,
		b.ID, b.Status, to, actor, meta).
		Scan(&e.ID, &e.BookingID, &e.FromStatus, &e.ToStatus, &e.ActorID, &e.Metadata, &e.CreatedAt)
	if err != nil {
		return Booking{}, Event{}, fmt.Errorf("bookings: insert event: %w", err)
	}

	b.Status = to
	b.UpdatedAt = time.Now()
	return b, e, nil
}

// nullableUUID maps an empty guide id to NULL (marketplace bookings are
// created guideless; the FK would reject "").
func nullableUUID(id string) any {
	if id == "" {
		return nil
	}
	return id
}

func isUniqueViolation(err error, constraintHint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" &&
		strings.Contains(pgErr.ConstraintName, constraintHint)
}

// isExclusionViolation matches the bookings_no_guide_overlap exclusion
// constraint (SQLSTATE 23P01) — the database backstop for the
// double-booking guard.
func isExclusionViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23P01"
}
