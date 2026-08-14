// Package payouts implements guide wallets, the weekly payout batch, the
// payout state machine (spec §8.4), finance CSV export, the tourism-levy
// report and payout-account tokenization (P7-01…P7-07).
//
// Money is always handled in minor units (pesewas) inside Go; the database
// stores NUMERIC major units, so every crossing converts with
// ROUND(x * 100)::bigint / ($n::numeric / 100) — the same convention the
// ledger package uses.
package payouts

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Payout statuses (spec §8.4). PENDING_ELIGIBILITY/ELIGIBLE exist in the
// schema's CHECK list; the batch creates rows directly in QUEUED.
const (
	StatusQueued       = "QUEUED"
	StatusProcessing   = "PROCESSING"
	StatusPaid         = "PAID"
	StatusFailed       = "FAILED"
	StatusRetryQueued  = "RETRY_QUEUED"
	StatusManualReview = "MANUAL_REVIEW"
)

// inFlightStatuses holds a guide's eligible balance hostage: a payout in
// any of these states has not concluded, so the batch subtracts them from
// the payout-eligible amount (P7-02).
var inFlightStatuses = []string{
	"PENDING_ELIGIBILITY", "ELIGIBLE", StatusQueued, StatusProcessing,
	StatusRetryQueued, StatusManualReview,
}

