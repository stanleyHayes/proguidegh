package tracking

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"proguidegh/api/internal/bookings"
	"proguidegh/api/internal/platform/httpx"
	"proguidegh/api/internal/platform/rbac"
)

// Handler serves the location endpoints (spec §13.5): POST is the guide's
// HTTPS update path (the WS channel carries the same fan-out to readers);
// GET is the realtime-degraded read for tourist/operations (§31.27).
type Handler struct {
	svc      *Service
	bookings *bookings.Repository
}

// NewHandler builds the handler.
func NewHandler(svc *Service, bookingsRepo *bookings.Repository) *Handler {
	return &Handler{svc: svc, bookings: bookingsRepo}
}

type locationRequest struct {
	Latitude   *float64   `json:"latitude"`
	Longitude  *float64   `json:"longitude"`
	AccuracyM  *float64   `json:"accuracy_m"`
	Heading    *float64   `json:"heading"`
	SpeedMps   *float64   `json:"speed_mps"`
	CapturedAt *time.Time `json:"captured_at"`
}

// Post handles POST /api/v1/bookings/{id}/location (auth required, assigned
// guide only, booking in the active window). The booking_id in the §11.1
// payload is the path parameter — body booking ids are not accepted.
func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	var req locationRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "malformed JSON body", nil)
		return
	}
	if req.Latitude == nil || req.Longitude == nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "latitude and longitude are required", nil)
		return
	}

	p := Position{
		Latitude:  *req.Latitude,
		Longitude: *req.Longitude,
		AccuracyM: req.AccuracyM,
		Heading:   req.Heading,
		SpeedMps:  req.SpeedMps,
	}
	if req.CapturedAt != nil {
		p.CapturedAt = *req.CapturedAt
	}

	id, _ := rbac.FromContext(r.Context())
	out, err := h.svc.Ingest(r.Context(), r.PathValue("id"), id.UserID, p)
	switch {
	case errors.Is(err, bookings.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "booking not found", nil)
		return
	case errors.Is(err, ErrForbidden):
		httpx.WriteError(w, r, http.StatusForbidden, "FORBIDDEN", "only the assigned guide may report location", nil)
		return
	case errors.Is(err, ErrInactive):
		httpx.WriteError(w, r, http.StatusConflict, "BOOKING_INACTIVE", err.Error(), nil)
		return
	case errors.Is(err, ErrValidation):
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
		return
	case err != nil:
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not record location", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"location": out, "ttl_seconds": int(LocationTTL.Seconds())})
}

// Get handles GET /api/v1/bookings/{id}/location — the current cached
// position. Readers: the owning tourist (active window only), the assigned
// guide, or dispatch.manage (operations live map, §11.2). Everyone else gets
// 404; outside the active window there is nothing to read (also 404 — no
// historical movement is exposed).
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	bookingID := r.PathValue("id")
	b, err := h.bookings.GetByID(r.Context(), bookingID)
	if errors.Is(err, bookings.ErrNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "booking not found", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load booking", nil)
		return
	}

	id, _ := rbac.FromContext(r.Context())
	owner := b.TouristID == id.UserID
	guide := b.GuideID != nil && *b.GuideID == id.UserID
	ops := id.Has("dispatch.manage")
	if !owner && !guide && !ops {
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "booking not found", nil)
		return
	}
	if !Active(b.Status) {
		httpx.WriteError(w, r, http.StatusNotFound, "NO_LOCATION",
			"location is only available while the tour is active (spec §11.2)", nil)
		return
	}

	p, err := h.svc.CurrentForBooking(r.Context(), bookingID)
	if errors.Is(err, ErrNoLocation) {
		httpx.WriteError(w, r, http.StatusNotFound, "NO_LOCATION", "no current position reported", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not read location", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"location": p})
}
