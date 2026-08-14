package incidents

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"proguidegh/api/internal/platform/httpx"
	"proguidegh/api/internal/platform/rbac"
)

// Handler serves the admin incident workflow and the quality-flag queue
// (spec §13.6). Permission scoping is applied at the router
// (incidents.read / incidents.manage / reviews.moderate).
type Handler struct {
	svc *Service
}

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func actorID(r *http.Request) string {
	id, _ := rbac.FromContext(r.Context())
	return id.UserID
}

// List handles GET /api/v1/admin/incidents.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	incidents, total, err := h.svc.List(r.Context(), ListFilter{
		Status:   strings.TrimSpace(q.Get("status")),
		Severity: strings.TrimSpace(q.Get("severity")),
		Type:     strings.TrimSpace(q.Get("type")),
	}, limit, offset)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not list incidents", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"incidents": incidents, "total": total})
}

// Detail handles GET /api/v1/admin/incidents/{id} — incident plus the full
// timestamped trail (§12 step 11).
func (h *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	inc, events, err := h.svc.Detail(r.Context(), r.PathValue("id"))
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "incident not found", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load incident", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"incident": inc, "events": events})
}

type noteRequest struct {
	Body string `json:"body"`
}

type assignRequest struct {
	UserID string `json:"user_id"`
}

type resolveRequest struct {
	Note string `json:"note"`
}

func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "malformed JSON body", nil)
		return false
	}
	return true
}

func (h *Handler) writeOutcome(w http.ResponseWriter, r *http.Request, inc Incident, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "incident not found", nil)
	case errors.Is(err, ErrIllegalTransition):
		httpx.WriteError(w, r, http.StatusConflict, "ILLEGAL_TRANSITION", err.Error(), nil)
	case errors.Is(err, ErrValidation):
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
	case err != nil:
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not update the incident", nil)
	default:
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"incident": inc})
	}
}

// Acknowledge handles POST /api/v1/admin/incidents/{id}/acknowledge.
func (h *Handler) Acknowledge(w http.ResponseWriter, r *http.Request) {
	inc, err := h.svc.Acknowledge(r.Context(), r.PathValue("id"), actorID(r))
	h.writeOutcome(w, r, inc, err)
}

// Note handles POST /api/v1/admin/incidents/{id}/notes.
func (h *Handler) Note(w http.ResponseWriter, r *http.Request) {
	var req noteRequest
	if !decodeBody(w, r, &req) {
		return
	}
	inc, err := h.svc.AddNote(r.Context(), r.PathValue("id"), actorID(r), req.Body)
	h.writeOutcome(w, r, inc, err)
}

// Escalate handles POST /api/v1/admin/incidents/{id}/escalate.
func (h *Handler) Escalate(w http.ResponseWriter, r *http.Request) {
	inc, err := h.svc.Escalate(r.Context(), r.PathValue("id"), actorID(r))
	h.writeOutcome(w, r, inc, err)
}

// Assign handles POST /api/v1/admin/incidents/{id}/assign.
func (h *Handler) Assign(w http.ResponseWriter, r *http.Request) {
	var req assignRequest
	if !decodeBody(w, r, &req) {
		return
	}
	inc, err := h.svc.Assign(r.Context(), r.PathValue("id"), actorID(r), req.UserID)
	h.writeOutcome(w, r, inc, err)
}

// Resolve handles POST /api/v1/admin/incidents/{id}/resolve.
func (h *Handler) Resolve(w http.ResponseWriter, r *http.Request) {
	var req resolveRequest
	if !decodeBody(w, r, &req) {
		return
	}
	inc, err := h.svc.Resolve(r.Context(), r.PathValue("id"), actorID(r), req.Note)
	h.writeOutcome(w, r, inc, err)
}

// Close handles POST /api/v1/admin/incidents/{id}/close.
func (h *Handler) Close(w http.ResponseWriter, r *http.Request) {
	var req resolveRequest
	if !decodeBody(w, r, &req) {
		return
	}
	var note *string
	if strings.TrimSpace(req.Note) != "" {
		note = &req.Note
	}
	inc, err := h.svc.Close(r.Context(), r.PathValue("id"), actorID(r), note)
	h.writeOutcome(w, r, inc, err)
}

// ListFlags handles GET /api/v1/admin/quality-flags — the quality/retraining
// queue (spec §4.4, P6-06).
func (h *Handler) ListFlags(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	flags, err := h.svc.ListFlags(r.Context(), strings.TrimSpace(q.Get("status")), limit, offset)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not list quality flags", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"flags": flags})
}

// ResolveFlag handles POST /api/v1/admin/quality-flags/{id}/resolve.
func (h *Handler) ResolveFlag(w http.ResponseWriter, r *http.Request) {
	if !uuidRe.MatchString(r.PathValue("id")) {
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "quality flag not found", nil)
		return
	}
	var req resolveRequest
	if !decodeBody(w, r, &req) {
		return
	}
	flag, err := h.svc.ResolveFlag(r.Context(), r.PathValue("id"), actorID(r), req.Note)
	switch {
	case errors.Is(err, ErrAlreadyResolved):
		httpx.WriteError(w, r, http.StatusConflict, "ALREADY_RESOLVED", "flag is already resolved or does not exist", nil)
	case errors.Is(err, ErrValidation):
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
	case err != nil:
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not resolve the flag", nil)
	default:
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"flag": flag})
	}
}
