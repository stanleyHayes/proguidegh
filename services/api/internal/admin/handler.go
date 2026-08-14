package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"proguidegh/api/internal/bookings"
	"proguidegh/api/internal/platform/audit"
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
	for i := range req.Roles {
		req.Roles[i] = strings.ToLower(strings.TrimSpace(req.Roles[i]))
	}

	before, err := h.repo.GetRoles(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load roles", nil)
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

	actor, _ := rbac.FromContext(r.Context())
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
