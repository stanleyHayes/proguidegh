package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"proguidegh/api/internal/bookings"
	"proguidegh/api/internal/platform/audit"
	pauth "proguidegh/api/internal/platform/auth"
	"proguidegh/api/internal/platform/httpx"
	"proguidegh/api/internal/platform/rbac"
)

// Handler serves the Phase 1 admin endpoints. Every route is wrapped in
// RequireAuth + RequirePermission at the router (spec §3 RBAC rule).
type Handler struct {
	repo         *Repository
	audit        *audit.Recorder
	onRoleChange func(userID string) // rbac cache invalidation hook
}

// NewHandler builds the handler. onRoleChange invalidates the permission
// cache; it may be nil.
func NewHandler(repo *Repository, aud *audit.Recorder, onRoleChange func(userID string)) *Handler {
	return &Handler{repo: repo, audit: aud, onRoleChange: onRoleChange}
}

var staffRoles = map[string]bool{"operations_agent": true, "verifier": true, "finance_officer": true, "content_admin": true, "administrator": true, "super_admin": true}

type invitationRequest struct {
	Email string   `json:"email"`
	Roles []string `json:"roles"`
}
type acceptInvitationRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

func (h *Handler) CreateInvitation(w http.ResponseWriter, r *http.Request) {
	var req invitationRequest
	if !decodeAdmin(w, r, &req) {
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if addr, err := mail.ParseAddress(req.Email); err != nil || addr.Address != req.Email {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "a valid email is required", nil)
		return
	}
	if len(req.Roles) == 0 {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "select at least one staff role", nil)
		return
	}
	seen := map[string]bool{}
	roles := make([]string, 0, len(req.Roles))
	for _, role := range req.Roles {
		role = strings.ToLower(strings.TrimSpace(role))
		if !staffRoles[role] {
			httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "invitations can only grant staff roles", nil)
			return
		}
		if !seen[role] {
			seen[role] = true
			roles = append(roles, role)
		}
	}
	actor, _ := rbac.FromContext(r.Context())
	token, err := pauth.NewOpaqueToken()
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not create invitation", nil)
		return
	}
	inv, err := h.repo.CreateInvitation(r.Context(), req.Email, roles, pauth.HashToken(token), actor.UserID, time.Now().Add(72*time.Hour))
	if errors.Is(err, ErrInvitationExists) {
		httpx.WriteError(w, r, http.StatusConflict, "INVITATION_EXISTS", "a pending invitation already exists for this email", nil)
		return
	}
	if errors.Is(err, ErrEmailInUse) {
		httpx.WriteError(w, r, http.StatusConflict, "EMAIL_TAKEN", "this email already belongs to an account", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not create invitation", nil)
		return
	}
	if err := h.audit.RecordHTTP(r.Context(), r, audit.Entry{ActorID: actor.UserID, Action: "admin.invitations.create", EntityType: "admin_invitation", EntityID: inv.ID, After: map[string]any{"email": inv.Email, "roles": inv.Roles, "expires_at": inv.ExpiresAt}}); err != nil {
		_ = h.repo.DeleteInvitation(r.Context(), inv.ID)
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not write audit record", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"invitation": inv, "accept_token": token})
}

func (h *Handler) ListInvitations(w http.ResponseWriter, r *http.Request) {
	invitations, err := h.repo.ListInvitations(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not list invitations", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"invitations": invitations})
}

func (h *Handler) AcceptInvitation(w http.ResponseWriter, r *http.Request) {
	var req acceptInvitationRequest
	if !decodeAdmin(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Token) == "" || len(req.Password) < 8 {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "a valid invitation and password of at least 8 characters are required", nil)
		return
	}
	hash, err := pauth.HashPassword(req.Password)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not accept invitation", nil)
		return
	}
	u, roles, err := h.repo.AcceptInvitation(r.Context(), pauth.HashToken(req.Token), hash)
	if errors.Is(err, ErrInvitationInvalid) {
		httpx.WriteError(w, r, http.StatusGone, "INVITATION_INVALID", "this invitation is invalid, expired, or has already been used", nil)
		return
	}
	if errors.Is(err, ErrEmailInUse) {
		httpx.WriteError(w, r, http.StatusConflict, "EMAIL_TAKEN", "an account already exists for this email", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not accept invitation", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"user": map[string]any{"id": u.ID, "email": u.Email, "roles": roles}})
}

func decodeAdmin(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "malformed JSON body", nil)
		return false
	}
	return true
}

