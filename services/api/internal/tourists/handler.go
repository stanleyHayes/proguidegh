package tourists

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"proguidegh/api/internal/platform/httpx"
	"proguidegh/api/internal/platform/rbac"
)

// Handler serves /api/v1/me/tourist-profile (auth required, self-scoped).
type Handler struct {
	repo *Repository
}

// NewHandler builds the handler.
func NewHandler(repo *Repository) *Handler { return &Handler{repo: repo} }

type patchRequest struct {
	FullName                  *string `json:"full_name"`
	Nationality               *string `json:"nationality"`
	PreferredLanguage         *string `json:"preferred_language"`
	EmergencyContactName      *string `json:"emergency_contact_name"`
	EmergencyContactPhoneE164 *string `json:"emergency_contact_phone_e164"`
}

// Get handles GET /api/v1/me/tourist-profile.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, _ := rbac.FromContext(r.Context())
	p, err := h.repo.Get(r.Context(), id.UserID)
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "tourist profile not found", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load profile", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"profile": p})
}

// Patch handles PATCH /api/v1/me/tourist-profile.
func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	var req patchRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "malformed JSON body", nil)
		return
	}
	if req.FullName != nil && strings.TrimSpace(*req.FullName) == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "full_name cannot be empty", nil)
		return
	}

	id, _ := rbac.FromContext(r.Context())
	p, err := h.repo.Patch(r.Context(), id.UserID, PatchInput{
		FullName:                  req.FullName,
		Nationality:               req.Nationality,
		PreferredLanguage:         req.PreferredLanguage,
		EmergencyContactName:      req.EmergencyContactName,
		EmergencyContactPhoneE164: req.EmergencyContactPhoneE164,
	})
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "tourist profile not found", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not update profile", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"profile": p})
}
