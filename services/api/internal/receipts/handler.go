package receipts

import (
	"errors"
	"net/http"
	"regexp"

	"proguidegh/api/internal/bookings"
	"proguidegh/api/internal/platform/httpx"
	"proguidegh/api/internal/platform/rbac"
)

// Handler serves the receipt endpoint (spec §13.3).
type Handler struct {
	svc      *Service
	bookings *bookings.Repository
}

// NewHandler builds the handler.
func NewHandler(svc *Service, bookingsRepo *bookings.Repository) *Handler {
	return &Handler{svc: svc, bookings: bookingsRepo}
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// Get handles GET /api/v1/bookings/{id}/receipt (auth required). Visible to
// the owning tourist, the assigned guide, or bookings.read holders — same
// scoping as the booking detail, 404 for anyone else. Returns the receipt
// metadata plus a short-lived signed download URL (spec §17; files are never
// public, stop condition 8).
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	bookingID := r.PathValue("id")
	if !uuidRe.MatchString(bookingID) {
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "receipt not found", nil)
		return
	}
	b, err := h.bookings.GetByID(r.Context(), bookingID)
	if errors.Is(err, bookings.ErrNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "receipt not found", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load booking", nil)
		return
	}

	id, _ := rbac.FromContext(r.Context())
	owner := b.TouristID == id.UserID
	guide := b.GuideID != nil && *b.GuideID == id.UserID
	if !owner && !guide && !id.Has("bookings.read") {
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "receipt not found", nil)
		return
	}

	rec, url, err := h.svc.Download(r.Context(), bookingID)
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "no receipt issued for this booking yet", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load receipt", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"receipt":      rec,
		"download_url": url,
		"expires_in":   900,
	})
}
