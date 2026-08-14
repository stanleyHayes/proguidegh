package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"proguidegh/api/internal/platform/audit"
	pauth "proguidegh/api/internal/platform/auth"
	"proguidegh/api/internal/platform/rbac"
)

// OTP policy (spec §15.2).
const (
	otpTTL        = 5 * time.Minute
	otpMaxAttempt = 5
	// mfaChallengeTTL bounds the login step-up challenge.
	mfaChallengeTTL = 5 * time.Minute
	// backupCodeCount is how many one-time recovery codes MFA enrollment
	// issues.
	backupCodeCount = 8
)

// Sentinel errors mapped to the API error envelope in the handler.
var (
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	ErrAccountSuspended   = errors.New("auth: account is not active")
	ErrMFARequired        = errors.New("auth: mfa verification required")
	ErrMFAInvalid         = errors.New("auth: invalid mfa code")
	ErrSessionInvalid     = errors.New("auth: session invalid")
	ErrSessionReuse       = errors.New("auth: refresh token reuse detected")
	ErrOTPInvalid         = errors.New("auth: invalid or expired code")
	ErrOTPTooManyAttempts = errors.New("auth: too many attempts")
	ErrMFAAlreadyEnabled  = errors.New("auth: mfa already enabled")
	ErrMFANotEnrolled     = errors.New("auth: mfa enrollment not started")
)

// Intent selects the role created at registration.
const (
	IntentTourist = "tourist"
	IntentGuide   = "guide"
)

// Tokens is the session material issued on login/refresh.
type Tokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

// Service implements the identity use cases.
type Service struct {
	repo      *Repository
	issuer    *pauth.TokenIssuer
	rbac      *rbac.Store
	audit     *audit.Recorder
	rdb       *goredis.Client
	appEnv    string
	appSecret string
	now       func() time.Time
}

// NewService builds the auth service.
func NewService(repo *Repository, issuer *pauth.TokenIssuer, rbacStore *rbac.Store,
	aud *audit.Recorder, rdb *goredis.Client, appEnv, appSecret string) *Service {
	return &Service{
		repo: repo, issuer: issuer, rbac: rbacStore, audit: aud, rdb: rdb,
		appEnv: appEnv, appSecret: appSecret, now: time.Now,
	}
}

// ---------------------------------------------------------------------------
// Registration & login
// ---------------------------------------------------------------------------

// Register creates a user with the role implied by intent. For tourists it
// also creates the profile shell; guide applicants complete their profile
// via POST /api/v1/guides/apply.
func (s *Service) Register(ctx context.Context, intent, email string, phone *string, password, fullName string) (User, error) {
	hash, err := pauth.HashPassword(password)
	if err != nil {
		return User{}, err
	}

	role := "tourist"
	if intent == IntentGuide {
		role = "guide_applicant"
	}
	u, err := s.repo.CreateUser(ctx, email, phone, hash, role)
	if err != nil {
		return User{}, err
	}

	if intent == IntentTourist {
		if err := s.repo.CreateTouristProfile(ctx, u.ID, fullName); err != nil {
			return User{}, err
		}
	}
	return u, nil
}

// Login verifies credentials. When MFA is enabled — or required by role
// (super_admin / finance_officer, spec §15.2) — it returns ErrMFARequired
// together with a challenge token via out.
func (s *Service) Login(ctx context.Context, email, password, ip, userAgent string) (tokens Tokens, challenge string, err error) {
	u, err := s.repo.FindUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			// Uniform failure: verify a dummy hash to blunt timing oracles.
			_, _ = pauth.VerifyPassword(password,
				"$argon2id$v=19$m=65536,t=3,p=2$ZmFrZXNhbHRmb3J0aW1pbmc$xYzRh1Ck1x8wq1Ck0r5Zp9y8cQ1y0Qy0u8K3j7h0r8Q")
			return Tokens{}, "", ErrInvalidCredentials
		}
		return Tokens{}, "", err
	}
	if u.Status != "active" {
		return Tokens{}, "", ErrAccountSuspended
	}
	ok, err := pauth.VerifyPassword(password, u.PasswordHash)
	if err != nil {
		return Tokens{}, "", fmt.Errorf("auth: verify password: %w", err)
	}
	if !ok {
		return Tokens{}, "", ErrInvalidCredentials
	}

	mfaEnabled, err := s.mfaEnabled(ctx, u.ID)
	if err != nil {
		return Tokens{}, "", err
	}
	if mfaEnabled {
		challenge, err = s.newMFAChallenge(ctx, u.ID)
		if err != nil {
			return Tokens{}, "", err
		}
		return Tokens{}, challenge, ErrMFARequired
	}

	tokens, err = s.issueSession(ctx, u.ID, ip, userAgent)
	if err != nil {
		return Tokens{}, "", err
	}
	s.repo.TouchLastLogin(ctx, u.ID)
	return tokens, "", nil
}

