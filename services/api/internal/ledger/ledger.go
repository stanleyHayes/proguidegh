// Package ledger implements the immutable double-entry ledger (spec §9).
//
// Every posting is one ledger_transactions row plus balanced ledger_entries
// rows, written atomically inside the caller's transaction so financial side
// effects commit or roll back with the rest of the business operation.
// Amounts are integer minor units (pesewas) in Go and NUMERIC(14,2) at the
// database; floats are forbidden (spec §1.2). Posted rows are immutable by
// policy: this package exposes no update or delete path, and corrections are
// reversal transactions (spec §9.2).
package ledger

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Entry directions, matching the ledger_entries CHECK constraint.
const (
	Debit  = "debit"
	Credit = "credit"
)

// Sentinel errors for validation failures (mapped to 422/500 by callers).
var (
	// ErrEmpty — a transaction must carry at least one debit and one credit.
	ErrEmpty = errors.New("ledger: transaction has no entries")
	// ErrBadDirection — direction must be "debit" or "credit".
	ErrBadDirection = errors.New("ledger: invalid entry direction")
	// ErrBadAmount — every entry amount must be strictly positive.
	ErrBadAmount = errors.New("ledger: entry amount must be positive")
	// ErrUnbalanced — sum of debits must equal sum of credits (§9.2).
	ErrUnbalanced = errors.New("ledger: debits do not equal credits")
	// ErrCurrencyMismatch — all accounts in one transaction must share a
	// currency; cross-currency postings are out of scope for V1.
	ErrCurrencyMismatch = errors.New("ledger: accounts have mixed currencies")
	// ErrNoReference — the caller-supplied unique reference is required; the
	// database UNIQUE constraint is the duplicate-posting backstop (§9.2).
	ErrNoReference = errors.New("ledger: reference is required")
	// ErrNotFound — unknown transaction or account.
	ErrNotFound = errors.New("ledger: not found")
)

// Entry is one leg of a balanced transaction. AmountMinor is integer minor
// units (pesewas) and must be > 0.
type Entry struct {
	AccountID   string `json:"account_id"`
	Direction   string `json:"direction"`
	AmountMinor int64  `json:"amount_minor"`
}

// Transaction is one balanced double-entry posting.
type Transaction struct {
	ID         string    `json:"id"`
	Reference  string    `json:"reference"`
	Type       string    `json:"type"`
	BookingID  string    `json:"booking_id,omitempty"`
	Entries    []Entry   `json:"entries"`
	OccurredAt time.Time `json:"occurred_at"`
}

// Validate enforces the §9.2 invariants expressible without the database:
// entries present, directions known, amounts positive, debits == credits.
// Pure integer math — exact by construction.
func Validate(t Transaction) error {
	if t.Reference == "" {
		return ErrNoReference
	}
	if len(t.Entries) == 0 {
		return ErrEmpty
	}
	var debits, credits int64
	for _, e := range t.Entries {
		switch e.Direction {
		case Debit:
			debits += e.AmountMinor
		case Credit:
			credits += e.AmountMinor
		default:
			return fmt.Errorf("%w: %q", ErrBadDirection, e.Direction)
		}
		if e.AmountMinor <= 0 {
			return fmt.Errorf("%w: got %d", ErrBadAmount, e.AmountMinor)
		}
		if e.AccountID == "" {
			return fmt.Errorf("%w: entry without account", ErrEmpty)
		}
	}
	if debits == 0 || debits != credits {
		return fmt.Errorf("%w: debits %d, credits %d", ErrUnbalanced, debits, credits)
	}
	return nil
}

// Querier is satisfied by both *pgxpool.Pool and pgx.Tx, so lookups work
// inside or outside a caller-managed transaction.
type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// Service owns ledger persistence (explicit SQL, spec §7.2). All writes take
// the caller's pgx.Tx: the ledger never commits on its own.
type Service struct {
	pool *pgxpool.Pool
}

// NewService builds the service.
func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// AccountID resolves an account by owner and code. ownerID nil matches
// platform-owned accounts (owner_id IS NULL).
func (s *Service) AccountID(ctx context.Context, q Querier, ownerType string, ownerID *string, code string) (string, error) {
	var id string
	var err error
	if ownerID == nil {
		err = q.QueryRow(ctx, `
			SELECT id FROM ledger_accounts
			WHERE owner_type = $1 AND owner_id IS NULL AND code = $2`, ownerType, code).Scan(&id)
	} else {
		err = q.QueryRow(ctx, `
			SELECT id FROM ledger_accounts
			WHERE owner_type = $1 AND owner_id = $2 AND code = $3`, ownerType, *ownerID, code).Scan(&id)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("ledger: account %s/%s: %w", ownerType, code, ErrNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("ledger: resolve account %s/%s: %w", ownerType, code, err)
	}
	return id, nil
}

// AccountCurrency returns the currency of an account (for response shaping).
func (s *Service) AccountCurrency(ctx context.Context, q Querier, accountID string) (string, error) {
	var c string
	err := q.QueryRow(ctx, `SELECT currency FROM ledger_accounts WHERE id = $1`, accountID).Scan(&c)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("ledger: account currency: %w", err)
	}
	return c, nil
}

