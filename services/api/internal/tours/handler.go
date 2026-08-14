package tours

import (
	"encoding/json"
	"errors"
	"net/http"

	"proguidegh/api/internal/bookings"
	"proguidegh/api/internal/platform/audit"
	"proguidegh/api/internal/platform/httpx"
	"proguidegh/api/internal/platform/rbac"
)

// Handler serves the tour-operations endpoints (spec §13.4, §13.6).
type Handler struct {
	svc     *Service
	auditor *audit.Recorder
}

// NewHandler builds the handler.
func NewHandler(svc *Service, auditor *audit.Recorder) *Handler {
	return &Handler{svc: svc, auditor: auditor}
}

// EnRoute handles POST /api/v1/bookings/{id}/en-route (assigned guide).
func (h *Handler) EnRoute(w http.ResponseWriter, r *http.Request) {
	h.guideStep(w, r, StepEnRoute)
}

// Arrived handles POST /api/v1/bookings/{id}/arrived (assigned guide).
func (h *Handler) Arrived(w http.ResponseWriter, r *http.Request) {
	h.guideStep(w, r, StepArrived)
}

// Start handles POST /api/v1/bookings/{id}/start (assigned guide).
func (h *Handler) Start(w http.ResponseWriter, r *http.Request) {
	h.guideStep(w, r, StepStart)
}

// Complete handles POST /api/v1/bookings/{id}/complete (assigned guide);
// sets ends_at and moves the guide payable pending -> eligible (§9.2).
func (h *Handler) Complete(w http.ResponseWriter, r *http.Request) {
	h.guideStep(w, r, StepComplete)
}

func (h *Handler) guideStep(w http.ResponseWriter, r *http.Request, st step) {
	id, _ := rbac.FromContext(r.Context())
	bookingID := r.PathValue("id")

	var b bookings.Booking
	var err error
	if st == StepComplete {
		b, err = h.svc.Complete(r.Context(), bookingID, id.UserID)
	} else {
		b, err = h.svc.GuideStep(r.Context(), bookingID, id.UserID, st)
	}
	switch {
	case errors.Is(err, bookings.ErrNotFound), errors.Is(err, ErrNotAssigned):
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "booking not found", nil)
		return
	case errors.Is(err, bookings.ErrIllegalTransition):
		httpx.WriteError(w, r, http.StatusConflict, "ILLEGAL_TRANSITION", err.Error(), nil)
		return
	case errors.Is(err, bookings.ErrOverlap):
		httpx.WriteError(w, r, http.StatusConflict, "BOOKING_OVERLAP",
			"guide already holds an active booking for this time", nil)
		return
	case err != nil:
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not transition booking", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"booking": b})
}

type adminTransitionRequest struct {
	ToStatus string `json:"to_status"`
	Reason   string `json:"reason"`
}

// AdminTransition handles POST /api/v1/admin/bookings/{id}/transition
// (permission dispatch.manage; reason required; audited, spec §1.2).
func (h *Handler) AdminTransition(w http.ResponseWriter, r *http.Request) {
	var req adminTransitionRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "malformed JSON body", nil)
		return
	}

	id, _ := rbac.FromContext(r.Context())
	bookingID := r.PathValue("id")

	before, loadErr := h.svc.bookings.GetByID(r.Context(), bookingID)
	b, e, err := h.svc.AdminTransition(r.Context(), bookingID, id.UserID, req.ToStatus, req.Reason)
	switch {
	case errors.Is(err, ErrReasonRequired):
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "reason is required", nil)
		return
	case errors.Is(err, bookings.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "booking not found", nil)
		return
	case errors.Is(err, bookings.ErrIllegalTransition):
		httpx.WriteError(w, r, http.StatusConflict, "ILLEGAL_TRANSITION", err.Error(), nil)
		return
	case errors.Is(err, bookings.ErrOverlap):
		httpx.WriteError(w, r, http.StatusConflict, "BOOKING_OVERLAP",
			"guide already holds an active booking for this time", nil)
		return
	case err != nil:
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not transition booking", nil)
		return
	}

	entry := audit.Entry{
		ActorID:    id.UserID,
		Action:     "admin.bookings.transition",
		EntityType: "booking",
		EntityID:   bookingID,
		After:      map[string]any{"to_status": b.Status, "reason": req.Reason, "event_id": e.ID},
	}
	if loadErr == nil {
		entry.Before = map[string]any{"status": before.Status, "guide_id": before.GuideID}
	}
	if err := h.auditor.RecordHTTP(r.Context(), r, entry); err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not audit transition", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"booking": b, "event": e})
}
