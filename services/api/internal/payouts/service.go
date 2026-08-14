package payouts

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"proguidegh/api/internal/ledger"
	"proguidegh/api/internal/platform/audit"
)

// allowedTransitions is the payout state machine (spec §8.4). PAID is
// terminal; FAILED can be retried or escalated to manual review.
var allowedTransitions = map[string][]string{
	"PENDING_ELIGIBILITY": {"ELIGIBLE", StatusQueued, StatusManualReview},
	"ELIGIBLE":            {StatusQueued, StatusManualReview},
	StatusQueued:          {StatusProcessing, StatusManualReview},
	StatusProcessing:      {StatusPaid, StatusFailed, StatusManualReview},
	StatusFailed:          {StatusRetryQueued, StatusManualReview},
	StatusRetryQueued:     {StatusProcessing, StatusManualReview},
	StatusManualReview:    {StatusRetryQueued},
	StatusPaid:            {},
}

const defaultPayoutDelayDays = 7

// Service is the payouts application service.
type Service struct {
	repo   *Repository
	pool   *pgxpool.Pool
	ledger *ledger.Service
	audit  *audit.Recorder
	key    []byte
}

// NewService builds the service. ledger/audit may be nil in tests. key is
// the payout-account encryption key (see tokenize.go).
func NewService(repo *Repository, pool *pgxpool.Pool, ledgerSvc *ledger.Service, auditor *audit.Recorder, key []byte) *Service {
	return &Service{repo: repo, pool: pool, ledger: ledgerSvc, audit: auditor, key: key}
}

// Key derives the AES key from explicit material with a secret fallback.
func Key(explicit, fallbackSecret string) []byte { return deriveKey(explicit, fallbackSecret) }

func (s *Service) record(ctx context.Context, actorID, action, entityID string, before, after any) {
	if s.audit != nil {
		_ = s.audit.Record(ctx, audit.Entry{
			ActorID:    actorID,
			Action:     action,
			EntityType: "payout",
			EntityID:   entityID,
			Before:     before,
			After:      after,
		})
	}
}

// payoutDelayDays reads the configured hold, defaulting to 7 (spec §8.4).
func (s *Service) payoutDelayDays(ctx context.Context) int {
	var v string
	err := s.pool.QueryRow(ctx,
		`SELECT value_json #>> '{}' FROM system_settings WHERE key = 'payout_delay_days'`).Scan(&v)
	if err != nil {
		return defaultPayoutDelayDays
	}
	var days int
	if _, err := fmt.Sscanf(v, "%d", &days); err != nil || days < 0 {
		return defaultPayoutDelayDays
	}
	return days
}

// Wallet returns the guide's balance summary (P7-01).
func (s *Service) Wallet(ctx context.Context, guideID string) (Wallet, error) {
	return s.repo.Wallet(ctx, guideID, s.payoutDelayDays(ctx))
}

// Statement returns one page of the guide's wallet statement (P7-01).
func (s *Service) Statement(ctx context.Context, guideID string, before *time.Time, beforeID string, limit int) ([]StatementEntry, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.repo.Statement(ctx, guideID, before, beforeID, limit)
}

// PayoutAccount returns the guide's current destination, masked.
func (s *Service) PayoutAccount(ctx context.Context, guideID string) (Account, error) {
	row, err := s.repo.CurrentAccount(ctx, guideID)
	if err != nil {
		return Account{}, err
	}
	plain, err := decryptRef(s.key, row.TokenizedRef)
	if err != nil {
		return Account{}, err
	}
	row.MaskedRef = maskRef(plain)
	return row.Account, nil
}

