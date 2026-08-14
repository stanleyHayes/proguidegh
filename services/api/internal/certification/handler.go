package certification

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"proguidegh/api/internal/platform/audit"
	"proguidegh/api/internal/platform/httpx"
	"proguidegh/api/internal/platform/rbac"
)

// Handler serves the admin certification queue endpoints (spec §13.6). Every
// route is wrapped in RequireAuth + RequirePermission at the router.
type Handler struct {
	repo          *Repository
	service       *Service
	onGuideActive func(userID string) // rbac cache invalidation hook
}

// NewHandler builds the handler. onGuideActive invalidates the permission
// cache when a transition grants the guide role; it may be nil.
func NewHandler(repo *Repository, svc *Service, onGuideActive func(userID string)) *Handler {
	return &Handler{repo: repo, service: svc, onGuideActive: onGuideActive}
}

// Queue handles GET /api/v1/admin/certification/queue (certification.read).
// Optional ?status= filter against the state machine statuses.
func (h *Handler) Queue(w http.ResponseWriter, r *http.Request) {
	limit, offset := pageParams(r)
	status := NormalizeStatus(r.URL.Query().Get("status"))
	if status != "" && !ValidStatus(status) {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "invalid status filter",
			map[string]any{"valid_statuses": Statuses()})
		return
	}
	rows, total, err := h.repo.ListQueue(r.Context(), status, limit, offset)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not list queue", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"cases":  rows,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// CaseDetail handles GET /api/v1/admin/certification/{caseId}
// (certification.read): the case, the guide's documents and the immutable
// event history.
func (h *Handler) CaseDetail(w http.ResponseWriter, r *http.Request) {
	caseID := r.PathValue("caseId")
	c, err := h.repo.GetCase(r.Context(), caseID)
	if errors.Is(err, ErrCaseNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "certification case not found", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load case", nil)
		return
	}
	docs, err := h.repo.ListDocuments(r.Context(), c.GuideID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load documents", nil)
		return
	}
	events, err := h.repo.ListEvents(r.Context(), c.ID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load events", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"case":      c,
		"documents": docs,
		"events":    events,
	})
}

type transitionRequest struct {
	ToStatus    string `json:"to_status"`
	Reason      string `json:"reason"`
	EvidenceRef string `json:"evidence_ref"`
}

// Transition handles POST /api/v1/admin/certification/{caseId}/transition
// (certification.review). The state machine validates the move; evidence
// stages (spec §5) require an evidence_ref plus a valid document.
func (h *Handler) Transition(w http.ResponseWriter, r *http.Request) {
	caseID := r.PathValue("caseId")
	var req transitionRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "malformed JSON body", nil)
		return
	}
	if strings.TrimSpace(req.ToStatus) == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "to_status is required", nil)
		return
	}

	actor, _ := rbac.FromContext(r.Context())
	res, err := h.service.Transition(r.Context(), TransitionParams{
		CaseID:      caseID,
		ActorID:     actor.UserID,
		ToStatus:    req.ToStatus,
		Reason:      req.Reason,
		EvidenceRef: req.EvidenceRef,
		IP:          audit.ClientIP(r),
	})
	switch {
	case errors.Is(err, ErrUnknownStatus):
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "unknown to_status",
			map[string]any{"valid_statuses": Statuses()})
		return
	case errors.Is(err, ErrReasonRequired):
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "reason is required", nil)
		return
	case errors.Is(err, ErrCaseNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "certification case not found", nil)
		return
	case errors.Is(err, ErrIllegalTransition):
		httpx.WriteError(w, r, http.StatusConflict, "ILLEGAL_TRANSITION", err.Error(), nil)
		return
	case errors.Is(err, ErrEvidenceRequired):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "EVIDENCE_REQUIRED",
			"this stage requires an evidence_ref and a valid, unexpired document (spec §5)", nil)
		return
	case err != nil:
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not apply transition", nil)
		return
	}

	if res.Case.Status == StatusActive && h.onGuideActive != nil {
		h.onGuideActive(res.Case.GuideID)
	}
	httpx.WriteJSON(w, http.StatusOK, res)
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
