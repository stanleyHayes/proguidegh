package safety

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"

	"proguidegh/api/internal/platform/httpx"
	"proguidegh/api/internal/platform/rbac"
)

// Handler serves POST /api/v1/bookings/{id}/sos (spec §13.5).
type Handler struct {
	svc *Service
}

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type sosRequest struct {
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
	AccuracyM *float64 `json:"accuracy_m"`
}

// Trigger handles POST /api/v1/bookings/{id}/sos (auth required, booking
// participant only). The response names ProGuideGH operations as the
// responder — never police or emergency services (§12 safety requirement).
func (h *Handler) Trigger(w http.ResponseWriter, r *http.Request) {
	bookingID := r.PathValue("id")
	if !uuidRe.MatchString(bookingID) {
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "booking not found", nil)
		return
	}
	var req sosRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "malformed JSON body", nil)
		return
	}
	if req.Latitude == nil || req.Longitude == nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION",
			"latitude and longitude are required — send the freshest fix you have (§12 step 7)", nil)
		return
	}

	id, _ := rbac.FromContext(r.Context())
	ev, inc, err := h.svc.Trigger(r.Context(), bookingID, id.UserID, *req.Latitude, *req.Longitude, req.AccuracyM)
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrForbidden):
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "booking not found", nil)
		return
	case errors.Is(err, ErrInactive):
		httpx.WriteError(w, r, http.StatusConflict, "BOOKING_INACTIVE",
			"SOS is only available on an active booking", nil)
		return
	case errors.Is(err, ErrRateLimited):
		w.Header().Set("Retry-After", strconv.Itoa(int(SOSLimit.Window.Seconds())))
		httpx.WriteError(w, r, http.StatusTooManyRequests, "RATE_LIMITED",
			"too many SOS triggers — your earlier alerts are already with operations", nil)
		return
	case errors.Is(err, ErrValidation):
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
		return
	case err != nil:
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not raise the SOS", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"sos_event": ev,
		"incident":  inc,
		"message":   "Your SOS has been sent to ProGuideGH operations with your location. Operations will respond; this is not a police or emergency-services dispatch.",
	})
}
