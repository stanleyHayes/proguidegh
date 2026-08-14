package reviews

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"

	"proguidegh/api/internal/platform/httpx"
	"proguidegh/api/internal/platform/rbac"
)

// Handler serves the review endpoints (spec §13.5).
type Handler struct {
	svc *Service
}

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type createRequest struct {
	Rating int      `json:"rating"`
	Body   *string  `json:"body"`
	Tags   []string `json:"tags"`
}

// Create handles POST /api/v1/bookings/{id}/review (auth required). One
// verified review per completed booking, by the booking's tourist only
// (spec §4.4).
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	bookingID := r.PathValue("id")
	if !uuidRe.MatchString(bookingID) {
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "booking not found", nil)
		return
	}
	var req createRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "malformed JSON body", nil)
		return
	}

	id, _ := rbac.FromContext(r.Context())
	rev, err := h.svc.Create(r.Context(), bookingID, id.UserID, req.Rating, req.Body, req.Tags)
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrForbidden):
		// 404 for both — review permission never leaks booking existence.
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "booking not found", nil)
		return
	case errors.Is(err, ErrBookingNotCompleted):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "REVIEW_NOT_ALLOWED",
			"only a completed booking can be reviewed (spec §4.4)", nil)
		return
	case errors.Is(err, ErrAlreadyReviewed):
		httpx.WriteError(w, r, http.StatusConflict, "ALREADY_REVIEWED",
			"this booking already has a review", nil)
		return
	case errors.Is(err, ErrNoGuide):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "REVIEW_NOT_ALLOWED",
			"this booking had no guide to review", nil)
		return
	case errors.Is(err, ErrValidation):
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
		return
	case err != nil:
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not save the review", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"review": rev})
}

// List handles GET /api/v1/guides/{id}/reviews (public): the guide's
// reviews plus the cached aggregate, offset-paginated.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	guideID := r.PathValue("id")
	if !uuidRe.MatchString(guideID) {
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "guide not found", nil)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	reviews, err := h.svc.List(r.Context(), guideID, limit, offset)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load reviews", nil)
		return
	}
	avg, count, err := h.svc.repo.Aggregate(r.Context(), guideID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load rating", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"reviews":      reviews,
		"rating_avg":   avg,
		"rating_count": count,
	})
}
