// Package privacy implements data-subject rights: account deletion
// (Apple 5.1.1(v), Google Play data-deletion policy) and access/portability
// plus consent records (Ghana Data Protection Act, 2012 — Act 843).
package privacy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when the subject has no such record.
var ErrNotFound = errors.New("privacy: not found")

// Repository holds the explicit SQL for data-subject operations.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository builds a Repository.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// LegalDocument is a published policy version the apps link to.
//
// Body is markdown and may be empty for legacy rows seeded before the text
// existed. Approved is false until counsel signs off; the public site renders
// a draft banner while it is, so the disclosure is driven by data rather than
// by anyone remembering to add a note.
type LegalDocument struct {
	Document    string     `json:"document"`
	Version     string     `json:"version"`
	URL         string     `json:"url"`
	Summary     *string    `json:"summary,omitempty"`
	Body        *string    `json:"body,omitempty"`
	Approved    bool       `json:"approved"`
	ApprovedAt  *time.Time `json:"approved_at,omitempty"`
	PublishedAt time.Time  `json:"published_at"`
}

// CurrentPolicies returns the newest version of each legal document.
func (r *Repository) CurrentPolicies(ctx context.Context) ([]LegalDocument, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT ON (document)
		       document, version, url, summary, body, approved, approved_at, published_at
		FROM legal_documents
		ORDER BY document, published_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("privacy: current policies: %w", err)
	}
	defer rows.Close()
	var out []LegalDocument
	for rows.Next() {
		var d LegalDocument
		if err := rows.Scan(&d.Document, &d.Version, &d.URL, &d.Summary, &d.Body,
			&d.Approved, &d.ApprovedAt, &d.PublishedAt); err != nil {
			return nil, fmt.Errorf("privacy: scan policy: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// RecordConsent appends one acceptance. Consent history is append-only
// (Act 843 s.20 requires consent to be demonstrable), so this never updates.
func (r *Repository) RecordConsent(ctx context.Context, userID, document, version, source string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO consent_records (user_id, document, version, source)
		VALUES ($1, $2, $3, $4)`, userID, document, version, source)
	if err != nil {
		return fmt.Errorf("privacy: record consent: %w", err)
	}
	return nil
}

// ConsentRecord is one stored acceptance.
type ConsentRecord struct {
	Document   string    `json:"document"`
	Version    string    `json:"version"`
	AcceptedAt time.Time `json:"accepted_at"`
	Source     string    `json:"source"`
}

// Consents returns every acceptance the subject has made, newest first.
func (r *Repository) Consents(ctx context.Context, userID string) ([]ConsentRecord, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT document, version, accepted_at, source
		FROM consent_records WHERE user_id = $1
		ORDER BY accepted_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("privacy: consents: %w", err)
	}
	defer rows.Close()
	out := []ConsentRecord{}
	for rows.Next() {
		var c ConsentRecord
		if err := rows.Scan(&c.Document, &c.Version, &c.AcceptedAt, &c.Source); err != nil {
			return nil, fmt.Errorf("privacy: scan consent: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeletionBlocker explains why an account cannot be erased right now.
// Both stores accept a refusal provided the user is told the specific reason
// and it is temporary — an open-ended "contact support" is not acceptable.
type DeletionBlocker struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// DeletionBlockers returns every reason erasure must wait. Empty means the
// account can be anonymized now.
func (r *Repository) DeletionBlockers(ctx context.Context, userID string) ([]DeletionBlocker, error) {
	var blockers []DeletionBlocker

	// An in-flight tour has a counterparty depending on this identity, and a
	// payment that has not finished settling.
	var activeBookings int
	err := r.pool.QueryRow(ctx, `
		SELECT count(*) FROM bookings
		WHERE (tourist_id = $1 OR guide_id = $1)
		  AND status IN ('PAYMENT_PENDING','CONFIRMED','GUIDE_EN_ROUTE',
		                 'GUIDE_ARRIVED','IN_PROGRESS','REFUND_PENDING')`,
		userID).Scan(&activeBookings)
	if err != nil {
		return nil, fmt.Errorf("privacy: active bookings: %w", err)
	}
	if activeBookings > 0 {
		blockers = append(blockers, DeletionBlocker{
			Reason: "active_booking",
			Message: "You have a booking that has not finished yet. " +
				"You can delete your account once it is completed or cancelled.",
		})
	}

	// Money we still owe cannot be paid to an anonymized payout account.
	var pendingPayouts int
	err = r.pool.QueryRow(ctx, `
		SELECT count(*) FROM payouts
		WHERE guide_id = $1
		  AND status IN ('PENDING_ELIGIBILITY','ELIGIBLE','QUEUED',
		                 'PROCESSING','RETRY_QUEUED','MANUAL_REVIEW')`,
		userID).Scan(&pendingPayouts)
	if err != nil {
		return nil, fmt.Errorf("privacy: pending payouts: %w", err)
	}
	if pendingPayouts > 0 {
		blockers = append(blockers, DeletionBlocker{
			Reason: "pending_payout",
			Message: "You have earnings that have not been paid out yet. " +
				"You can delete your account once your final payout has settled.",
		})
	}

	return blockers, nil
}

// AnonymizeResult lists the data classes cleared by an erasure.
type AnonymizeResult struct {
	Cleared []string `json:"cleared"`
	// R2 object keys the caller must delete from private storage. The DB
	// transaction cannot roll back an object-store delete, so the rows go
	// first and the objects are removed after it commits.
	ObjectKeys []string `json:"-"`
}

// Anonymize irreversibly strips personal data from the subject's records in
// one transaction, leaving append-only financial and audit rows intact.
//
// What is deliberately NOT touched: bookings, payments, ledger_*, receipts,
// refunds and audit_logs. Those are append-only by spec §8 and are retained
// under Act 843's legal-obligation exemption (tax and tourism-levy
// reconciliation). They reference the user by id only; with the users row
// anonymized they no longer identify a person.
func (r *Repository) Anonymize(ctx context.Context, userID string) (AnonymizeResult, error) {
	var result AnonymizeResult

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return result, fmt.Errorf("privacy: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Collect the private-storage keys before deleting the rows that name them.
	rows, err := tx.Query(ctx,
		`SELECT object_key FROM guide_documents WHERE guide_id = $1`, userID)
	if err != nil {
		return result, fmt.Errorf("privacy: list documents: %w", err)
	}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			rows.Close()
			return result, fmt.Errorf("privacy: scan document key: %w", err)
		}
		result.ObjectKeys = append(result.ObjectKeys, key)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("privacy: documents: %w", err)
	}

	// Identity. The email is replaced with a unique unusable value rather than
	// NULL so the UNIQUE constraint still holds and the address becomes
	// available for re-registration. The password hash is replaced with a
	// non-verifying sentinel, not blanked, so no login path can ever match it.
	tag, err := tx.Exec(ctx, `
		UPDATE users
		SET email         = 'deleted+' || id::text || '@deleted.invalid',
		    phone_e164    = NULL,
		    password_hash = 'deleted',
		    status        = 'deleted',
		    anonymized_at = now(),
		    updated_at    = now()
		WHERE id = $1 AND status <> 'deleted'`, userID)
	if err != nil {
		return result, fmt.Errorf("privacy: anonymize user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return result, ErrNotFound
	}
	result.Cleared = append(result.Cleared, "identity")

	// Each statement is paired with the data class it clears so the
	// account_deletions receipt reflects what actually happened.
	steps := []struct {
		class string
		sql   string
	}{
		{"tourist_profile", `DELETE FROM tourist_profiles WHERE user_id = $1`},
		{"guide_documents", `DELETE FROM guide_documents WHERE guide_id = $1`},
		{"payout_accounts", `DELETE FROM payout_accounts WHERE guide_id = $1`},
		{"mfa_secrets", `DELETE FROM mfa_secrets WHERE user_id = $1`},
		{"otp_codes", `DELETE FROM otp_codes WHERE user_id = $1`},
		{"sessions", `DELETE FROM refresh_sessions WHERE user_id = $1`},
		{"notifications", `DELETE FROM notifications WHERE user_id = $1`},
		// Movement history is personal data with no financial retention basis
		// (§11.2 keeps only what safety/audit needs, and the SOS events below
		// carry that).
		{"location_checkpoints", `DELETE FROM location_checkpoints WHERE guide_id = $1`},
	}
	for _, step := range steps {
		tag, err := tx.Exec(ctx, step.sql, userID)
		if err != nil {
			return result, fmt.Errorf("privacy: clear %s: %w", step.class, err)
		}
		if tag.RowsAffected() > 0 {
			result.Cleared = append(result.Cleared, step.class)
		}
	}

	// The public guide profile stays (bookings and reviews reference it) but
	// is stripped of anything identifying.
	tag, err = tx.Exec(ctx, `
		UPDATE guide_profiles
		SET public_name = 'Former guide', bio = NULL, updated_at = now()
		WHERE user_id = $1`, userID)
	if err != nil {
		return result, fmt.Errorf("privacy: anonymize guide profile: %w", err)
	}
	if tag.RowsAffected() > 0 {
		result.Cleared = append(result.Cleared, "guide_profile")
	}

	// Review bodies are retained: they are statements about a guide that other
	// travellers rely on, and the author is already referenced by id only. Any
	// free text the author wrote about themselves is not stored here.

	cleared, err := json.Marshal(result.Cleared)
	if err != nil {
		return result, fmt.Errorf("privacy: marshal cleared: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO account_deletions (user_id, completed_at, cleared)
		VALUES ($1, now(), $2)`, userID, cleared); err != nil {
		return result, fmt.Errorf("privacy: record deletion: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return result, fmt.Errorf("privacy: commit: %w", err)
	}
	return result, nil
}

// RecordBlocked notes a refused erasure so a reviewer or regulator can see the
// request was received and why it could not be completed.
func (r *Repository) RecordBlocked(ctx context.Context, userID, reason string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO account_deletions (user_id, blocked_reason) VALUES ($1, $2)`,
		userID, reason)
	if err != nil {
		return fmt.Errorf("privacy: record blocked deletion: %w", err)
	}
	return nil
}

// Export is the subject-access payload (Act 843 s.32; GDPR Art 15/20 shape).
type Export struct {
	GeneratedAt time.Time        `json:"generated_at"`
	Account     map[string]any   `json:"account"`
	Profile     map[string]any   `json:"profile,omitempty"`
	Bookings    []map[string]any `json:"bookings"`
	Reviews     []map[string]any `json:"reviews"`
	Consents    []ConsentRecord  `json:"consents"`
	Notes       []string         `json:"notes"`
}

// BuildExport assembles everything held about one subject.
func (r *Repository) BuildExport(ctx context.Context, userID string) (Export, error) {
	out := Export{
		GeneratedAt: time.Now().UTC(),
		Bookings:    []map[string]any{},
		Reviews:     []map[string]any{},
		Notes: []string{
			"Financial records (payments, ledger entries, receipts) are retained " +
				"for tax and tourism-levy reconciliation and are not included here.",
			"Location history is kept only for the retention window in the privacy policy.",
		},
	}

	var email string
	var phone *string
	var status string
	var createdAt time.Time
	var lastLogin *time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT email, phone_e164, status, created_at, last_login_at
		FROM users WHERE id = $1`, userID).
		Scan(&email, &phone, &status, &createdAt, &lastLogin)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, fmt.Errorf("privacy: export account: %w", err)
	}
	out.Account = map[string]any{
		"email":         email,
		"phone_e164":    phone,
		"status":        status,
		"created_at":    createdAt,
		"last_login_at": lastLogin,
	}

	var fullName string
	var nationality, language, ecName, ecPhone *string
	err = r.pool.QueryRow(ctx, `
		SELECT full_name, nationality, preferred_language,
		       emergency_contact_name, emergency_contact_phone_e164
		FROM tourist_profiles WHERE user_id = $1`, userID).
		Scan(&fullName, &nationality, &language, &ecName, &ecPhone)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Guide-only account; no tourist profile to export.
	case err != nil:
		return out, fmt.Errorf("privacy: export profile: %w", err)
	default:
		out.Profile = map[string]any{
			"full_name":                    fullName,
			"nationality":                  nationality,
			"preferred_language":           language,
			"emergency_contact_name":       ecName,
			"emergency_contact_phone_e164": ecPhone,
		}
	}

	bookingRows, err := r.pool.Query(ctx, `
		SELECT reference, status, starts_at, ends_at, num_guests, meeting_point_text
		FROM bookings WHERE tourist_id = $1 OR guide_id = $1
		ORDER BY starts_at DESC`, userID)
	if err != nil {
		return out, fmt.Errorf("privacy: export bookings: %w", err)
	}
	defer bookingRows.Close()
	for bookingRows.Next() {
		var ref, st string
		var startsAt, endsAt time.Time
		var guests int
		var meeting *string
		if err := bookingRows.Scan(&ref, &st, &startsAt, &endsAt, &guests, &meeting); err != nil {
			return out, fmt.Errorf("privacy: scan booking: %w", err)
		}
		out.Bookings = append(out.Bookings, map[string]any{
			"reference": ref, "status": st, "starts_at": startsAt,
			"ends_at": endsAt, "num_guests": guests, "meeting_point": meeting,
		})
	}
	if err := bookingRows.Err(); err != nil {
		return out, fmt.Errorf("privacy: bookings: %w", err)
	}

	reviewRows, err := r.pool.Query(ctx, `
		SELECT rating, body, created_at FROM reviews
		WHERE tourist_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return out, fmt.Errorf("privacy: export reviews: %w", err)
	}
	defer reviewRows.Close()
	for reviewRows.Next() {
		var rating int
		var body *string
		var created time.Time
		if err := reviewRows.Scan(&rating, &body, &created); err != nil {
			return out, fmt.Errorf("privacy: scan review: %w", err)
		}
		out.Reviews = append(out.Reviews, map[string]any{
			"rating": rating, "body": body, "created_at": created,
		})
	}
	if err := reviewRows.Err(); err != nil {
		return out, fmt.Errorf("privacy: reviews: %w", err)
	}

	if out.Consents, err = r.Consents(ctx, userID); err != nil {
		return out, err
	}
	return out, nil
}
