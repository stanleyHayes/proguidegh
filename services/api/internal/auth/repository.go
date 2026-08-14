// Package auth implements the Phase 1 identity slice: registration,
// password login with TOTP step-up, rotating refresh sessions, OTP flows,
// password reset and MFA enrollment (spec §13.1, §15).
package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// User is the identity row from the users table.
type User struct {
	ID           string
	Email        string
	PhoneE164    *string
	PasswordHash string
	Status       string
	LastLoginAt  *time.Time
	CreatedAt    time.Time
}

// Session is a refresh_sessions row.
type Session struct {
	ID        string
	UserID    string
	TokenHash string
	RotatedTo *string
	RevokedAt *time.Time
	ExpiresAt time.Time
	CreatedAt time.Time
}

// OTPCode is an otp_codes row (hash only — plaintext never leaves delivery).
type OTPCode struct {
	ID          string
	Destination string
	Channel     string
	Purpose     string
	CodeHash    string
	Attempts    int
	ExpiresAt   time.Time
}

// MFASecret is an mfa_secrets row. TOTPSecretEncrypted stays encrypted in
// transit through this layer; only the service decrypts it in memory.
type MFASecret struct {
	UserID              string
	TOTPSecretEncrypted string
	EnabledAt           *time.Time
	BackupCodesHash     []string
}

// ErrEmailTaken is returned when registering with an existing email.
var ErrEmailTaken = errors.New("auth: email already registered")

// ErrNotFound is returned when a lookup matches no row.
var ErrNotFound = errors.New("auth: not found")