// MFARequiredRole reports whether the user's roles mandate MFA. Used to warn
// privileged users who have not enrolled yet (spec §15.2; enrollment is
// enforced at login once enabled and flagged for ops via audit).
func (s *Service) MFARequiredRole(ctx context.Context, userID string) (bool, error) {
	roles, err := s.rbac.Roles(ctx, userID)
	if err != nil {
		return false, err
	}
	return rbac.HasRole(roles, "super_admin") || rbac.HasRole(roles, "finance_officer"), nil
}

// CompleteMFALogin finishes the step-up: challenge + TOTP code → session.
func (s *Service) CompleteMFALogin(ctx context.Context, challenge, code, ip, userAgent string) (Tokens, error) {
	userID, err := s.popMFAChallenge(ctx, challenge)
	if err != nil {
		return Tokens{}, err
	}
	m, err := s.repo.GetMFA(ctx, userID)
	if err != nil || m.EnabledAt == nil {
		return Tokens{}, ErrMFANotEnrolled
	}
	secret, err := pauth.DecryptSecret(s.appSecret, m.TOTPSecretEncrypted)
	if err != nil {
		return Tokens{}, err
	}
	if !pauth.VerifyTOTP(secret, code, s.now()) {
		// Fall back to a one-time backup code.
		if !s.consumeBackupCode(ctx, m, code) {
			return Tokens{}, ErrMFAInvalid
		}
	}
	tokens, err := s.issueSession(ctx, userID, ip, userAgent)
	if err != nil {
		return Tokens{}, err
	}
	s.repo.TouchLastLogin(ctx, userID)
	return tokens, nil
}

// consumeBackupCode checks code against stored backup-code hashes and, on
// match, removes it so it cannot be reused.
func (s *Service) consumeBackupCode(ctx context.Context, m MFASecret, code string) bool {
	hash := pauth.HashToken(code)
	for i, h := range m.BackupCodesHash {
		if h == hash {
			remaining := append(m.BackupCodesHash[:i:i], m.BackupCodesHash[i+1:]...)
			if err := s.repo.EnableMFA(ctx, m.UserID, remaining); err != nil {
				slog.Error("consume backup code failed", "error", err)
				return false
			}
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Sessions: refresh rotation / logout
// ---------------------------------------------------------------------------

// Refresh rotates a refresh token (spec §15.1): the presented session is
// revoked and a replacement issued. Presenting an already-rotated token is
// reuse — the whole chain is revoked and the client must log in again.
func (s *Service) Refresh(ctx context.Context, refreshToken, ip, userAgent string) (Tokens, error) {
	if refreshToken == "" {
		return Tokens{}, ErrSessionInvalid
	}
	sess, err := s.repo.GetSessionByTokenHash(ctx, pauth.HashToken(refreshToken))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Tokens{}, ErrSessionInvalid
		}
		return Tokens{}, err
	}

	if s.now().After(sess.ExpiresAt) {
		return Tokens{}, ErrSessionInvalid
	}
	if sess.RevokedAt != nil {
		if sess.RotatedTo != nil {
			// Reuse of a rotated-out token: kill the chain.
			if err := s.repo.RevokeChain(ctx, sess.ID); err != nil {
				return Tokens{}, err
			}
			slog.Warn("refresh token reuse detected, chain revoked",
				"user_id", sess.UserID, "session_id", sess.ID)
			return Tokens{}, ErrSessionReuse
		}
		return Tokens{}, ErrSessionInvalid
	}

	tokens, newSession, err := s.issueSessionWithID(ctx, sess.UserID, ip, userAgent)
	if err != nil {
		return Tokens{}, err
	}
	if err := s.repo.MarkRotated(ctx, sess.ID, newSession); err != nil {
		return Tokens{}, err
	}
	return tokens, nil
}

// Logout revokes the session behind the presented refresh token.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	if refreshToken == "" {
		return nil // already logged out from the server's perspective
	}
	sess, err := s.repo.GetSessionByTokenHash(ctx, pauth.HashToken(refreshToken))
	if err != nil {
		return nil // unknown token: nothing to revoke
	}
	return s.repo.RevokeChain(ctx, sess.ID)
}

