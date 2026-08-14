package payments

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"proguidegh/api/internal/bookings"
	"proguidegh/api/internal/ledger"
	"proguidegh/api/internal/receipts"
)

// Service is the payments domain service: initiation, webhook confirmation
// and refunds (spec §4.5, §8.3, §13.3).
type Service struct {
	pool     *pgxpool.Pool
	repo     *Repository
	bookings *bookings.Repository
	ledger   *ledger.Service
	receipts *receipts.Service
	provider PaymentProvider
	now      func() time.Time

	// OnConfirmed runs after a payment-success webhook commits (booking now
	// CONFIRMED). Phase 5 wires dispatch here: marketplace bookings (no
	// guide at creation) are offered to eligible guides. It runs outside
	// the webhook transaction; failures must be logged by the callback, not
	// propagated — the payment is already durable and operations can
	// re-dispatch manually (POST /admin/bookings/{id}/dispatch).
	OnConfirmed func(ctx context.Context, bookingID string)
}

// NewService builds the service.
func NewService(pool *pgxpool.Pool, repo *Repository, bookingsRepo *bookings.Repository,
	ledgerSvc *ledger.Service, receiptsSvc *receipts.Service, provider PaymentProvider) *Service {
	return &Service{pool: pool, repo: repo, bookings: bookingsRepo,
		ledger: ledgerSvc, receipts: receiptsSvc, provider: provider, now: time.Now}
}

// Provider exposes the active adapter (the webhook handler verifies
// signatures through it).
func (s *Service) Provider() PaymentProvider { return s.provider }

// Sentinel errors mapped to HTTP statuses by the handler.
var (
	// ErrValidation — malformed request (missing Idempotency-Key etc.).
	ErrValidation = errors.New("payments: invalid request")
	// ErrNotPayable — the booking is not awaiting payment (409).
	ErrNotPayable = errors.New("payments: booking is not awaiting payment")
	// ErrAlreadyActive — a non-terminal payment already exists for the
	// booking under a different Idempotency-Key (409).
	ErrAlreadyActive = errors.New("payments: an active payment already exists for this booking")
	// ErrIdempotencyConflict — same key, different payload (409).
	ErrIdempotencyConflict = errors.New("payments: idempotency key reused with a different payload")
	// ErrUnknownReference — the webhook names a payment we never initialized.
	ErrUnknownReference = errors.New("payments: webhook for unknown provider reference")
	// ErrNotRefundable — the payment is not in a refundable state (409).
	ErrNotRefundable = errors.New("payments: payment is not refundable")
)

// IntentResult is the payment-intent response: the payment row plus the
// provider authorization URL, and whether the call was an idempotent replay.
type IntentResult struct {
	Payment  Payment
	Replayed bool
}