// PutPayoutAccount registers a new tokenized destination (latest wins).
// The audit trail records only the masked form (P7-02).
func (s *Service) PutPayoutAccount(ctx context.Context, guideID, provider, network, accountRef, actorID string) (Account, error) {
	provider = strings.TrimSpace(provider)
	accountRef = strings.TrimSpace(accountRef)
	network = strings.TrimSpace(network)
	if provider == "" || accountRef == "" {
		return Account{}, fmt.Errorf("%w: provider and account_ref are required", ErrValidation)
	}
	if len(accountRef) > 64 || len(provider) > 64 || len(network) > 64 {
		return Account{}, fmt.Errorf("%w: field too long", ErrValidation)
	}
	tokenized, err := encryptRef(s.key, accountRef)
	if err != nil {
		return Account{}, err
	}
	var networkArg *string
	if network != "" {
		networkArg = &network
	}
	row, err := s.repo.InsertAccount(ctx, guideID, provider, networkArg, tokenized)
	if err != nil {
		return Account{}, err
	}
	s.record(ctx, actorID, "payout_account.registered", row.ID, nil, map[string]any{
		"guide_id": guideID, "provider": provider, "masked_ref": maskRef(accountRef),
	})
	row.MaskedRef = maskRef(accountRef)
	return row.Account, nil
}

// VerifyAccount marks a payout account verified (finance officer, P7-02).
func (s *Service) VerifyAccount(ctx context.Context, id, actorID string) (Account, error) {
	a, err := s.repo.VerifyAccount(ctx, id)
	if err != nil {
		return Account{}, err
	}
	s.record(ctx, actorID, "payout_account.verified", a.ID, nil, map[string]any{"guide_id": a.GuideID})
	return a, nil
}

// ListPayouts returns the admin payout list.
func (s *Service) ListPayouts(ctx context.Context, status, scheduledFor string, limit, offset int) ([]Payout, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.ListPayouts(ctx, status, scheduledFor, limit, offset)
}

// RunBatch queues this cycle's payouts: one QUEUED payout per guide whose
// eligible balance has cleared the payout-delay hold, net of in-flight
// payouts (P7-03). Re-running for the same scheduled_for is a no-op per
// guide thanks to idx_payouts_guide_schedule (P7-07).
func (s *Service) RunBatch(ctx context.Context, scheduledFor, actorID string) (created, eligible int, err error) {
	if _, err := time.Parse("2006-01-02", scheduledFor); err != nil {
		return 0, 0, fmt.Errorf("%w: scheduled_for must be YYYY-MM-DD", ErrValidation)
	}
	items, err := s.repo.EligiblePayoutAmounts(ctx, s.payoutDelayDays(ctx))
	if err != nil {
		return 0, 0, err
	}
	created, err = s.repo.InsertBatch(ctx, scheduledFor, items)
	if err != nil {
		return 0, 0, err
	}
	s.record(ctx, actorID, "payouts.batch", scheduledFor, nil, map[string]any{
		"eligible_guides": len(items), "created": created,
	})
	return created, len(items), nil
}

