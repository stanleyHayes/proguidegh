package bookings

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"proguidegh/api/internal/platform/httpx"
	"proguidegh/api/internal/platform/rbac"
)

// Handler serves the booking endpoints (spec §13.3). Routes that need auth
// are wrapped in RequireAuth at the router; permission scoping (owner,
// assigned guide, bookings.read) is enforced per request here.
type Handler struct {
	svc *Service
}

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "malformed JSON body", nil)
		return false
	}
	return true
}

type quoteRequest struct {
	PackageID string    `json:"package_id"`
	StartsAt  time.Time `json:"starts_at"`
	Guests    int       `json:"guests"`
}

// Quote handles POST /api/v1/bookings/quote (unauthenticated): returns the
// server-authoritative price breakdown (spec §14 — never trust client
// totals). Region-scoped pricing applies at booking creation, which re-quotes
// with the guide's region.
func (h *Handler) Quote(w http.ResponseWriter, r *http.Request) {
	var req quoteRequest
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.PackageID) == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "package_id is required", nil)
		return
	}
	q, err := h.svc.Quote(r.Context(), QuoteParams{
		PackageID: req.PackageID,
		StartsAt:  req.StartsAt,
		Guests:    req.Guests,
	})
	switch {
	case errors.Is(err, ErrPackageNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "package not found", nil)
		return
	case errors.Is(err, ErrPackageInactive):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "PACKAGE_INACTIVE", "package is not bookable", nil)
		return
	case errors.Is(err, ErrValidation):
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
		return
	case err != nil:
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not compute quote", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"quote": q})
}

type createRequest struct {
	PackageID    string       `json:"package_id"`
	GuideID      string       `json:"guide_id"`
	StartsAt     time.Time    `json:"starts_at"`
	MeetingPoint *string      `json:"meeting_point"`
	MeetingLat   *json.Number `json:"meeting_lat"`
	MeetingLng   *json.Number `json:"meeting_lng"`
	Guests       int          `json:"guests"`
	Notes        *string      `json:"notes"`
}

// Create handles POST /api/v1/bookings (auth required). Creates the booking
// in PAYMENT_PENDING after §10.2 eligibility, availability and overlap
// validation; the Idempotency-Key header is required (spec §14) — replays
// return the original booking, reuse with a different payload conflicts.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.PackageID) == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "package_id is required", nil)
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED",
			"booking creation requires an Idempotency-Key header (spec §14)", nil)
		return
	}

	id, _ := rbac.FromContext(r.Context())
	res, quote, err := h.svc.Create(r.Context(), CreateParams{
		TouristID:      id.UserID,
		PackageID:      req.PackageID,
		GuideID:        req.GuideID,
		StartsAt:       req.StartsAt,
		MeetingPoint:   req.MeetingPoint,
		MeetingLat:     numberString(req.MeetingLat),
		MeetingLng:     numberString(req.MeetingLng),
		Guests:         req.Guests,
		Notes:          req.Notes,
		IdempotencyKey: key,
	})
	switch {
	case errors.Is(err, ErrValidation):
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
		return
	case errors.Is(err, ErrPackageNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "package not found", nil)
		return
	case errors.Is(err, ErrPackageInactive):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "PACKAGE_INACTIVE", "package is not bookable", nil)
		return
	case errors.Is(err, ErrGuideNotEligible):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "GUIDE_NOT_ELIGIBLE",
			"guide is not certified and active (spec §10.2)", nil)
		return
	case errors.Is(err, ErrGuideUnavailable):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "GUIDE_UNAVAILABLE",
			"guide is not available at the requested time", nil)
		return
	case errors.Is(err, ErrOverlap):
		httpx.WriteError(w, r, http.StatusConflict, "BOOKING_OVERLAP",
			"guide already holds an active booking for this time", nil)
		return
	case errors.Is(err, ErrIdempotencyConflict):
		httpx.WriteError(w, r, http.StatusConflict, "IDEMPOTENCY_CONFLICT",
			"Idempotency-Key was already used with a different payload", nil)
		return
	case errors.Is(err, ErrIdempotencyInProgress):
		httpx.WriteError(w, r, http.StatusConflict, "IDEMPOTENCY_IN_PROGRESS",
			"the original request with this key is still being processed; retry shortly", nil)
		return
	case err != nil:
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not create booking", nil)
		return
	}

	status := http.StatusCreated
	if res.Replayed {
		status = http.StatusOK
	}
	httpx.WriteJSON(w, status, map[string]any{
		"booking": res.Booking,
		"price":   quote.Price,
	})
}

// Get handles GET /api/v1/bookings/{id} (auth required). Visible to the
// owning tourist, the assigned guide, or holders of bookings.read (spec
// §13.3); anyone else gets 404 so booking existence never leaks.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	bookingID := r.PathValue("id")
	if !uuidRe.MatchString(bookingID) {
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "booking not found", nil)
		return
	}
	b, err := h.svc.repo.GetByID(r.Context(), bookingID)
	if errors.Is(err, ErrNotFound) {
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
	if !owner && !guide && !id.Has("bookings.read") {
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "booking not found", nil)
		return
	}

	events, err := h.svc.repo.ListEvents(r.Context(), b.ID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load booking history", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"booking": b, "events": events})
}

// ListMine handles GET /api/v1/me/bookings (auth required): the caller's own
// bookings, newest first, cursor-paginated (spec §14: cursor pagination for
// growing history tables).
func (h *Handler) ListMine(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	var cursorAt time.Time
	var cursorID string
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		t, cid, err := decodeCursor(raw)
		if err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "invalid cursor", nil)
			return
		}
		cursorAt, cursorID = t, cid
	}

	id, _ := rbac.FromContext(r.Context())
	rows, err := h.svc.repo.ListByTourist(r.Context(), id.UserID, cursorAt, cursorID, limit+1)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not list bookings", nil)
		return
	}

	var next *string
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		next = &c
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"bookings":    rows,
		"next_cursor": next,
		"limit":       limit,
	})
}

// ListGuide handles GET /api/v1/me/guide/bookings (auth required): every
// booking assigned to the caller as guide, upcoming-first then past — the
// split-friendly shape for the guide mobile "my tours" list.
func (h *Handler) ListGuide(w http.ResponseWriter, r *http.Request) {
	id, _ := rbac.FromContext(r.Context())
	rows, err := h.svc.repo.ListByGuide(r.Context(), id.UserID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not list bookings", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"bookings": rows})
}

func numberString(n *json.Number) *string {
	if n == nil {
		return nil
	}
	s := n.String()
	return &s
}