// Account is a guide's tokenized payout destination. MaskedRef is all that
// ever leaves the package outside the finance export.
type Account struct {
	ID         string     `json:"id"`
	GuideID    string     `json:"guide_id"`
	Provider   string     `json:"provider"`
	Network    *string    `json:"network"`
	MaskedRef  string     `json:"masked_ref"`
	VerifiedAt *time.Time `json:"verified_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// accountRow is the stored form; TokenizedRef stays inside the package.
type accountRow struct {
	Account
	TokenizedRef string
}

// Payout is one payouts row.
type Payout struct {
	ID                  string    `json:"id"`
	GuideID             string    `json:"guide_id"`
	GuideName           *string   `json:"guide_name"`
	AmountMinor         int64     `json:"amount_minor"`
	Currency            string    `json:"currency"`
	Status              string    `json:"status"`
	ProviderReference   *string   `json:"provider_reference"`
	ScheduledFor        *string   `json:"scheduled_for"`
	FailureReason       *string   `json:"failure_reason"`
	LedgerTransactionID *string   `json:"ledger_transaction_id"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// Wallet is the guide-facing balance summary (P7-01).
type Wallet struct {
	PendingMinor        int64 `json:"pending_minor"`
	EligibleMinor       int64 `json:"eligible_minor"`
	PayoutEligibleMinor int64 `json:"payout_eligible_minor"`
	InFlightMinor       int64 `json:"in_flight_minor"`
	PaidTotalMinor      int64 `json:"paid_total_minor"`
	PayoutDelayDays     int   `json:"payout_delay_days"`
}

// StatementEntry is one wallet-statement line — either a ledger movement on
// the guide's eligible payable balance or a payout (P7-01).
type StatementEntry struct {
	At          time.Time `json:"at"`
	ID          string    `json:"id"`
	Kind        string    `json:"kind"` // ledger | payout
	Reference   string    `json:"reference"`
	Detail      string    `json:"detail"` // ledger type or payout status
	AmountMinor int64     `json:"amount_minor"`
}

// guideAmount pairs a guide with a computed minor-unit amount.
type guideAmount struct {
	GuideID     string
	AmountMinor int64
}

// exportRow is one CSV line before decryption.
type exportRow struct {
	PayoutID     string
	GuideName    *string
	Provider     string
	Network      *string
	TokenizedRef string
	AmountMinor  int64
	Currency     string
	ScheduledFor string
}

// Sentinel errors mapped by the handler.
var (
	// ErrNotFound — no such payout / payout account.
	ErrNotFound = errors.New("payouts: not found")
	// ErrIllegalTransition — status move the §8.4 machine does not allow.
	ErrIllegalTransition = errors.New("payouts: illegal status transition")
	// ErrValidation — malformed input.
	ErrValidation = errors.New("payouts: validation failed")
)

// Repository is the payouts data layer.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository builds the repository.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

const payoutCols = `p.id, p.guide_id, gp.public_name, ROUND(p.amount * 100)::bigint,
	p.currency, p.status, p.provider_reference, p.scheduled_for::text,
	p.failure_reason, p.ledger_transaction_id, p.created_at, p.updated_at`

const payoutFrom = ` FROM payouts p JOIN guide_profiles gp ON gp.user_id = p.guide_id `

func scanPayout(row interface{ Scan(dest ...any) error }) (Payout, error) {
	var p Payout
	err := row.Scan(&p.ID, &p.GuideID, &p.GuideName, &p.AmountMinor, &p.Currency,
		&p.Status, &p.ProviderReference, &p.ScheduledFor, &p.FailureReason,
		&p.LedgerTransactionID, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

// CurrentAccount returns the guide's latest payout account. ErrNotFound
// when the guide has never registered one.
func (r *Repository) CurrentAccount(ctx context.Context, guideID string) (accountRow, error) {
	var a accountRow
	err := r.pool.QueryRow(ctx,
		`SELECT id, guide_id, provider, network, account_ref_tokenized, verified_at, created_at
		 FROM payout_accounts WHERE guide_id = $1
		 ORDER BY created_at DESC LIMIT 1`, guideID).
		Scan(&a.ID, &a.GuideID, &a.Provider, &a.Network, &a.TokenizedRef, &a.VerifiedAt, &a.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return accountRow{}, ErrNotFound
	}
	if err != nil {
		return accountRow{}, fmt.Errorf("payouts: current account: %w", err)
	}
	return a, nil
}

// InsertAccount stores a new tokenized destination (latest wins).
func (r *Repository) InsertAccount(ctx context.Context, guideID, provider string, network *string, tokenized string) (accountRow, error) {
	var a accountRow
	err := r.pool.QueryRow(ctx,
		`INSERT INTO payout_accounts (guide_id, provider, network, account_ref_tokenized)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, guide_id, provider, network, account_ref_tokenized, verified_at, created_at`,
		guideID, provider, network, tokenized).
		Scan(&a.ID, &a.GuideID, &a.Provider, &a.Network, &a.TokenizedRef, &a.VerifiedAt, &a.CreatedAt)
	if err != nil {
		return accountRow{}, fmt.Errorf("payouts: insert account: %w", err)
	}
	return a, nil
}

// VerifyAccount marks a payout account verified (finance officer action).
func (r *Repository) VerifyAccount(ctx context.Context, id string) (Account, error) {
	var a accountRow
	err := r.pool.QueryRow(ctx,
		`UPDATE payout_accounts SET verified_at = now(), updated_at = now()
		 WHERE id = $1
		 RETURNING id, guide_id, provider, network, account_ref_tokenized, verified_at, created_at`, id).
		Scan(&a.ID, &a.GuideID, &a.Provider, &a.Network, &a.TokenizedRef, &a.VerifiedAt, &a.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, fmt.Errorf("payouts: verify account: %w", err)
	}
	return a.Account, nil
}

// payableSum returns the signed minor-unit balance of one platform payable
// account attributed to one guide via ledger_transactions.booking_id. When
// completedBefore is non-nil, only bookings completed on or before it count
// (the payout-delay hold, P7-02).
func (r *Repository) payableSum(ctx context.Context, guideID, accountCode string, completedBefore *time.Time) (int64, error) {
	q := `SELECT COALESCE(ROUND(SUM(CASE WHEN e.direction = 'credit' THEN e.amount ELSE -e.amount END) * 100)::bigint, 0)
		FROM ledger_entries e
		JOIN ledger_accounts a ON a.id = e.account_id
		JOIN ledger_transactions t ON t.id = e.transaction_id
		JOIN bookings b ON b.id = t.booking_id
		WHERE a.owner_type = 'platform' AND a.code = $1 AND b.guide_id = $2`
	args := []any{accountCode, guideID}
	if completedBefore != nil {
		q += ` AND b.ends_at <= $3`
		args = append(args, *completedBefore)
	}
	var sum int64
	if err := r.pool.QueryRow(ctx, q, args...).Scan(&sum); err != nil {
		return 0, fmt.Errorf("payouts: payable sum %s: %w", accountCode, err)
	}
	return sum, nil
}

// payoutSum returns the total minor units of a guide's payouts in the
// given statuses.
func (r *Repository) payoutSum(ctx context.Context, guideID string, statuses []string) (int64, error) {
	var sum int64
	if err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(ROUND(SUM(amount) * 100)::bigint, 0)
		 FROM payouts WHERE guide_id = $1 AND status = ANY ($2)`, guideID, statuses).Scan(&sum); err != nil {
		return 0, fmt.Errorf("payouts: payout sum: %w", err)
	}
	return sum, nil
}

// deductedStatuses are payouts that draw down a guide's eligible balance:
// everything in flight plus everything already PAID. The PAID ledger
// posting is platform-level (booking_id NULL), so per-guide attribution
// subtracts payout rows instead of reading the ledger — a payout spans
// many bookings and cannot be attributed to one.
var deductedStatuses = append(append([]string{}, inFlightStatuses...), StatusPaid)

// Wallet computes the guide's balance summary. Eligible is the ledger
// eligible balance net of payouts (in-flight + paid); payout-eligible is
// the portion whose bookings have cleared the payout-delay hold, likewise
// net of payouts.
func (r *Repository) Wallet(ctx context.Context, guideID string, delayDays int) (Wallet, error) {
	cutoff := time.Now().AddDate(0, 0, -delayDays)
	pending, err := r.payableSum(ctx, guideID, "guide_payable_pending", nil)
	if err != nil {
		return Wallet{}, err
	}
	rawEligible, err := r.payableSum(ctx, guideID, "guide_payable_eligible", nil)
	if err != nil {
		return Wallet{}, err
	}
	eligibleHeld, err := r.payableSum(ctx, guideID, "guide_payable_eligible", &cutoff)
	if err != nil {
		return Wallet{}, err
	}
	inFlight, err := r.payoutSum(ctx, guideID, inFlightStatuses)
	if err != nil {
		return Wallet{}, err
	}
	paid, err := r.payoutSum(ctx, guideID, []string{StatusPaid})
	if err != nil {
		return Wallet{}, err
	}
	eligible := rawEligible - inFlight - paid
	if eligible < 0 {
		eligible = 0
	}
	payoutEligible := eligibleHeld - inFlight - paid
	if payoutEligible < 0 {
		payoutEligible = 0
	}
	return Wallet{
		PendingMinor:        pending,
		EligibleMinor:       eligible,
		PayoutEligibleMinor: payoutEligible,
		InFlightMinor:       inFlight,
		PaidTotalMinor:      paid,
		PayoutDelayDays:     delayDays,
	}, nil
}

// EligiblePayoutAmounts returns, per guide, the payout-eligible amount for
// the batch: eligible balance past the delay hold minus payout drawdowns
// (in-flight + paid), positives only (P7-03).
func (r *Repository) EligiblePayoutAmounts(ctx context.Context, delayDays int) ([]guideAmount, error) {
	rows, err := r.pool.Query(ctx,
		`WITH eligible AS (
			SELECT b.guide_id,
			       ROUND(SUM(CASE WHEN e.direction = 'credit' THEN e.amount ELSE -e.amount END) * 100)::bigint AS minor
			FROM ledger_entries e
			JOIN ledger_accounts a ON a.id = e.account_id
			JOIN ledger_transactions t ON t.id = e.transaction_id
			JOIN bookings b ON b.id = t.booking_id
			WHERE a.owner_type = 'platform' AND a.code = 'guide_payable_eligible'
			  AND b.ends_at <= now() - ($1::int * INTERVAL '1 day')
			GROUP BY b.guide_id
		), deducted AS (
			SELECT guide_id, ROUND(SUM(amount) * 100)::bigint AS minor
			FROM payouts WHERE status = ANY ($2)
			GROUP BY guide_id
		)
		SELECT el.guide_id, el.minor - COALESCE(d.minor, 0)
		FROM eligible el LEFT JOIN deducted d ON d.guide_id = el.guide_id
		WHERE el.minor - COALESCE(d.minor, 0) > 0`,
		delayDays, deductedStatuses)
	if err != nil {
		return nil, fmt.Errorf("payouts: eligible amounts: %w", err)
	}
	defer rows.Close()

	var out []guideAmount
	for rows.Next() {
		var g guideAmount
		if err := rows.Scan(&g.GuideID, &g.AmountMinor); err != nil {
			return nil, fmt.Errorf("payouts: scan eligible: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// InsertBatch queues one payout per guide for the scheduled date,
// atomically. ON CONFLICT DO NOTHING against idx_payouts_guide_schedule
// makes re-runs idempotent (P7-07); it returns the number actually created.
func (r *Repository) InsertBatch(ctx context.Context, scheduledFor string, items []guideAmount) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("payouts: begin batch: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	created := 0
	for _, item := range items {
		tag, err := tx.Exec(ctx,
			`INSERT INTO payouts (guide_id, amount, currency, status, scheduled_for)
			 VALUES ($1, $2::numeric / 100, 'GHS', $3, $4)
			 ON CONFLICT (guide_id, scheduled_for) WHERE status <> 'FAILED' DO NOTHING`,
			item.GuideID, item.AmountMinor, StatusQueued, scheduledFor)
		if err != nil {
			return 0, fmt.Errorf("payouts: insert batch row: %w", err)
		}
		created += int(tag.RowsAffected())
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("payouts: commit batch: %w", err)
	}
	return created, nil
}

// ListPayouts returns payouts, newest first, offset-paginated (low-volume
// admin list, §14).
func (r *Repository) ListPayouts(ctx context.Context, status, scheduledFor string, limit, offset int) ([]Payout, int, error) {
	where := "WHERE TRUE"
	args := []any{}
	add := func(clause string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(" AND %s$%d", clause, len(args))
	}
	if status != "" {
		add("p.status = ", status)
	}
	if scheduledFor != "" {
		add("p.scheduled_for = ", scheduledFor)
	}

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*)::int`+payoutFrom+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("payouts: count: %w", err)
	}

	args = append(args, limit, offset)
	rows, err := r.pool.Query(ctx,
		`SELECT `+payoutCols+payoutFrom+where+
			fmt.Sprintf(" ORDER BY p.created_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("payouts: list: %w", err)
	}
	defer rows.Close()

	var out []Payout
	for rows.Next() {
		p, err := scanPayout(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, p)
	}
	return out, total, rows.Err()
}

// GetPayout loads one payout. ErrNotFound when absent.
func (r *Repository) GetPayout(ctx context.Context, id string) (Payout, error) {
	p, err := scanPayout(r.pool.QueryRow(ctx,
		`SELECT `+payoutCols+payoutFrom+`WHERE p.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Payout{}, ErrNotFound
	}
	if err != nil {
		return Payout{}, fmt.Errorf("payouts: get: %w", err)
	}
	return p, nil
}

// SetStatus moves a payout, guarded on the expected current status so a
// concurrent transition cannot silently win. ErrNotFound means the guard
// failed — the row moved under us.
func (r *Repository) SetStatus(ctx context.Context, id, expected, to string, failureReason, providerRef *string) (Payout, error) {
	p, err := scanPayout(r.pool.QueryRow(ctx,
		`UPDATE payouts SET
		   status             = $3,
		   failure_reason     = COALESCE($4, failure_reason),
		   provider_reference = COALESCE($5, provider_reference),
		   updated_at         = now()
		 WHERE id = $1 AND status = $2
		 RETURNING id, guide_id, NULL, ROUND(amount * 100)::bigint, currency, status,
		           provider_reference, scheduled_for::text, failure_reason,
		           ledger_transaction_id, created_at, updated_at`,
		id, expected, to, failureReason, providerRef))
	if errors.Is(err, pgx.ErrNoRows) {
		return Payout{}, ErrNotFound
	}
	if err != nil {
		return Payout{}, fmt.Errorf("payouts: set status: %w", err)
	}
	return p, nil
}

// MarkPaid moves a payout to PAID and links its ledger posting, inside the
// caller's transaction (the ledger entries and the status change commit or
// roll back together).
func MarkPaid(ctx context.Context, tx pgx.Tx, id, ledgerTxnID string) error {
	tag, err := tx.Exec(ctx,
		`UPDATE payouts SET status = $2, ledger_transaction_id = $3, updated_at = now()
		 WHERE id = $1 AND status = 'PROCESSING'`, id, StatusPaid, ledgerTxnID)
	if err != nil {
		return fmt.Errorf("payouts: mark paid: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ExportRows returns the QUEUED/RETRY_QUEUED payouts for one scheduled
// date with each guide's latest payout account attached (P7-04).
func (r *Repository) ExportRows(ctx context.Context, scheduledFor string) ([]exportRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT p.id, gp.public_name, pa.provider, pa.network, pa.account_ref_tokenized,
		        ROUND(p.amount * 100)::bigint, p.currency, p.scheduled_for::text
		 FROM payouts p
		 JOIN guide_profiles gp ON gp.user_id = p.guide_id
		 JOIN LATERAL (
		   SELECT provider, network, account_ref_tokenized
		   FROM payout_accounts WHERE guide_id = p.guide_id
		   ORDER BY created_at DESC LIMIT 1
		 ) pa ON TRUE
		 WHERE p.scheduled_for = $1 AND p.status IN ($2, $3)
		 ORDER BY gp.public_name NULLS LAST, p.created_at`,
		scheduledFor, StatusQueued, StatusRetryQueued)
	if err != nil {
		return nil, fmt.Errorf("payouts: export rows: %w", err)
	}
	defer rows.Close()

	var out []exportRow
	for rows.Next() {
		var e exportRow
		if err := rows.Scan(&e.PayoutID, &e.GuideName, &e.Provider, &e.Network,
			&e.TokenizedRef, &e.AmountMinor, &e.Currency, &e.ScheduledFor); err != nil {
			return nil, fmt.Errorf("payouts: scan export: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Statement returns the guide's wallet statement — ledger movements on the
// eligible payable balance plus payout rows — newest first, keyset
// paginated on (at, id) (P7-01).
func (r *Repository) Statement(ctx context.Context, guideID string, before *time.Time, beforeID string, limit int) ([]StatementEntry, error) {
	q := `SELECT * FROM (
		SELECT t.occurred_at AS at, t.id::text AS id, 'ledger' AS kind,
		       t.reference, t.type AS detail,
		       ROUND(SUM(CASE WHEN e.direction = 'credit' THEN e.amount ELSE -e.amount END) * 100)::bigint AS amount_minor
		FROM ledger_entries e
		JOIN ledger_accounts a ON a.id = e.account_id
		JOIN ledger_transactions t ON t.id = e.transaction_id
		JOIN bookings b ON b.id = t.booking_id
		WHERE b.guide_id = $1 AND a.owner_type = 'platform' AND a.code = 'guide_payable_eligible'
		GROUP BY t.id
		UNION ALL
		SELECT p.updated_at, p.id::text, 'payout', 'PAYOUT:' || p.id::text, p.status,
		       -ROUND(p.amount * 100)::bigint
		FROM payouts p WHERE p.guide_id = $1
	) s`
	args := []any{guideID}
	if before != nil {
		q += ` WHERE (s.at, s.id) < ($2, $3)`
		args = append(args, *before, beforeID)
	}
	q += fmt.Sprintf(` ORDER BY s.at DESC, s.id DESC LIMIT $%d`, len(args)+1)
	args = append(args, limit)

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("payouts: statement: %w", err)
	}
	defer rows.Close()

	var out []StatementEntry
	for rows.Next() {
		var e StatementEntry
		if err := rows.Scan(&e.At, &e.ID, &e.Kind, &e.Reference, &e.Detail, &e.AmountMinor); err != nil {
			return nil, fmt.Errorf("payouts: scan statement: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// LevyReport returns the tourism-levy payable balance (all time) plus the
// period's credited/debited totals (P7-06).
func (r *Repository) LevyReport(ctx context.Context, from, to *time.Time) (balanceMinor, creditsMinor, debitsMinor int64, err error) {
	base := `FROM ledger_entries e
		JOIN ledger_accounts a ON a.id = e.account_id
		JOIN ledger_transactions t ON t.id = e.transaction_id
		WHERE a.owner_type = 'platform' AND a.code = 'tourism_levy_payable'`
	if err = r.pool.QueryRow(ctx,
		`SELECT COALESCE(ROUND(SUM(CASE WHEN e.direction = 'credit' THEN e.amount ELSE -e.amount END) * 100)::bigint, 0) `+base).
		Scan(&balanceMinor); err != nil {
		return 0, 0, 0, fmt.Errorf("payouts: levy balance: %w", err)
	}

	period := base
	args := []any{}
	if from != nil {
		args = append(args, *from)
		period += fmt.Sprintf(` AND t.occurred_at >= $%d`, len(args))
	}
	if to != nil {
		args = append(args, *to)
		period += fmt.Sprintf(` AND t.occurred_at < $%d`, len(args))
	}
	err = r.pool.QueryRow(ctx,
		`SELECT COALESCE(ROUND(SUM(CASE WHEN e.direction = 'credit' THEN e.amount END) * 100)::bigint, 0),
		        COALESCE(ROUND(SUM(CASE WHEN e.direction = 'debit' THEN e.amount END) * 100)::bigint, 0) `+period,
		args...).Scan(&creditsMinor, &debitsMinor)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("payouts: levy period: %w", err)
	}
	return balanceMinor, creditsMinor, debitsMinor, nil
}