// ListUsers handles GET /api/v1/admin/users (users.read).
func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	limit, offset := pageParams(r)
	users, total, err := h.repo.ListUsers(r.Context(), limit, offset)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not list users", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"users":  users,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

type setRolesRequest struct {
	Roles []string `json:"roles"`
}

// SetUserRoles handles PATCH /api/v1/admin/users/{id}/roles (users.manage).
// The change is audited with before/after role sets (spec §1.2), the
// permission cache is invalidated, and when roles were removed the user's
// sessions are revoked (spec §15.2).
func (h *Handler) SetUserRoles(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	var req setRolesRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "malformed JSON body", nil)
		return
	}
	if req.Roles == nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "roles is required", nil)
		return
	}
	if len(req.Roles) == 0 {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "at least one role is required", nil)
		return
	}
	for i := range req.Roles {
		req.Roles[i] = strings.ToLower(strings.TrimSpace(req.Roles[i]))
	}

	before, err := h.repo.GetRoles(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load roles", nil)
		return
	}
	actor, _ := rbac.FromContext(r.Context())
	if actor.UserID == userID && containsRole(before, "super_admin") && !containsRole(req.Roles, "super_admin") {
		httpx.WriteError(w, r, http.StatusBadRequest, "SELF_LOCKOUT", "you cannot remove your own super admin role", nil)
		return
	}
	after, err := h.repo.SetRoles(r.Context(), userID, req.Roles)
	if errors.Is(err, ErrUserNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "user not found", nil)
		return
	}
	if errors.Is(err, ErrUnknownRole) {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "unknown role code", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not update roles", nil)
		return
	}

	if h.onRoleChange != nil {
		h.onRoleChange(userID)
	}
	// Revoke sessions when the role set shrank (spec §15.2).
	if removedRole(before, after) {
		if err := h.repo.RevokeSessionsForUser(r.Context(), userID); err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not revoke sessions", nil)
			return
		}
	}

	if err := h.audit.RecordHTTP(r.Context(), r, audit.Entry{
		ActorID:    actor.UserID,
		Action:     "admin.users.roles.update",
		EntityType: "user",
		EntityID:   userID,
		Before:     map[string]any{"roles": before},
		After:      map[string]any{"roles": after},
	}); err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not write audit record", nil)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"user_id": userID, "roles": after})
}

func containsRole(roles []string, target string) bool {
	for _, role := range roles {
		if role == target {
			return true
		}
	}
	return false
}

// ListGuides handles GET /api/v1/admin/guides (guides.read). Optional
// ?status= filter against the guide_profiles status set.
func (h *Handler) ListGuides(w http.ResponseWriter, r *http.Request) {
	limit, offset := pageParams(r)
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status != "" {
		valid := map[string]bool{"pending": true, "in_review": true, "certified": true, "suspended": true, "disabled": true}
		if !valid[status] {
			httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "invalid status filter", nil)
			return
		}
	}
	guides, total, err := h.repo.ListGuides(r.Context(), status, limit, offset)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not list guides", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"guides": guides,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// ListBookings handles GET /api/v1/admin/bookings (bookings.read). The
// ?status= filter accepts "active" (CONFIRMED/GUIDE_EN_ROUTE/GUIDE_ARRIVED/
// IN_PROGRESS — the operations board default), a single §8.2 status, or a
// comma-separated list of statuses. Offset pagination.
func (h *Handler) ListBookings(w http.ResponseWriter, r *http.Request) {
	limit, offset := pageParams(r)
	statuses, err := parseBookingStatusFilter(r.URL.Query().Get("status"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
		return
	}
	bookings, total, err := h.repo.ListBookings(r.Context(), statuses, limit, offset)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not list bookings", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"bookings": bookings,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
	})
}

// parseBookingStatusFilter maps the ?status= query value to a status set:
// empty → nil (no filter), "active" → the on-calendar statuses, otherwise a
// comma-separated list of §8.2 status codes (validated, case-insensitive).
func parseBookingStatusFilter(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if strings.EqualFold(raw, "active") {
		return bookings.ActiveStatuses, nil
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		s := strings.ToUpper(strings.TrimSpace(part))
		if !bookings.ValidStatus(s) {
			return nil, fmt.Errorf("invalid status filter %q", part)
		}
		out = append(out, s)
	}
	return out, nil
}

func pageParams(r *http.Request) (limit, offset int) {
	limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ = strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func removedRole(before, after []string) bool {
	keep := map[string]bool{}
	for _, r := range after {
		keep[r] = true
	}
	for _, r := range before {
		if !keep[r] {
			return true
		}
	}
	return false
}
