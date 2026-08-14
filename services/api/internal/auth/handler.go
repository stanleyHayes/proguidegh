package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"proguidegh/api/internal/platform/audit"
	pauth "proguidegh/api/internal/platform/auth"
	"proguidegh/api/internal/platform/httpx"
	"proguidegh/api/internal/platform/ratelimit"
	"proguidegh/api/internal/platform/rbac"
)

// Rate-limit policies for the auth abuse vectors (spec §15.2), applied as
// middleware in cmd/api.
var (
	LoginLimit = ratelimit.Limit{Bucket: "login", Max: 10, Window: time.Minute}
	OTPLimit   = ratelimit.Limit{Bucket: "otp", Max: 5, Window: time.Minute}
	ResetLimit = ratelimit.Limit{Bucket: "pwd_reset", Max: 5, Window: time.Minute}
)

// Handler serves the /api/v1/auth endpoints.
type Handler struct {
	svc    *Service
	secure bool // cookie Secure flag (APP_ENV != local)
}

// NewHandler builds the auth handler.
func NewHandler(svc *Service, secure bool) *Handler {
	return &Handler{svc: svc, secure: secure}
}

// ---------------------------------------------------------------------------
// request/response types
// ---------------------------------------------------------------------------

type registerRequest struct {
	Intent   string  `json:"intent"`
	Email    string  `json:"email"`
	Phone    *string `json:"phone"`
	Password string  `json:"password"`
	FullName string  `json:"full_name"`
}