// Repository owns identity persistence. Queries stay explicit per spec §7.2.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository builds the auth repository.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// CreateUser inserts a user and assigns the initial role in one transaction.
func (r *Repository) CreateUser(ctx context.Context, email string, phone *string, passwordHash, roleCode string) (User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return User{}, fmt.Errorf("auth: begin create user: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after Commit

	var u User
	err = tx.QueryRow(ctx, `
		INSERT INTO users (email, phone_e164, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, email, phone_e164, password_hash, status, last_login_at, created_at`,
		email, phone, passwordHash).
		Scan(&u.ID, &u.Email, &u.PhoneE164, &u.PasswordHash, &u.Status, &u.LastLoginAt, &u.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return User{}, ErrEmailTaken
		}
		return User{}, fmt.Errorf("auth: insert user: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id)
		SELECT $1, id FROM roles WHERE code = $2`, u.ID, roleCode); err != nil {
		return User{}, fmt.Errorf("auth: assign role %s: %w", roleCode, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return User{}, fmt.Errorf("auth: commit create user: %w", err)
	}
	return u, nil
}

// FindUserByEmail returns the active user for an email.
func (r *Repository) FindUserByEmail(ctx context.Context, email string) (User, error) {
	return r.findUser(ctx, `email = $1`, email)
}

// FindUserByID returns the user by primary key.
func (r *Repository) FindUserByID(ctx context.Context, id string) (User, error) {
	return r.findUser(ctx, `id = $1`, id)
}

func (r *Repository) findUser(ctx context.Context, where string, arg any) (User, error) {
	var u User
	err := r.pool.QueryRow(ctx, `
		SELECT id, email, phone_e164, password_hash, status, last_login_at, created_at
		FROM users WHERE `+where, arg).
		Scan(&u.ID, &u.Email, &u.PhoneE164, &u.PasswordHash, &u.Status, &u.LastLoginAt, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("auth: find user: %w", err)
	}
	return u, nil
}

// SetPassword replaces the password hash (reset flow).
func (r *Repository) SetPassword(ctx context.Context, userID, passwordHash string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1`,
		userID, passwordHash)
	if err != nil {
		return fmt.Errorf("auth: set password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// TouchLastLogin stamps last_login_at.
func (r *Repository) TouchLastLogin(ctx context.Context, userID string) {
	r.pool.Exec(ctx, `UPDATE users SET last_login_at = now() WHERE id = $1`, userID) //nolint:errcheck
}

// ---------------------------------------------------------------------------
// Refresh sessions
// ---------------------------------------------------------------------------

// CreateSession inserts a refresh session for tokenHash.
func (r *Repository) CreateSession(ctx context.Context, userID, tokenHash, ip, userAgent string, expiresAt time.Time) (Session, error) {
	var s Session
	err := r.pool.QueryRow(ctx, `
		INSERT INTO refresh_sessions (user_id, token_hash, ip, user_agent, expires_at)
		VALUES ($1, $2, NULLIF($3, '')::inet, NULLIF($4, ''), $5)
		RETURNING id, user_id, token_hash, rotated_to, revoked_at, expires_at, created_at`,
		userID, tokenHash, ip, userAgent, expiresAt).
		Scan(&s.ID, &s.UserID, &s.TokenHash, &s.RotatedTo, &s.RevokedAt, &s.ExpiresAt, &s.CreatedAt)
	if err != nil {
		return Session{}, fmt.Errorf("auth: create session: %w", err)
	}
	return s, nil
}

// GetSessionByTokenHash looks up a session by the hash of a presented token.
func (r *Repository) GetSessionByTokenHash(ctx context.Context, tokenHash string) (Session, error) {
	var s Session
	err := r.pool.QueryRow(ctx, `
		SELECT id, user_id, token_hash, rotated_to, revoked_at, expires_at, created_at
		FROM refresh_sessions WHERE token_hash = $1`, tokenHash).
		Scan(&s.ID, &s.UserID, &s.TokenHash, &s.RotatedTo, &s.RevokedAt, &s.ExpiresAt, &s.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("auth: get session: %w", err)
	}
	return s, nil
}

// MarkRotated points the old session at its replacement and revokes it.
func (r *Repository) MarkRotated(ctx context.Context, oldID, newID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE refresh_sessions
		SET rotated_to = $2, revoked_at = now()
		WHERE id = $1 AND revoked_at IS NULL`, oldID, newID)
	if err != nil {
		return fmt.Errorf("auth: mark rotated: %w", err)
	}
	return nil
}

// RevokeSession revokes one session by id.
func (r *Repository) RevokeSession(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE refresh_sessions SET revoked_at = now()
		WHERE id = $1 AND revoked_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("auth: revoke session: %w", err)
	}
	return nil
}

// RevokeChain revokes a session and every session rotated from it (reuse
// detection, spec §15.1). Walks forward along rotated_to links.
func (r *Repository) RevokeChain(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `
		WITH RECURSIVE chain AS (
			SELECT id, rotated_to FROM refresh_sessions WHERE id = $1
			UNION
			SELECT rs.id, rs.rotated_to FROM refresh_sessions rs
			JOIN chain c ON c.rotated_to = rs.id
		)
		UPDATE refresh_sessions SET revoked_at = now()
		WHERE id IN (SELECT id FROM chain) AND revoked_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("auth: revoke chain: %w", err)
	}
	return nil
}

// RevokeAllForUser revokes every active session for a user (password reset,
// role removal, compromise).
func (r *Repository) RevokeAllForUser(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE refresh_sessions SET revoked_at = now()
		WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	if err != nil {
		return fmt.Errorf("auth: revoke user sessions: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// OTP codes
// ---------------------------------------------------------------------------

// CreateOTP stores a hashed code for destination+purpose.
func (r *Repository) CreateOTP(ctx context.Context, userID *string, destination, channel, purpose, codeHash string, expiresAt time.Time) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO otp_codes (user_id, destination, channel, purpose, code_hash, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		userID, destination, channel, purpose, codeHash, expiresAt)
	if err != nil {
		return fmt.Errorf("auth: create otp: %w", err)
	}
	return nil
}

// LatestOTP returns the most recent unconsumed code for destination+purpose.
func (r *Repository) LatestOTP(ctx context.Context, destination, purpose string) (OTPCode, error) {
	var o OTPCode
	err := r.pool.QueryRow(ctx, `
		SELECT id, destination, channel, purpose, code_hash, attempts, expires_at
		FROM otp_codes
		WHERE destination = $1 AND purpose = $2 AND consumed_at IS NULL
		ORDER BY created_at DESC
		LIMIT 1`, destination, purpose).
		Scan(&o.ID, &o.Destination, &o.Channel, &o.Purpose, &o.CodeHash, &o.Attempts, &o.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return OTPCode{}, ErrNotFound
	}
	if err != nil {
		return OTPCode{}, fmt.Errorf("auth: latest otp: %w", err)
	}
	return o, nil
}

// IncrementOTPAttempts bumps the attempt counter and returns the new value.
func (r *Repository) IncrementOTPAttempts(ctx context.Context, id string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `
		UPDATE otp_codes SET attempts = attempts + 1 WHERE id = $1 RETURNING attempts`, id).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("auth: bump otp attempts: %w", err)
	}
	return n, nil
}

// ConsumeOTP marks a code used (single use).
func (r *Repository) ConsumeOTP(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE otp_codes SET consumed_at = now() WHERE id = $1 AND consumed_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("auth: consume otp: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// MFA
// ---------------------------------------------------------------------------

// GetMFA returns the user's MFA row, or ErrNotFound when none exists.
func (r *Repository) GetMFA(ctx context.Context, userID string) (MFASecret, error) {
	var m MFASecret
	err := r.pool.QueryRow(ctx, `
		SELECT user_id, totp_secret_encrypted, enabled_at, backup_codes_hash
		FROM mfa_secrets WHERE user_id = $1`, userID).
		Scan(&m.UserID, &m.TOTPSecretEncrypted, &m.EnabledAt, &m.BackupCodesHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return MFASecret{}, ErrNotFound
	}
	if err != nil {
		return MFASecret{}, fmt.Errorf("auth: get mfa: %w", err)
	}
	return m, nil
}

// UpsertPendingMFA stores a freshly enrolled (not yet enabled) secret.
func (r *Repository) UpsertPendingMFA(ctx context.Context, userID, encryptedSecret string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO mfa_secrets (user_id, totp_secret_encrypted)
		VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE
		SET totp_secret_encrypted = EXCLUDED.totp_secret_encrypted,
		    enabled_at = NULL,
		    backup_codes_hash = '[]'::jsonb,
		    updated_at = now()`, userID, encryptedSecret)
	if err != nil {
		return fmt.Errorf("auth: upsert mfa: %w", err)
	}
	return nil
}

// EnableMFA marks the secret verified and stores backup-code hashes.
func (r *Repository) EnableMFA(ctx context.Context, userID string, backupHashes []string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE mfa_secrets
		SET enabled_at = now(), backup_codes_hash = $2, updated_at = now()
		WHERE user_id = $1`, userID, backupHashes)
	if err != nil {
		return fmt.Errorf("auth: enable mfa: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Tourist profile creation (register intent=tourist)
// ---------------------------------------------------------------------------

// CreateTouristProfile inserts the profile shell created at registration.
func (r *Repository) CreateTouristProfile(ctx context.Context, userID, fullName string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO tourist_profiles (user_id, full_name) VALUES ($1, $2)
		ON CONFLICT (user_id) DO NOTHING`, userID, fullName)
	if err != nil {
		return fmt.Errorf("auth: create tourist profile: %w", err)
	}
	return nil
}