// BatchDueToday reports whether a payout batch already exists for the date
// — the scheduler's run-once guard (P7-03).
func (s *Service) BatchDueToday(ctx context.Context, today string) (bool, error) {
	var n int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*)::int FROM payouts WHERE scheduled_for = $1`, today).Scan(&n); err != nil {
		return false, fmt.Errorf("payouts: batch check: %w", err)
	}
	return n == 0, nil
}

// Transition moves a payout through the §8.4 machine. PAID additionally
// posts the balanced ledger move — debit guide_payable_eligible, credit
// tourist_clearing — in the same database transaction, and links it via
// payouts.ledger_transaction_id (P7-05).
func (s *Service) Transition(ctx context.Context, id, to, failureReason, providerRef, actorID string) (Payout, error) {
	p, err := s.repo.GetPayout(ctx, id)
	if err != nil {
		return Payout{}, err
	}
	allowed := false
	for _, candidate := range allowedTransitions[p.Status] {
		if candidate == to {
			allowed = true
			break
		}
	}
	if !allowed {
		return Payout{}, fmt.Errorf("%w: %s -> %s", ErrIllegalTransition, p.Status, to)
	}
	if to == StatusFailed && strings.TrimSpace(failureReason) == "" {
		return Payout{}, fmt.Errorf("%w: failure_reason is required when failing a payout", ErrValidation)
	}

	if to == StatusPaid {
		return s.markPaid(ctx, p, providerRef, actorID)
	}

	var reasonArg, refArg *string
	if strings.TrimSpace(failureReason) != "" {
		reasonArg = &failureReason
	}
	if strings.TrimSpace(providerRef) != "" {
		refArg = &providerRef
	}
	updated, err := s.repo.SetStatus(ctx, id, p.Status, to, reasonArg, refArg)
	if errors.Is(err, ErrNotFound) {
		return Payout{}, fmt.Errorf("%w: payout moved concurrently", ErrIllegalTransition)
	}
	if err != nil {
		return Payout{}, err
	}
	s.record(ctx, actorID, "payout.transition", id,
		map[string]any{"status": p.Status},
		map[string]any{"status": to, "failure_reason": reasonArg})
	return updated, nil
}

// markPaid posts the ledger move and flips the status atomically.
func (s *Service) markPaid(ctx context.Context, p Payout, providerRef, actorID string) (Payout, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Payout{}, fmt.Errorf("payouts: begin paid: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	eligibleID, err := s.ledger.AccountID(ctx, tx, "platform", nil, "guide_payable_eligible")
	if err != nil {
		return Payout{}, err
	}
	clearingID, err := s.ledger.AccountID(ctx, tx, "platform", nil, "tourist_clearing")
	if err != nil {
		return Payout{}, err
	}
	posted, err := s.ledger.Post(ctx, tx, ledger.Transaction{
		Reference: "PAYOUT:" + p.ID,
		Type:      "payout",
		Entries: []ledger.Entry{
			{AccountID: eligibleID, Direction: ledger.Debit, AmountMinor: p.AmountMinor},
			{AccountID: clearingID, Direction: ledger.Credit, AmountMinor: p.AmountMinor},
		},
	})
	if err != nil {
		return Payout{}, fmt.Errorf("payouts: post ledger: %w", err)
	}
	if err := MarkPaid(ctx, tx, p.ID, posted.ID); err != nil {
		return Payout{}, err
	}
	if providerRef = strings.TrimSpace(providerRef); providerRef != "" {
		if _, err := tx.Exec(ctx,
			`UPDATE payouts SET provider_reference = $2 WHERE id = $1`, p.ID, providerRef); err != nil {
			return Payout{}, fmt.Errorf("payouts: provider reference: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Payout{}, fmt.Errorf("payouts: commit paid: %w", err)
	}

	s.record(ctx, actorID, "payout.paid", p.ID,
		map[string]any{"status": p.Status},
		map[string]any{"status": StatusPaid, "ledger_transaction_id": posted.ID, "amount_minor": p.AmountMinor})
	return s.repo.GetPayout(ctx, p.ID)
}

// ExportCSV renders the finance transfer file for one scheduled date:
// QUEUED/RETRY_QUEUED payouts with decrypted destination references. This
// is the only surface where plaintext account refs leave the package
// (P7-04); the export is audited.
func (s *Service) ExportCSV(ctx context.Context, scheduledFor, actorID string) (string, error) {
	if _, err := time.Parse("2006-01-02", scheduledFor); err != nil {
		return "", fmt.Errorf("%w: scheduled_for must be YYYY-MM-DD", ErrValidation)
	}
	rows, err := s.repo.ExportRows(ctx, scheduledFor)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("payout_id,guide_name,provider,network,account_ref,amount,currency,scheduled_for\n")
	for _, row := range rows {
		ref, err := decryptRef(s.key, row.TokenizedRef)
		if err != nil {
			return "", err
		}
		name := ""
		if row.GuideName != nil {
			name = *row.GuideName
		}
		network := ""
		if row.Network != nil {
			network = *row.Network
		}
		fmt.Fprintf(&b, "%s,%s,%s,%s,%s,%.2f,%s,%s\n",
			row.PayoutID, csvField(name), row.Provider, network, ref,
			float64(row.AmountMinor)/100, row.Currency, row.ScheduledFor)
	}
	s.record(ctx, actorID, "payouts.export", scheduledFor, nil, map[string]any{"rows": len(rows)})
	return b.String(), nil
}

func csvField(s string) string {
	if strings.ContainsAny(s, ",\"\n") {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}

// LevyReport returns the tourism-levy payable balance and period movement
// (P7-06).
func (s *Service) LevyReport(ctx context.Context, from, to *time.Time) (balance, credits, debits int64, err error) {
	return s.repo.LevyReport(ctx, from, to)
}