type userResponse struct {
	ID        string   `json:"id"`
	Email     string   `json:"email"`
	PhoneE164 *string  `json:"phone_e164"`
	Status    string   `json:"status"`
	Roles     []string `json:"roles"`
	CreatedAt string   `json:"created_at"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type mfaLoginRequest struct {
	Challenge string `json:"challenge"`
	Code      string `json:"code"`
}

type otpRequestRequest struct {
	Destination string `json:"destination"`
	Channel     string `json:"channel"`
	Purpose     string `json:"purpose"`
}

type otpVerifyRequest struct {
	Destination string `json:"destination"`
	Purpose     string `json:"purpose"`
	Code        string `json:"code"`
}

type forgotRequest struct {
	Email string `json:"email"`
}

type resetRequest struct {
	Email    string `json:"email"`
	Code     string `json:"code"`
	Password string `json:"password"`
}

type mfaVerifyRequest struct {
	Code string `json:"code"`
}

// ---------------------------------------------------------------------------
// endpoints
// ---------------------------------------------------------------------------

// Register handles POST /api/v1/auth/register.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if !decode(w, r, &req) {
		return
	}
	req.Intent = strings.ToLower(strings.TrimSpace(req.Intent))
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Intent != IntentTourist && req.Intent != IntentGuide {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "intent must be tourist or guide", nil)
		return
	}
	if !validEmail(req.Email) {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "a valid email is required", nil)
		return
	}
	if len(req.Password) < 8 {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "password must be at least 8 characters", nil)
		return
	}
	if req.Intent == IntentTourist && strings.TrimSpace(req.FullName) == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "full_name is required for tourist registration", nil)
		return
	}

	u, err := h.svc.Register(r.Context(), req.Intent, req.Email, req.Phone, req.Password, strings.TrimSpace(req.FullName))
	if errors.Is(err, ErrEmailTaken) {
		httpx.WriteError(w, r, http.StatusConflict, "EMAIL_TAKEN", "an account with this email already exists", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not create account", nil)
		return
	}

	roles, _ := h.svc.rbac.Roles(r.Context(), u.ID)
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"user": toUserResponse(u, roles)})
}

// Login handles POST /api/v1/auth/login. On MFA-enabled accounts it answers
// 200 with {"mfa_required": true, "challenge": "..."} and the client must
// call POST /api/v1/auth/login/mfa to complete the step-up.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !decode(w, r, &req) {
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" || req.Password == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "email and password are required", nil)
		return
	}

	tokens, challenge, err := h.svc.Login(r.Context(), req.Email, req.Password,
		audit.ClientIP(r), r.UserAgent())
	if errors.Is(err, ErrMFARequired) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"mfa_required": true,
			"challenge":    challenge,
		})
		return
	}
	if errors.Is(err, ErrInvalidCredentials) {
		httpx.WriteError(w, r, http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid email or password", nil)
		return
	}
	if errors.Is(err, ErrAccountSuspended) {
		httpx.WriteError(w, r, http.StatusForbidden, "ACCOUNT_SUSPENDED", "this account is not active", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "login failed", nil)
		return
	}
	h.writeSession(w, r, tokens)
}

// LoginMFA handles POST /api/v1/auth/login/mfa (step-up completion).
func (h *Handler) LoginMFA(w http.ResponseWriter, r *http.Request) {
	var req mfaLoginRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Challenge == "" || req.Code == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "challenge and code are required", nil)
		return
	}
	tokens, err := h.svc.CompleteMFALogin(r.Context(), req.Challenge, req.Code,
		audit.ClientIP(r), r.UserAgent())
	if errors.Is(err, ErrMFAInvalid) || errors.Is(err, ErrMFANotEnrolled) {
		httpx.WriteError(w, r, http.StatusUnauthorized, "MFA_INVALID", "invalid or expired mfa challenge/code", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "login failed", nil)
		return
	}
	h.writeSession(w, r, tokens)
}

// Refresh handles POST /api/v1/auth/refresh (rotating refresh tokens).
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	token := pauth.RefreshFromRequest(r)
	if token == "" {
		httpx.WriteError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "refresh token required", nil)
		return
	}
	tokens, err := h.svc.Refresh(r.Context(), token, audit.ClientIP(r), r.UserAgent())
	if errors.Is(err, ErrSessionReuse) {
		pauth.ClearSessionCookies(w, h.secure)
		httpx.WriteError(w, r, http.StatusUnauthorized, "SESSION_REUSE",
			"session was revoked; please sign in again", nil)
		return
	}
	if errors.Is(err, ErrSessionInvalid) {
		pauth.ClearSessionCookies(w, h.secure)
		httpx.WriteError(w, r, http.StatusUnauthorized, "SESSION_INVALID",
			"session expired or revoked; please sign in again", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not refresh session", nil)
		return
	}
	h.writeSession(w, r, tokens)
}

// Logout handles POST /api/v1/auth/logout.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	_ = h.svc.Logout(r.Context(), pauth.RefreshFromRequest(r))
	pauth.ClearSessionCookies(w, h.secure)
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// RequestOTP handles POST /api/v1/auth/otp/request.
func (h *Handler) RequestOTP(w http.ResponseWriter, r *http.Request) {
	var req otpRequestRequest
	if !decode(w, r, &req) {
		return
	}
	req.Destination = strings.TrimSpace(req.Destination)
	req.Channel = strings.ToLower(strings.TrimSpace(req.Channel))
	req.Purpose = strings.ToLower(strings.TrimSpace(req.Purpose))
	if req.Destination == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "destination is required", nil)
		return
	}
	if req.Channel != "sms" && req.Channel != "email" {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "channel must be sms or email", nil)
		return
	}
	switch req.Purpose {
	case "login", "verify_contact":
	default:
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "purpose must be login or verify_contact", nil)
		return
	}

	devCode, err := h.svc.RequestOTP(r.Context(), nil, req.Destination, req.Channel, req.Purpose)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not issue code", nil)
		return
	}
	resp := map[string]any{"status": "sent"}
	if devCode != "" {
		resp["dev_code"] = devCode // local env only
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// VerifyOTP handles POST /api/v1/auth/otp/verify.
func (h *Handler) VerifyOTP(w http.ResponseWriter, r *http.Request) {
	var req otpVerifyRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Destination == "" || req.Code == "" || req.Purpose == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "destination, purpose and code are required", nil)
		return
	}
	if err := h.svc.VerifyOTP(r.Context(), req.Destination, req.Purpose, req.Code); err != nil {
		writeOTPError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "verified"})
}

// ForgotPassword handles POST /api/v1/auth/password/forgot. The response is
// uniform whether or not the account exists.
func (h *Handler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req forgotRequest
	if !decode(w, r, &req) {
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if !validEmail(req.Email) {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "a valid email is required", nil)
		return
	}
	devCode, err := h.svc.ForgotPassword(r.Context(), req.Email)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not start reset", nil)
		return
	}
	resp := map[string]any{"status": "sent"}
	if devCode != "" {
		resp["dev_code"] = devCode // local env only
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// ResetPassword handles POST /api/v1/auth/password/reset.
func (h *Handler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req resetRequest
	if !decode(w, r, &req) {
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if !validEmail(req.Email) || len(req.Password) < 8 || req.Code == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION",
			"email, code and a password of at least 8 characters are required", nil)
		return
	}
	if err := h.svc.ResetPassword(r.Context(), req.Email, req.Code, req.Password); err != nil {
		writeOTPError(w, r, err)
		return
	}
	pauth.ClearSessionCookies(w, h.secure)
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// EnrollMFA handles POST /api/v1/me/mfa/enroll (auth required).
func (h *Handler) EnrollMFA(w http.ResponseWriter, r *http.Request) {
	id, _ := rbac.FromContext(r.Context())
	u, err := h.svc.repo.FindUserByID(r.Context(), id.UserID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load account", nil)
		return
	}
	enrollment, err := h.svc.EnrollMFA(r.Context(), id.UserID, u.Email)
	if errors.Is(err, ErrMFAAlreadyEnabled) {
		httpx.WriteError(w, r, http.StatusConflict, "MFA_ENABLED", "mfa is already enabled", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not start enrollment", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, enrollment)
}

// VerifyMFA handles POST /api/v1/me/mfa/verify (auth required). Returns the
// one-time backup codes.
func (h *Handler) VerifyMFA(w http.ResponseWriter, r *http.Request) {
	var req mfaVerifyRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Code == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "code is required", nil)
		return
	}
	id, _ := rbac.FromContext(r.Context())
	backupCodes, err := h.svc.VerifyMFA(r.Context(), id.UserID, req.Code, audit.ClientIP(r))
	if errors.Is(err, ErrMFAInvalid) {
		httpx.WriteError(w, r, http.StatusUnauthorized, "MFA_INVALID", "invalid code", nil)
		return
	}
	if errors.Is(err, ErrMFANotEnrolled) {
		httpx.WriteError(w, r, http.StatusConflict, "MFA_NOT_ENROLLED", "start enrollment first", nil)
		return
	}
	if errors.Is(err, ErrMFAAlreadyEnabled) {
		httpx.WriteError(w, r, http.StatusConflict, "MFA_ENABLED", "mfa is already enabled", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not enable mfa", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":       "enabled",
		"backup_codes": backupCodes,
	})
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func (h *Handler) writeSession(w http.ResponseWriter, r *http.Request, tokens Tokens) {
	pauth.SetSessionCookies(w, h.secure, tokens.AccessToken, tokens.RefreshToken)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
		"expires_at":    tokens.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	})
}

func writeOTPError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrOTPTooManyAttempts):
		httpx.WriteError(w, r, http.StatusTooManyRequests, "OTP_LOCKED", "too many attempts; request a new code", nil)
	case errors.Is(err, ErrOTPInvalid):
		httpx.WriteError(w, r, http.StatusUnauthorized, "OTP_INVALID", "invalid or expired code", nil)
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "account not found", nil)
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "operation failed", nil)
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "malformed JSON body", nil)
		return false
	}
	return true
}

func validEmail(s string) bool {
	_, err := mail.ParseAddress(s)
	return err == nil && strings.Contains(s, "@")
}

func toUserResponse(u User, roles []string) userResponse {
	return userResponse{
		ID:        u.ID,
		Email:     u.Email,
		PhoneE164: u.PhoneE164,
		Status:    u.Status,
		Roles:     roles,
		CreatedAt: u.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
}
