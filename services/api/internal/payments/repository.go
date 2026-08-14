package payments

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Payment statuses (spec §8.3). Stored uppercase; the transitions map below
// is the only legal source of moves.
const (
	StatusCreated           = "CREATED"
	StatusPending           = "PENDING"
	StatusSucceeded         = "SUCCEEDED"
	StatusFailed            = "FAILED"
	StatusExpired           = "EXPIRED"
	StatusRefundPending     = "REFUND_PENDING"
	StatusPartiallyRefunded = "PARTIALLY_REFUNDED"
	StatusRefunded          = "REFUNDED"
)

// paymentTransitions is the legal edge set for spec §8.3:
// CREATED -> PENDING -> SUCCEEDED | FAILED | EXPIRED; a succeeded payment can
// enter REFUND_PENDING and land PARTIALLY_REFUNDED | REFUNDED. FAILED/EXPIRED
// and REFUNDED are terminal.
var paymentTransitions = map[string][]string{
	StatusCreated:           {StatusPending, StatusFailed, StatusExpired},
	StatusPending:           {StatusSucceeded, StatusFailed, StatusExpired},
	StatusFailed:            {},
	StatusExpired:           {},
	StatusSucceeded:         {StatusRefundPending},
	StatusRefundPending:     {StatusPartiallyRefunded, StatusRefunded},
	StatusPartiallyRefunded: {StatusRefundPending, StatusRefunded},
	StatusRefunded:          {},
}

// canTransition reports whether from -> to is a legal §8.3 edge.
func canTransition(from, to string) bool {
	for _, t := range paymentTransitions[from] {
		if t == to {
			return true
		}
	}
	return false
}

// CanTransition reports whether from -> to is a legal §8.3 edge. Exported so
// other domains (and tests) can validate payment moves without duplicating
// the state machine.
func CanTransition(from, to string) bool { return canTransition(from, to) }

// Sentinel errors mapped to HTTP statuses by the handler.
var (
	// ErrNotFound — no payment with the given id/reference.
	ErrNotFound = errors.New("payments: not found")
	// ErrIllegalTransition — from -> to is not a legal §8.3 edge.
	ErrIllegalTransition = errors.New("payments: illegal transition")
	// ErrDuplicateReference — provider_reference is already recorded; the
	// UNIQUE constraint is the duplicate-posting backstop (spec §9.2).
	ErrDuplicateReference = errors.New("payments: provider reference already exists")
)

// Payment is a payments row.
type Payment struct {
	ID                string     `json:"id"`
	BookingID         string     `json:"booking_id"`
	Provider          string     `json:"provider"`
	ProviderReference string     `json:"provider_reference"`
	Amount            string     `json:"amount"`
	Currency          string     `json:"currency"`
	Status            string     `json:"status"`
	AuthorizationURL  *string    `json:"authorization_url,omitempty"`
	PaidAt            *time.Time `json:"paid_at"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

const paymentColumns = `id, booking_id, provider, provider_reference, amount::text,
	currency, status, authorization_url, paid_at, created_at, updated_at`

// Repository owns payment persistence (explicit SQL, spec §7.2).
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository builds the repository.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func scanPayment(row pgx.Row) (Payment, error) {
	var p Payment
	err := row.Scan(&p.ID, &p.BookingID, &p.Provider, &p.ProviderReference,
		&p.Amount, &p.Currency, &p.Status, &p.AuthorizationURL, &p.PaidAt,
		&p.CreatedAt, &p.UpdatedAt)
	return p, err
}

// Insert writes a new payment in CREATED inside the caller's transaction.
// A duplicate provider_reference surfaces as ErrDuplicateReference.
func (r *Repository) Insert(ctx context.Context, tx pgx.Tx, p Payment) (Payment, error) {
	out, err := scanPayment(tx.QueryRow(ctx, `
		INSERT INTO payments (booking_id, provider, provider_reference, amount, currency,
		                      status, authorization_url)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+paymentColumns,
		p.BookingID, p.Provider, p.ProviderReference, p.Amount, p.Currency,
		StatusCreated, p.AuthorizationURL))
	if isProviderReferenceViolation(err) {
		return Payment{}, ErrDuplicateReference
	}
	if err != nil {
		return Payment{}, fmt.Errorf("payments: insert: %w", err)
	}
	return out, nil
}

