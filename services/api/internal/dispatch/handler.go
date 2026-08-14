package dispatch

import (
	"errors"
	"net/http"

	"proguidegh/api/internal/bookings"
	"proguidegh/api/internal/platform/audit"
	"proguidegh/api/internal/platform/httpx"
	"proguidegh/api/internal/platform/rbac"
)

// Handler serves the dispatch REST endpoints (spec §13.4, §13.6). The WS
// feed is realtime-only sugar: every offer here is the same row the hub
// pushes, so a disconnected guide catches up via GET /me/guide/offers.
type Handler struct {
	svc     *Service
	auditor *audit.Recorder
}

// NewHandler builds the handler.
func NewHandler(svc *Service, auditor *audit.Recorder) *Handler {
	return &Handler{svc: svc, auditor: auditor}
}

// ListMine handles GET /api/v1/me/guide/offers — the caller's current,
// unexpired offers (auth required; the caller's user id is the guide id).
func (h *Handler) ListMine(w http.ResponseWriter, r *http.Request) {
	id, _ := rbac.FromContext(r.Context())
	offers, err := h.svc.ListMine(r.Context(), id.UserID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not list offers", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"offers": offers})
}

// Accept handles POST /api/v1/offers/{id}/accept — atomic first-wins
// acceptance (§10.3 step 4, §30.2).
func (h *Handler) Accept(w http.ResponseWriter, r *http.Request) {
	id, _ := rbac.FromContext(r.Context())
	res, err := h.svc.Accept(r.Context(), r.PathValue("id"), id.UserID)
	switch {
	case errors.Is(err, ErrOfferNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "offer not found", nil)
		return
	case errors.Is(err, ErrOfferExpired):
		httpx.WriteError(w, r, http.StatusGone, "OFFER_EXPIRED", "offer has expired", nil)
		return
	case errors.Is(err, ErrOfferClosed):
		httpx.WriteError(w, r, http.StatusConflict, "OFFER_CLOSED", err.Error(), nil)
		return
	case errors.Is(err, bookings.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "booking not found", nil)
		return
	case errors.Is(err, bookings.ErrOverlap):
		httpx.WriteError(w, r, http.StatusConflict, "BOOKING_OVERLAP",
			"guide already holds an active booking for this time", nil)
		return
	case err != nil:
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not accept offer", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, res)
}

// Decline handles POST /api/v1/offers/{id}/decline.
func (h *Handler) Decline(w http.ResponseWriter, r *http.Request) {
	id, _ := rbac.FromContext(r.Context())
	offer, err := h.svc.Decline(r.Context(), r.PathValue("id"), id.UserID)
	switch {
	case errors.Is(err, ErrOfferNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "offer not found", nil)
		return
	case errors.Is(err, ErrOfferExpired):
		httpx.WriteError(w, r, http.StatusGone, "OFFER_EXPIRED", "offer has expired", nil)
		return
	case errors.Is(err, ErrOfferClosed):
		httpx.WriteError(w, r, http.StatusConflict, "OFFER_CLOSED", err.Error(), nil)
		return
	case err != nil:
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not decline offer", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"offer": offer})
}

// Dispatch handles POST /api/v1/admin/bookings/{id}/dispatch — manual
// (re)dispatch of a confirmed, guideless booking (permission dispatch.manage;
// audited, spec §1.2). Returns the live batch when one exists.
func (h *Handler) Dispatch(w http.ResponseWriter, r *http.Request) {
	id, _ := rbac.FromContext(r.Context())
	bookingID := r.PathValue("id")
	res, err := h.svc.Dispatch(r.Context(), bookingID, "admin.manual")
	switch {
	case errors.Is(err, bookings.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "booking not found", nil)
		return
	case errors.Is(err, ErrNotDispatchable):
		httpx.WriteError(w, r, http.StatusConflict, "NOT_DISPATCHABLE", err.Error(), nil)
		return
	case err != nil:
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not dispatch booking", nil)
		return
	}
	if err := h.auditor.RecordHTTP(r.Context(), r, audit.Entry{
		ActorID:    id.UserID,
		Action:     "admin.bookings.dispatch",
		EntityType: "booking",
		EntityID:   bookingID,
		After: map[string]any{
			"batch_seq": res.BatchSeq, "offers": len(res.Offers),
			"candidates": res.Candidates, "reused_live": res.ReusedLive,
		},
	}); err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not audit dispatch", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"dispatch": res})
}

// BookingOffers handles GET /api/v1/admin/bookings/{id}/dispatch — the
// operations "why has this booking not been matched" view (§30.2): every
// offer batch with outcomes, newest first (permission dispatch.manage).
func (h *Handler) BookingOffers(w http.ResponseWriter, r *http.Request) {
	bookingID := r.PathValue("id")
	b, err := h.svc.bookings.GetByID(r.Context(), bookingID)
	if errors.Is(err, bookings.ErrNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "booking not found", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load booking", nil)
		return
	}
	offers, err := h.svc.BookingOffers(r.Context(), bookingID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not list offers", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"booking": map[string]any{
			"id": b.ID, "reference": b.Reference, "status": b.Status,
			"guide_id": b.GuideID, "starts_at": b.StartsAt,
		},
		"offers": offers,
	})
}