// CreateIntent initializes a provider payment for a PAYMENT_PENDING booking
// owned by touristID (ownership is enforced by the handler). The amount is
// the booking's server-authoritative snapshot — client totals are never
// trusted (spec §14). Idempotency: the Idempotency-Key is claimed inside the
// creation transaction; a replay returns the original payment with the SAME
// provider reference and authorization URL (stored in 0005).
func (s *Service) CreateIntent(ctx context.Context, booking bookings.Booking, touristEmail, idemKey string) (IntentResult, error) {
	if idemKey == "" || len(idemKey) > 200 {
		return IntentResult{}, fmt.Errorf("%w: Idempotency-Key header is required (max 200 chars)", ErrValidation)
	}
	if booking.Status != bookings.StatusPaymentPending {
		return IntentResult{}, fmt.Errorf("%w: booking is %s", ErrNotPayable, booking.Status)
	}
	if booking.Amount == nil {
		return IntentResult{}, fmt.Errorf("payments: booking %s has no server price snapshot", booking.ID)
	}
	amountMinor, err := bookings.ParseDecimal(*booking.Amount)
	if err != nil {
		return IntentResult{}, fmt.Errorf("payments: booking amount %q: %w", *booking.Amount, err)
	}

	// Generate our provider reference up front; both adapters echo it back.
	ref := "ggpay_" + randomHex(12)
	init, err := s.provider.InitializePayment(ctx, InitializePaymentRequest{
		Email:       touristEmail,
		AmountMinor: amountMinor,
		Currency:    booking.Currency,
		Reference:   ref,
		Metadata:    map[string]string{"booking_id": booking.ID, "booking_reference": booking.Reference},
	})
	if err != nil {
		return IntentResult{}, fmt.Errorf("payments: initialize at provider: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return IntentResult{}, fmt.Errorf("payments: begin intent: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after Commit

	// Claim the idempotency key; on conflict replay or reject.
	scope := "payment.intent:" + booking.ID
	payloadHash := hashStrings(booking.ID, *booking.Amount, booking.Currency)
	var claimed string
	err = tx.QueryRow(ctx, `
		INSERT INTO idempotency_keys (key, scope, response_code, response_body_hash, expires_at)
		VALUES ($1, $2, NULL, $3, now() + interval '24 hours')
		ON CONFLICT (key, scope) DO NOTHING
		RETURNING key`, idemKey, scope, payloadHash).Scan(&claimed)
	if errors.Is(err, pgx.ErrNoRows) {
		tx.Rollback(ctx) //nolint:errcheck
		return s.replayIntent(ctx, booking.ID, idemKey, scope, payloadHash)
	}
	if err != nil {
		return IntentResult{}, fmt.Errorf("payments: claim idempotency key: %w", err)
	}

	// Refuse a second active payment for the same booking under a new key.
	if _, err := s.repo.ActiveForBooking(ctx, tx, booking.ID); err == nil {
		return IntentResult{}, ErrAlreadyActive
	} else if !errors.Is(err, ErrNotFound) {
		return IntentResult{}, err
	}

	p, err := s.repo.Insert(ctx, tx, Payment{
		BookingID:         booking.ID,
		Provider:          s.provider.Name(),
		ProviderReference: init.Reference,
		Amount:            *booking.Amount,
		Currency:          booking.Currency,
		AuthorizationURL:  &init.AuthorizationURL,
	})
	if err != nil {
		return IntentResult{}, err
	}
	// CREATED -> PENDING: the provider now awaits the tourist's payment.
	if p, err = s.repo.Transition(ctx, tx, p, StatusPending); err != nil {
		return IntentResult{}, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE idempotency_keys SET entity_id = $3, response_code = 201
		WHERE key = $1 AND scope = $2`, idemKey, scope, p.ID); err != nil {
		return IntentResult{}, fmt.Errorf("payments: complete idempotency key: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return IntentResult{}, fmt.Errorf("payments: commit intent: %w", err)
	}
	return IntentResult{Payment: p}, nil
}

// replayIntent resolves a conflicting idempotency claim for initiation.
func (s *Service) replayIntent(ctx context.Context, bookingID, key, scope, payloadHash string) (IntentResult, error) {
	var entityID *string
	var hash *string
	err := s.pool.QueryRow(ctx, `
		SELECT entity_id, response_body_hash FROM idempotency_keys
		WHERE key = $1 AND scope = $2`, key, scope).Scan(&entityID, &hash)
	if err != nil {
		return IntentResult{}, fmt.Errorf("payments: read idempotency key: %w", err)
	}
	if hash == nil || *hash != payloadHash {
		return IntentResult{}, ErrIdempotencyConflict
	}
	if entityID == nil {
		return IntentResult{}, fmt.Errorf("%w: original request still in progress", ErrIdempotencyConflict)
	}
	p, err := s.repo.GetByID(ctx, *entityID)
	if err != nil {
		return IntentResult{}, fmt.Errorf("payments: replay lookup: %w", err)
	}
	return IntentResult{Payment: p, Replayed: true}, nil
}

// WebhookOutcome classifies the webhook result for the handler.
type WebhookOutcome struct {
	Replay  bool // duplicate delivery: 200 with zero side effects
	Ignored bool // event type carries no payment action (still recorded)
}

// HandleWebhook processes one signed provider callback (spec §4.5, §14):
//
//  1. Signature verification happens in the HANDLER on the exact raw bytes
//     before this is called.
//  2. The dedupe row (webhook_events, UNIQUE provider+event_reference) is
//     inserted in the SAME transaction as every side effect, so a replay is
//     a no-op and a crash mid-processing lets the provider retry cleanly.
//  3. On success: payment -> SUCCEEDED (paid_at), booking PAYMENT_PENDING ->
//     CONFIRMED through the state machine, ONE balanced ledger allocation
//     (§9.1), the receipt, and queued notification stubs — atomically.
func (s *Service) HandleWebhook(ctx context.Context, headers http.Header, rawBody []byte) (WebhookOutcome, error) {
	if err := s.provider.VerifyWebhook(headers, rawBody); err != nil {
		return WebhookOutcome{}, err
	}
	event, err := ParseWebhookEvent(rawBody)
	if err != nil {
		return WebhookOutcome{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return WebhookOutcome{}, fmt.Errorf("payments: begin webhook: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after Commit

	// Dedupe: first delivery claims the event; replays no-op (spec §14).
	bodyHash := sha256.Sum256(rawBody)
	var claimID string
	err = tx.QueryRow(ctx, `
		INSERT INTO webhook_events (provider, event_reference, raw_body_hash)
		VALUES ($1, $2, $3)
		ON CONFLICT (provider, event_reference) DO NOTHING
		RETURNING id`, s.provider.Name(), event.Reference, hex.EncodeToString(bodyHash[:])).Scan(&claimID)
	if errors.Is(err, pgx.ErrNoRows) {
		tx.Rollback(ctx) //nolint:errcheck // nothing was written beyond the failed claim
		return WebhookOutcome{Replay: true}, nil
	}
	if err != nil {
		return WebhookOutcome{}, fmt.Errorf("payments: claim webhook event: %w", err)
	}

	p, err := s.repo.GetByProviderReferenceForUpdate(ctx, tx, event.Reference)
	if errors.Is(err, ErrNotFound) {
		// Unknown payment: roll back (dropping the dedupe claim) so a
		// legitimate later retry can still be processed.
		return WebhookOutcome{}, ErrUnknownReference
	}
	if err != nil {
		return WebhookOutcome{}, err
	}

	// Already final (e.g. provider resent a differently-referenced event):
	// record-only, zero side effects.
	if p.Status == StatusSucceeded || p.Status == StatusRefundPending ||
		p.Status == StatusPartiallyRefunded || p.Status == StatusRefunded {
		if err := tx.Commit(ctx); err != nil {
			return WebhookOutcome{}, fmt.Errorf("payments: commit record-only webhook: %w", err)
		}
		return WebhookOutcome{Replay: true}, nil
	}

	if event.Status != "success" {
		// Failure/expiry events: mark the payment FAILED and the booking
		// PAYMENT_FAILED (legal §8.2 edge) so the tourist can retry.
		if _, err := s.repo.Transition(ctx, tx, p, StatusFailed); err != nil {
			return WebhookOutcome{}, err
		}
		if _, _, err := s.bookings.TransitionTx(ctx, tx, p.BookingID, "", bookings.StatusPaymentFailed,
			json.RawMessage(`{"action":"payment.failed","provider_reference":"`+p.ProviderReference+`"}`)); err != nil {
			return WebhookOutcome{}, fmt.Errorf("payments: fail booking: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return WebhookOutcome{}, fmt.Errorf("payments: commit failure webhook: %w", err)
		}
		return WebhookOutcome{}, nil
	}

	if err := s.confirmPayment(ctx, tx, p); err != nil {
		return WebhookOutcome{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return WebhookOutcome{}, fmt.Errorf("payments: commit webhook: %w", err)
	}
	if s.OnConfirmed != nil {
		hookCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		s.OnConfirmed(hookCtx, p.BookingID)
		cancel()
	}
	return WebhookOutcome{}, nil
}

// confirmPayment runs the full success side effects inside tx: SUCCEEDED,
// booking CONFIRMED, balanced ledger allocation, receipt, notifications.
func (s *Service) confirmPayment(ctx context.Context, tx pgx.Tx, p Payment) error {
	p, err := s.repo.Transition(ctx, tx, p, StatusSucceeded)
	if err != nil {
		return err
	}

	booking, _, err := s.bookings.TransitionTx(ctx, tx, p.BookingID, "", bookings.StatusConfirmed,
		json.RawMessage(`{"action":"payment.succeeded","provider_reference":"`+p.ProviderReference+`"}`))
	if err != nil {
		return fmt.Errorf("payments: confirm booking: %w", err)
	}

	// Ledger allocation (§9.1): percentages from system_settings, integer
	// pesewas, exact-sum invariant by construction (Allocate).
	alloc, err := s.allocationFor(ctx, tx, booking)
	if err != nil {
		return err
	}
	clearing, err := s.ledger.AccountID(ctx, tx, "platform", nil, "tourist_clearing")
	if err != nil {
		return err
	}
	revenue, err := s.ledger.AccountID(ctx, tx, "platform", nil, "platform_revenue")
	if err != nil {
		return err
	}
	levy, err := s.ledger.AccountID(ctx, tx, "platform", nil, "tourism_levy_payable")
	if err != nil {
		return err
	}
	payable, err := s.ledger.AccountID(ctx, tx, "platform", nil, "guide_payable_pending")
	if err != nil {
		return err
	}
	entries := []ledger.Entry{
		{AccountID: clearing, Direction: ledger.Debit, AmountMinor: alloc.Gross},
	}
	if alloc.PlatformFee > 0 {
		entries = append(entries, ledger.Entry{AccountID: revenue, Direction: ledger.Credit, AmountMinor: alloc.PlatformFee})
	}
	if alloc.TourismLevy > 0 {
		entries = append(entries, ledger.Entry{AccountID: levy, Direction: ledger.Credit, AmountMinor: alloc.TourismLevy})
	}
	if alloc.GuidePayable > 0 {
		entries = append(entries, ledger.Entry{AccountID: payable, Direction: ledger.Credit, AmountMinor: alloc.GuidePayable})
	}
	if _, err := s.ledger.Post(ctx, tx, ledger.Transaction{
		Reference: "PAY:" + p.ProviderReference,
		Type:      "PAYMENT",
		BookingID: booking.ID,
		Entries:   entries,
	}); err != nil {
		return fmt.Errorf("payments: post allocation: %w", err)
	}

	// Receipt (spec §17): render + store + row, all inside the transaction.
	content, err := s.receiptContent(ctx, tx, booking, p, alloc)
	if err != nil {
		return err
	}
	if _, err := s.receipts.Issue(ctx, tx, booking.ID, content); err != nil {
		return fmt.Errorf("payments: issue receipt: %w", err)
	}

	// Notification stubs — the worker owns actual delivery (spec §20/§21).
	for _, n := range []struct {
		userID, channel, template string
	}{
		{booking.TouristID, "email", "booking.payment_confirmed"},
		{deref(booking.GuideID), "push", "booking.new_confirmed"},
	} {
		if n.userID == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO notifications (user_id, channel, template, status)
			VALUES ($1, $2, $3, 'queued')`, n.userID, n.channel, n.template); err != nil {
			return fmt.Errorf("payments: queue notification: %w", err)
		}
	}
	return nil
}

// allocationFor reads the configured percentages and splits the booking's
// server-authoritative amount.
func (s *Service) allocationFor(ctx context.Context, tx pgx.Tx, b bookings.Booking) (Allocation, error) {
	if b.Amount == nil {
		return Allocation{}, fmt.Errorf("payments: booking %s has no amount", b.ID)
	}
	gross, err := bookings.ParseDecimal(*b.Amount)
	if err != nil {
		return Allocation{}, fmt.Errorf("payments: booking amount: %w", err)
	}
	feePct, err := settingTextTx(ctx, tx, "platform_fee_pct")
	if err != nil {
		return Allocation{}, err
	}
	levyPct, err := settingTextTx(ctx, tx, "tourism_levy_pct")
	if err != nil {
		return Allocation{}, err
	}
	feeCenti, err := bookings.ParseDecimal(feePct)
	if err != nil {
		return Allocation{}, fmt.Errorf("payments: platform_fee_pct %q: %w", feePct, err)
	}
	levyCenti, err := bookings.ParseDecimal(levyPct)
	if err != nil {
		return Allocation{}, fmt.Errorf("payments: tourism_levy_pct %q: %w", levyPct, err)
	}
	return Allocate(gross, feeCenti, levyCenti), nil
}

// receiptContent gathers the §17 receipt fields. The insurance indicator is
// included only when the guide holds an approved, unexpired insurance
// document (never displayable without valid evidence — stop condition 7).
func (s *Service) receiptContent(ctx context.Context, tx pgx.Tx, b bookings.Booking, p Payment, alloc Allocation) (receipts.Content, error) {
	var pkgName, touristName string
	var guideName *string
	var insured bool
	err := tx.QueryRow(ctx, `
		SELECT tp.name, up.full_name, gp.public_name,
			EXISTS(
				SELECT 1 FROM guide_documents d
				WHERE d.guide_id = b2.guide_id AND d.type = 'insurance'
				  AND d.status = 'approved'
				  AND (d.expires_at IS NULL OR d.expires_at > now())
			)
		FROM bookings b2
		JOIN tour_packages tp ON tp.id = b2.package_id
		JOIN tourist_profiles up ON up.user_id = b2.tourist_id
		LEFT JOIN guide_profiles gp ON gp.user_id = b2.guide_id
		WHERE b2.id = $1`, b.ID).Scan(&pkgName, &touristName, &guideName, &insured)
	if err != nil {
		return receipts.Content{}, fmt.Errorf("payments: receipt content: %w", err)
	}
	guide := "Guide TBD"
	if guideName != nil && *guideName != "" {
		guide = *guideName
	}
	return receipts.Content{
		BookingReference:  b.Reference,
		PackageName:       pkgName,
		StartsAt:          b.StartsAt,
		TouristName:       touristName,
		GuideName:         guide,
		GrossAmount:       bookings.FormatMinor(alloc.Gross),
		Currency:          b.Currency,
		PaymentMethod:     p.Provider,
		ProviderReference: p.ProviderReference,
		PlatformFee:       bookings.FormatMinor(alloc.PlatformFee),
		TourismLevy:       bookings.FormatMinor(alloc.TourismLevy),
		GuidePayable:      bookings.FormatMinor(alloc.GuidePayable),
		InsuranceActive:   insured,
	}, nil
}

// RefundOutcome is the refund response: the refunded payment and the reversal
// ledger transaction id.
type RefundOutcome struct {
	Payment           Payment
	RefundID          string
	ReversalReference string
	Replayed          bool
}

// Refund performs a full refund of a SUCCEEDED payment (spec §4.5, §13.3;
// privileged — the handler requires payments.refund and audits):
//
//  1. Provider refund call (mock in sandbox).
//  2. payment SUCCEEDED -> REFUND_PENDING -> REFUNDED (§8.3).
//  3. Booking driven to REFUNDED strictly through the §8.2 state machine
//     (CONFIRMED -> CANCELLED_BY_ADMIN -> REFUND_PENDING -> REFUNDED etc.).
//  4. The original ledger allocation is REVERSED with compensating entries;
//     originals are never touched (§9.2).
//  5. Idempotent on Idempotency-Key; a replay returns the first result.
func (s *Service) Refund(ctx context.Context, paymentID, reason, idemKey string) (RefundOutcome, error) {
	if idemKey == "" || len(idemKey) > 200 {
		return RefundOutcome{}, fmt.Errorf("%w: Idempotency-Key header is required (max 200 chars)", ErrValidation)
	}

	p, err := s.repo.GetByID(ctx, paymentID)
	if errors.Is(err, ErrNotFound) {
		return RefundOutcome{}, ErrNotFound
	}
	if err != nil {
		return RefundOutcome{}, err
	}

	// Idempotent replay short-circuit BEFORE calling the provider: the key
	// was fully processed if it carries an entity.
	scope := "payment.refund:" + paymentID
	payloadHash := hashStrings(paymentID, reason)
	if replay, ok, err := s.replayRefund(ctx, idemKey, scope, payloadHash); err != nil || ok {
		return replay, err
	}

	if p.Status != StatusSucceeded && p.Status != StatusPartiallyRefunded {
		return RefundOutcome{}, fmt.Errorf("%w: payment is %s", ErrNotRefundable, p.Status)
	}

	refund, err := s.provider.Refund(ctx, RefundRequest{
		ProviderReference: p.ProviderReference,
		AmountMinor:       mustParseMinor(p.Amount),
		Reason:            reason,
	})
	if err != nil {
		return RefundOutcome{}, fmt.Errorf("payments: provider refund: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RefundOutcome{}, fmt.Errorf("payments: begin refund: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after Commit

	// Claim the idempotency key (first writer wins; the replay path above
	// already handled the completed case, so a conflict here means an
	// in-flight concurrent refund — treat as conflict).
	var claimed string
	err = tx.QueryRow(ctx, `
		INSERT INTO idempotency_keys (key, scope, response_code, response_body_hash, expires_at)
		VALUES ($1, $2, NULL, $3, now() + interval '24 hours')
		ON CONFLICT (key, scope) DO NOTHING
		RETURNING key`, idemKey, scope, payloadHash).Scan(&claimed)
	if errors.Is(err, pgx.ErrNoRows) {
		return RefundOutcome{}, fmt.Errorf("%w: concurrent refund with this key is in progress", ErrIdempotencyConflict)
	}
	if err != nil {
		return RefundOutcome{}, fmt.Errorf("payments: claim refund key: %w", err)
	}

	p, err = s.repo.GetByIDForUpdate(ctx, tx, paymentID)
	if err != nil {
		return RefundOutcome{}, err
	}
	if p.Status != StatusSucceeded && p.Status != StatusPartiallyRefunded {
		return RefundOutcome{}, fmt.Errorf("%w: payment is %s", ErrNotRefundable, p.Status)
	}
	if p, err = s.repo.Transition(ctx, tx, p, StatusRefundPending); err != nil {
		return RefundOutcome{}, err
	}
	if p, err = s.repo.Transition(ctx, tx, p, StatusRefunded); err != nil {
		return RefundOutcome{}, err
	}

	refundID, err := s.repo.InsertRefund(ctx, tx, p.ID, refund.Reference, p.Amount, reason)
	if err != nil {
		return RefundOutcome{}, err
	}

	if err := s.refundBooking(ctx, tx, p.BookingID); err != nil {
		return RefundOutcome{}, err
	}

	// Reverse the original allocation: originals preserved, compensating
	// entries net every account back (§9.2).
	var origTxnID string
	err = tx.QueryRow(ctx, `
		SELECT id FROM ledger_transactions WHERE reference = $1`,
		"PAY:"+p.ProviderReference).Scan(&origTxnID)
	if errors.Is(err, pgx.ErrNoRows) {
		return RefundOutcome{}, fmt.Errorf("payments: original ledger posting missing for %s", p.ProviderReference)
	}
	if err != nil {
		return RefundOutcome{}, fmt.Errorf("payments: load original posting: %w", err)
	}
	revRef := "REV:" + p.ProviderReference
	if _, err := s.ledger.Reversal(ctx, tx, origTxnID, revRef, reason); err != nil {
		return RefundOutcome{}, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE idempotency_keys SET entity_id = $3, response_code = 200
		WHERE key = $1 AND scope = $2`, idemKey, scope, refundID); err != nil {
		return RefundOutcome{}, fmt.Errorf("payments: complete refund key: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return RefundOutcome{}, fmt.Errorf("payments: commit refund: %w", err)
	}
	return RefundOutcome{Payment: p, RefundID: refundID, ReversalReference: revRef}, nil
}

// replayRefund returns the completed refund for a reused key.
func (s *Service) replayRefund(ctx context.Context, key, scope, payloadHash string) (RefundOutcome, bool, error) {
	var entityID, hash *string
	err := s.pool.QueryRow(ctx, `
		SELECT entity_id, response_body_hash FROM idempotency_keys
		WHERE key = $1 AND scope = $2`, key, scope).Scan(&entityID, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return RefundOutcome{}, false, nil // first attempt with this key
	}
	if err != nil {
		return RefundOutcome{}, false, fmt.Errorf("payments: read refund key: %w", err)
	}
	if hash == nil || *hash != payloadHash {
		return RefundOutcome{}, false, ErrIdempotencyConflict
	}
	if entityID == nil {
		return RefundOutcome{}, false, fmt.Errorf("%w: refund with this key is in progress", ErrIdempotencyConflict)
	}
	var paymentID string
	if err := s.pool.QueryRow(ctx,
		`SELECT payment_id FROM refunds WHERE id = $1`, *entityID).Scan(&paymentID); err != nil {
		return RefundOutcome{}, false, fmt.Errorf("payments: replay refund lookup: %w", err)
	}
	p, err := s.repo.GetByID(ctx, paymentID)
	if err != nil {
		return RefundOutcome{}, false, err
	}
	return RefundOutcome{Payment: p, RefundID: *entityID, Replayed: true}, true, nil
}

// refundBooking drives the booking to REFUNDED strictly along legal §8.2
// edges. Mid-tour and terminal states are covered; unpaid states are not
// refundable through payments (they carry no succeeded payment).
func (s *Service) refundBooking(ctx context.Context, tx pgx.Tx, bookingID string) error {
	b, err := s.bookings.GetByID(ctx, bookingID)
	if err != nil {
		return fmt.Errorf("payments: refund booking load: %w", err)
	}
	meta := json.RawMessage(`{"action":"payment.refunded"}`)
	step := func(to string) error {
		_, _, err := s.bookings.TransitionTx(ctx, tx, bookingID, "", to, meta)
		return err
	}
	switch b.Status {
	case bookings.StatusRefunded:
		return nil
	case bookings.StatusRefundPending:
		return step(bookings.StatusRefunded)
	case bookings.StatusCompleted, bookings.StatusNoShow,
		bookings.StatusCancelledByTourist, bookings.StatusCancelledByGuide,
		bookings.StatusCancelledByAdmin:
		if err := step(bookings.StatusRefundPending); err != nil {
			return err
		}
		return step(bookings.StatusRefunded)
	case bookings.StatusConfirmed, bookings.StatusGuideEnRoute, bookings.StatusGuideArrived:
		if err := step(bookings.StatusCancelledByAdmin); err != nil {
			return err
		}
		if err := step(bookings.StatusRefundPending); err != nil {
			return err
		}
		return step(bookings.StatusRefunded)
	default:
		return fmt.Errorf("payments: booking in %s cannot be refund-driven", b.Status)
	}
}

// settingTextTx reads a system_settings scalar inside tx.
func settingTextTx(ctx context.Context, tx pgx.Tx, key string) (string, error) {
	var v string
	err := tx.QueryRow(ctx,
		`SELECT value_json #>> '{}' FROM system_settings WHERE key = $1`, key).Scan(&v)
	if err != nil {
		return "", fmt.Errorf("payments: read setting %q: %w", key, err)
	}
	return v, nil
}

// mustParseMinor parses a NUMERIC(14,2) string to pesewas; database values
// are guaranteed well-formed by the schema, so a failure is a hard error.
func mustParseMinor(amount string) int64 {
	v, err := bookings.ParseDecimal(amount)
	if err != nil {
		panic(fmt.Sprintf("payments: corrupt amount in database %q: %v", amount, err))
	}
	return v
}

func hashStrings(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0x1f})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