// Post validates the transaction and inserts the header plus entries
// atomically inside tx. The caller supplies a unique reference; a duplicate
// reference fails on the ledger_transactions_reference_key constraint, which
// is the database backstop against duplicate postings (spec §9.2).
func (s *Service) Post(ctx context.Context, tx pgx.Tx, t Transaction) (Transaction, error) {
	if err := Validate(t); err != nil {
		return Transaction{}, err
	}

	// All legs must share one currency (V1 is GHS-only; the check is cheap
	// and makes mixed-currency drift impossible to write).
	ids := make([]string, 0, len(t.Entries))
	for _, e := range t.Entries {
		ids = append(ids, e.AccountID)
	}
	var currencies int
	if err := tx.QueryRow(ctx, `
		SELECT count(DISTINCT currency) FROM ledger_accounts WHERE id = ANY($1::uuid[])`,
		ids).Scan(&currencies); err != nil {
		return Transaction{}, fmt.Errorf("ledger: currency check: %w", err)
	}
	if currencies != 1 {
		return Transaction{}, fmt.Errorf("%w: %d distinct currencies (or unknown account)", ErrCurrencyMismatch, currencies)
	}

	var booking any
	if t.BookingID != "" {
		booking = t.BookingID
	}
	err := tx.QueryRow(ctx, `
		INSERT INTO ledger_transactions (reference, type, booking_id)
		VALUES ($1, $2, $3)
		RETURNING id, occurred_at`, t.Reference, t.Type, booking).
		Scan(&t.ID, &t.OccurredAt)
	if err != nil {
		return Transaction{}, fmt.Errorf("ledger: insert transaction: %w", err)
	}

	for _, e := range t.Entries {
		// Amounts arrive as integer minor units; the database stores
		// NUMERIC(14,2) major units (GHS).
		if _, err := tx.Exec(ctx, `
			INSERT INTO ledger_entries (transaction_id, account_id, direction, amount)
			VALUES ($1, $2, $3, $4::numeric / 100)`, t.ID, e.AccountID, e.Direction, e.AmountMinor); err != nil {
			return Transaction{}, fmt.Errorf("ledger: insert entry: %w", err)
		}
	}
	return t, nil
}

// Get loads a transaction with its entries.
func (s *Service) Get(ctx context.Context, q Querier, id string) (Transaction, error) {
	var t Transaction
	var booking *string
	err := q.QueryRow(ctx, `
		SELECT id, reference, type, booking_id, occurred_at
		FROM ledger_transactions WHERE id = $1`, id).
		Scan(&t.ID, &t.Reference, &t.Type, &booking, &t.OccurredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Transaction{}, ErrNotFound
	}
	if err != nil {
		return Transaction{}, fmt.Errorf("ledger: get transaction: %w", err)
	}
	if booking != nil {
		t.BookingID = *booking
	}
	t.Entries, err = s.entries(ctx, q, t.ID)
	if err != nil {
		return Transaction{}, err
	}
	return t, nil
}

func (s *Service) entries(ctx context.Context, q Querier, txnID string) ([]Entry, error) {
	rows, err := q.Query(ctx, `
		SELECT account_id, direction, ROUND(amount * 100)::bigint
		FROM ledger_entries WHERE transaction_id = $1
		ORDER BY id`, txnID)
	if err != nil {
		return nil, fmt.Errorf("ledger: list entries: %w", err)
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.AccountID, &e.Direction, &e.AmountMinor); err != nil {
			return nil, fmt.Errorf("ledger: scan entry: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Balance derives an account's balance from its entries (credit-positive,
// integer minor units). Balances are never stored — they are always derived
// sums over the immutable entries (spec §9, ADR 0008).
func (s *Service) Balance(ctx context.Context, accountID string) (int64, error) {
	var bal int64
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(ROUND(SUM(CASE WHEN direction = 'credit' THEN amount ELSE -amount END) * 100), 0)::bigint
		FROM ledger_entries WHERE account_id = $1`, accountID).Scan(&bal)
	if err != nil {
		return 0, fmt.Errorf("ledger: balance: %w", err)
	}
	return bal, nil
}

// Reversal posts the compensating transaction for origTxnID (spec §9.2,
// §4.5): every original entry is reposted with the direction flipped, so the
// net effect on every account is zero while the originals remain intact.
// The caller supplies a fresh unique reference and the reason (recorded via
// the transaction type suffix). Runs inside the caller's transaction.
func (s *Service) Reversal(ctx context.Context, tx pgx.Tx, origTxnID, reference, reason string) (Transaction, error) {
	orig, err := s.Get(ctx, tx, origTxnID)
	if err != nil {
		return Transaction{}, fmt.Errorf("ledger: reversal of %s: %w", origTxnID, err)
	}
	rev := Transaction{
		Reference: reference,
		Type:      orig.Type + "_REVERSAL",
		BookingID: orig.BookingID,
		Entries:   reversedEntries(orig.Entries),
	}
	posted, err := s.Post(ctx, tx, rev)
	if err != nil {
		return Transaction{}, fmt.Errorf("ledger: post reversal (%s): %w", reason, err)
	}
	return posted, nil
}

// reversedEntries flips every direction, preserving amounts and accounts.
// Pure: unit-tested without a database.
func reversedEntries(entries []Entry) []Entry {
	out := make([]Entry, len(entries))
	for i, e := range entries {
		switch e.Direction {
		case Debit:
			e.Direction = Credit
		case Credit:
			e.Direction = Debit
		}
		out[i] = e
	}
	return out
}