// GetByID returns a payment by id.
func (r *Repository) GetByID(ctx context.Context, id string) (Payment, error) {
	p, err := scanPayment(r.pool.QueryRow(ctx,
		`SELECT `+paymentColumns+` FROM payments WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Payment{}, ErrNotFound
	}
	if err != nil {
		return Payment{}, fmt.Errorf("payments: get by id: %w", err)
	}
	return p, nil
}

// GetByProviderReferenceForUpdate locks the payment for the given provider
// reference inside tx (webhook processing).
func (r *Repository) GetByProviderReferenceForUpdate(ctx context.Context, tx pgx.Tx, ref string) (Payment, error) {
	p, err := scanPayment(tx.QueryRow(ctx,
		`SELECT `+paymentColumns+` FROM payments WHERE provider_reference = $1 FOR UPDATE`, ref))
	if errors.Is(err, pgx.ErrNoRows) {
		return Payment{}, ErrNotFound
	}
	if err != nil {
		return Payment{}, fmt.Errorf("payments: lock by provider reference: %w", err)
	}
	return p, nil
}

// GetByIDForUpdate locks a payment by id inside tx (refund processing).
func (r *Repository) GetByIDForUpdate(ctx context.Context, tx pgx.Tx, id string) (Payment, error) {
	p, err := scanPayment(tx.QueryRow(ctx,
		`SELECT `+paymentColumns+` FROM payments WHERE id = $1 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Payment{}, ErrNotFound
	}
	if err != nil {
		return Payment{}, fmt.Errorf("payments: lock by id: %w", err)
	}
	return p, nil
}

// ActiveForBooking returns the booking's non-terminal payment, if any
// (CREATED/PENDING) — used to refuse duplicate initiations.
func (r *Repository) ActiveForBooking(ctx context.Context, tx pgx.Tx, bookingID string) (Payment, error) {
	p, err := scanPayment(tx.QueryRow(ctx, `
		SELECT `+paymentColumns+` FROM payments
		WHERE booking_id = $1 AND status IN ($2, $3)
		ORDER BY created_at DESC LIMIT 1 FOR UPDATE`,
		bookingID, StatusCreated, StatusPending))
	if errors.Is(err, pgx.ErrNoRows) {
		return Payment{}, ErrNotFound
	}
	if err != nil {
		return Payment{}, fmt.Errorf("payments: active for booking: %w", err)
	}
	return p, nil
}

// Transition applies one validated §8.3 move inside the caller's
// transaction. paidAt stamps paid_at (only legal on -> SUCCEEDED).
func (r *Repository) Transition(ctx context.Context, tx pgx.Tx, p Payment, to string) (Payment, error) {
	if !canTransition(p.Status, to) {
		return Payment{}, fmt.Errorf("%w: %s -> %s", ErrIllegalTransition, p.Status, to)
	}
	out, err := scanPayment(tx.QueryRow(ctx, `
		UPDATE payments SET status = $2,
			paid_at = CASE WHEN $2 = '`+StatusSucceeded+`' THEN now() ELSE paid_at END,
			updated_at = now()
		WHERE id = $1
		RETURNING `+paymentColumns, p.ID, to))
	if err != nil {
		return Payment{}, fmt.Errorf("payments: transition %s -> %s: %w", p.Status, to, err)
	}
	return out, nil
}

// UserEmail returns a user's email for provider initialization.
func (r *Repository) UserEmail(ctx context.Context, userID string) (string, error) {
	var email string
	err := r.pool.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, userID).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("payments: user email: %w", err)
	}
	return email, nil
}

// InsertRefund writes a refunds row inside the caller's transaction.
func (r *Repository) InsertRefund(ctx context.Context, tx pgx.Tx, paymentID, providerRef, amount, reason string) (string, error) {
	var id string
	var ref any
	if providerRef != "" {
		ref = providerRef
	}
	var why any
	if reason != "" {
		why = reason
	}
	err := tx.QueryRow(ctx, `
		INSERT INTO refunds (payment_id, provider_reference, amount, status, reason)
		VALUES ($1, $2, $3, 'SUCCEEDED', $4)
		RETURNING id`, paymentID, ref, amount, why).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("payments: insert refund: %w", err)
	}
	return id, nil
}

func isProviderReferenceViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" &&
		strings.Contains(pgErr.ConstraintName, "provider_reference")
}