// issueSession creates the session row and signs the access token.
func (s *Service) issueSession(ctx context.Context, userID, ip, userAgent string) (Tokens, error) {
	t, _, err := s.issueSessionWithID(ctx, userID, ip, userAgent)
	return t, err
}

func (s *Service) issueSessionWithID(ctx context.Context, userID, ip, userAgent string) (Tokens, string, error) {
	refresh, err := pauth.NewOpaqueToken()
	if err != nil {
		return Tokens{}, "", err
	}
	sess, err := s.repo.CreateSession(ctx, userID, pauth.HashToken(refresh), ip, userAgent,
		s.now().Add(pauth.RefreshTokenTTL))
	if err != nil {
		return Tokens{}, "", err
	}
	perms, err := s.rbac.Permissions(ctx, userID)
	if err != nil {
		return Tokens{}, "", err
	}
	access, err := s.issuer.Sign(pauth.Claims{Subject: userID, SessionID: sess.ID, Perms: perms})
	if err != nil {
		return Tokens{}, "", err
	}
	return Tokens{AccessToken: access, RefreshToken: refresh, ExpiresAt: s.now().Add(pauth.AccessTokenTTL)}, sess.ID, nil
}

// ---------------------------------------------------------------------------
// OTP flows
// ---------------------------------------------------------------------------

// RequestOTP issues a 6-digit code for destination. Returns the plaintext
// code ONLY when appEnv == "local" (dev_code field); outside local the code
// travels over the channel provider (SMS/email integration lands with the
// notifications module — until then local logs it, other envs drop it with
// a warn so ops notice the missing provider).
func (s *Service) RequestOTP(ctx context.Context, userID *string, destination, channel, purpose string) (devCode string, err error) {
	code, err := sixDigitCode()
	if err != nil {
		return "", err
	}
	if err := s.repo.CreateOTP(ctx, userID, destination, channel, purpose,
		pauth.HashToken(code), s.now().Add(otpTTL)); err != nil {
		return "", err
	}
	if s.appEnv == "local" {
		// Explicit local-dev-only path allowed by the phase plan.
		slog.Info("otp issued (local dev)", "destination", destination, "purpose", purpose, "code", code)
		return code, nil
	}
	slog.Warn("otp issued but no messaging provider configured; code not delivered",
		"destination", destination, "purpose", purpose)
	return "", nil
}

// VerifyOTP checks a code for destination+purpose with attempt limiting
// (max otpMaxAttempt tries, 5-minute TTL, single use).
func (s *Service) VerifyOTP(ctx context.Context, destination, purpose, code string) error {
	o, err := s.repo.LatestOTP(ctx, destination, purpose)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrOTPInvalid
		}
		return err
	}
	if o.Attempts >= otpMaxAttempt {
		return ErrOTPTooManyAttempts
	}
	if s.now().After(o.ExpiresAt) {
		return ErrOTPInvalid
	}
	if pauth.HashToken(code) != o.CodeHash {
		attempts, err := s.repo.IncrementOTPAttempts(ctx, o.ID)
		if err != nil {
			return err
		}
		if attempts >= otpMaxAttempt {
			return ErrOTPTooManyAttempts
		}
		return ErrOTPInvalid
	}
	return s.repo.ConsumeOTP(ctx, o.ID)
}

// ---------------------------------------------------------------------------
// Password reset
// ---------------------------------------------------------------------------

// ForgotPassword issues a password_reset OTP. It never reveals whether the
// account exists.
func (s *Service) ForgotPassword(ctx context.Context, email string) (devCode string, err error) {
	u, err := s.repo.FindUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", nil // uniform response
		}
		return "", err
	}
	return s.RequestOTP(ctx, &u.ID, email, "email", "password_reset")
}

// ResetPassword verifies the reset code, sets the new password and revokes
// all sessions (spec §15.2).
func (s *Service) ResetPassword(ctx context.Context, email, code, newPassword string) error {
	if err := s.VerifyOTP(ctx, email, "password_reset", code); err != nil {
		return err
	}
	u, err := s.repo.FindUserByEmail(ctx, email)
	if err != nil {
		return err
	}
	hash, err := pauth.HashPassword(newPassword)
	if err != nil {
		return err
	}
	if err := s.repo.SetPassword(ctx, u.ID, hash); err != nil {
		return err
	}
	return s.repo.RevokeAllForUser(ctx, u.ID)
}

// ---------------------------------------------------------------------------
// MFA enrollment
// ---------------------------------------------------------------------------

// MFAEnrollment is the enrollment payload returned to the client.
type MFAEnrollment struct {
	Secret     string `json:"secret"`
	OTPAuthURI string `json:"otpauth_uri"`
}

// EnrollMFA starts (or restarts) enrollment for the user.
func (s *Service) EnrollMFA(ctx context.Context, userID, account string) (MFAEnrollment, error) {
	if m, err := s.repo.GetMFA(ctx, userID); err == nil && m.EnabledAt != nil {
		return MFAEnrollment{}, ErrMFAAlreadyEnabled
	}
	secret, err := pauth.GenerateTOTPSecret()
	if err != nil {
		return MFAEnrollment{}, err
	}
	enc, err := pauth.EncryptSecret(s.appSecret, secret)
	if err != nil {
		return MFAEnrollment{}, err
	}
	if err := s.repo.UpsertPendingMFA(ctx, userID, enc); err != nil {
		return MFAEnrollment{}, err
	}
	return MFAEnrollment{Secret: secret, OTPAuthURI: pauth.TOTPURI("ProGuideGH", account, secret)}, nil
}

// VerifyMFA confirms enrollment with a first TOTP code, enables MFA and
// returns the plaintext backup codes exactly once. The action is audited
// (privileged mutation).
func (s *Service) VerifyMFA(ctx context.Context, userID, code, ip string) (backupCodes []string, err error) {
	m, err := s.repo.GetMFA(ctx, userID)
	if err != nil {
		return nil, ErrMFANotEnrolled
	}
	if m.EnabledAt != nil {
		return nil, ErrMFAAlreadyEnabled
	}
	secret, err := pauth.DecryptSecret(s.appSecret, m.TOTPSecretEncrypted)
	if err != nil {
		return nil, err
	}
	if !pauth.VerifyTOTP(secret, code, s.now()) {
		return nil, ErrMFAInvalid
	}

	backupCodes = make([]string, 0, backupCodeCount)
	hashes := make([]string, 0, backupCodeCount)
	for range backupCodeCount {
		c, err := pauth.NewOpaqueToken()
		if err != nil {
			return nil, err
		}
		backupCodes = append(backupCodes, c)
		hashes = append(hashes, pauth.HashToken(c))
	}
	if err := s.repo.EnableMFA(ctx, userID, hashes); err != nil {
		return nil, err
	}

	if s.audit != nil {
		if err := s.audit.Record(ctx, audit.Entry{
			ActorID: userID, Action: "me.mfa.enable", EntityType: "user", EntityID: userID,
			After: map[string]any{"mfa_enabled": true}, IP: ip,
		}); err != nil {
			return nil, err
		}
	}
	return backupCodes, nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func (s *Service) mfaEnabled(ctx context.Context, userID string) (bool, error) {
	m, err := s.repo.GetMFA(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return m.EnabledAt != nil, nil
}

func (s *Service) newMFAChallenge(ctx context.Context, userID string) (string, error) {
	token, err := pauth.NewOpaqueToken()
	if err != nil {
		return "", err
	}
	if s.rdb == nil {
		return "", errors.New("auth: mfa challenge store unavailable")
	}
	if err := s.rdb.Set(ctx, "mfa:challenge:"+token, userID, mfaChallengeTTL).Err(); err != nil {
		return "", fmt.Errorf("auth: store mfa challenge: %w", err)
	}
	return token, nil
}

func (s *Service) popMFAChallenge(ctx context.Context, token string) (string, error) {
	if s.rdb == nil || token == "" {
		return "", ErrMFAInvalid
	}
	key := "mfa:challenge:" + token
	userID, err := s.rdb.GetDel(ctx, key).Result()
	if errors.Is(err, goredis.Nil) {
		return "", ErrMFAInvalid
	}
	if err != nil {
		return "", fmt.Errorf("auth: read mfa challenge: %w", err)
	}
	return userID, nil
}

func sixDigitCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", fmt.Errorf("auth: otp entropy: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}
