package payments

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"

	"proguidegh/api/internal/bookings"
	"proguidegh/api/internal/platform/audit"
	"proguidegh/api/internal/platform/httpx"
	"proguidegh/api/internal/platform/rbac"
)

// Handler serves the payment endpoints (spec §13.3). The webhook route is
// public but authenticated by the provider signature; the rest run behind
// RequireAuth (and RequirePermission for refunds) at the router.
type Handler struct {
	svc      *Service
	bookings *bookings.Repository
	auditor  *audit.Recorder
}

// NewHandler builds the handler.
func NewHandler(svc *Service, bookingsRepo *bookings.Repository, auditor *audit.Recorder) *Handler {
	return &Handler{svc: svc, bookings: bookingsRepo, auditor: auditor}
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// CreateIntent handles POST /api/v1/bookings/{id}/payment-intent (owner
// tourist only, Idempotency-Key required). Initializes the provider payment
// against the booking's server-authoritative amount and returns the hosted
// authorization URL. A replayed key returns the original payment and URL.
func (h *Handler) CreateIntent(w http.ResponseWriter, r *http.Request) {
	bookingID := r.PathValue("id")
	if !uuidRe.MatchString(bookingID) {
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "booking not found", nil)
		return
	}
	b, err := h.bookings.GetByID(r.Context(), bookingID)
	if errors.Is(err, bookings.ErrNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "booking not found", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load booking", nil)
		return
	}

	// Owner tourists only; 404 for anyone else so existence never leaks.
	id, _ := rbac.FromContext(r.Context())
	if b.TouristID != id.UserID {
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "booking not found", nil)
		return
	}

	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED",
			"payment initiation requires an Idempotency-Key header (spec §14)", nil)
		return
	}

	email, err := h.svc.repo.UserEmail(r.Context(), id.UserID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load account", nil)
		return
	}

	res, err := h.svc.CreateIntent(r.Context(), b, email, key)
	switch {
	case errors.Is(err, ErrValidation):
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
		return
	case errors.Is(err, ErrNotPayable):
		httpx.WriteError(w, r, http.StatusConflict, "NOT_PAYABLE", err.Error(), nil)
		return
	case errors.Is(err, ErrAlreadyActive):
		httpx.WriteError(w, r, http.StatusConflict, "PAYMENT_ALREADY_ACTIVE",
			"an active payment already exists for this booking", nil)
		return
	case errors.Is(err, ErrIdempotencyConflict):
		httpx.WriteError(w, r, http.StatusConflict, "IDEMPOTENCY_CONFLICT", err.Error(), nil)
		return
	case err != nil:
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not initialize payment", nil)
		return
	}

	status := http.StatusCreated
	if res.Replayed {
		status = http.StatusOK
	}
	httpx.WriteJSON(w, status, map[string]any{"payment": res.Payment})
}

// Webhook handles POST /api/v1/webhooks/payments/{provider} (public; the
// provider signature IS the authentication, spec §14). The exact raw bytes
// are verified before any parsing; replays are 200 no-ops.
func (h *Handler) Webhook(w http.ResponseWriter, r *http.Request) {
	providerName := r.PathValue("provider")
	if providerName != h.svc.Provider().Name() {
		// Unknown/inactive provider — do not reveal which provider is live.
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "unknown webhook endpoint", nil)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "could not read webhook body", nil)
		return
	}

	outcome, err := h.svc.HandleWebhook(r.Context(), r.Header, raw)
	switch {
	case errors.Is(err, ErrBadSignature):
		httpx.WriteError(w, r, http.StatusUnauthorized, "INVALID_SIGNATURE", "webhook signature verification failed", nil)
		return
	case errors.Is(err, ErrUnknownReference):
		httpx.WriteError(w, r, http.StatusBadRequest, "UNKNOWN_REFERENCE", "webhook references an unknown payment", nil)
		return
	case err != nil:
		httpx.WriteError(w, r, http.StatusBadRequest, "INVALID_WEBHOOK", "could not process webhook", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"received": true, "replay": outcome.Replay})
}

// Get handles GET /api/v1/payments/{id} — visible to the owning tourist or
// payments.read holders; 404 otherwise.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	paymentID := r.PathValue("id")
	if !uuidRe.MatchString(paymentID) {
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "payment not found", nil)
		return
	}
	p, err := h.svc.repo.GetByID(r.Context(), paymentID)
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "payment not found", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load payment", nil)
		return
	}

	id, _ := rbac.FromContext(r.Context())
	if !id.Has("payments.read") {
		b, err := h.bookings.GetByID(r.Context(), p.BookingID)
		if err != nil || b.TouristID != id.UserID {
			httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "payment not found", nil)
			return
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"payment": p})
}

type refundRequest struct {
	Reason string `json:"reason"`
}

// Refund handles POST /api/v1/payments/{id}/refund (permission
// payments.refund, Idempotency-Key required, audited — spec §4.5, §14).
// Issues the provider refund, drives payment and booking to REFUNDED through
// their state machines and posts the reversing ledger entries.
func (h *Handler) Refund(w http.ResponseWriter, r *http.Request) {
	paymentID := r.PathValue("id")
	if !uuidRe.MatchString(paymentID) {
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "payment not found", nil)
		return
	}
	var req refundRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "malformed JSON body", nil)
			return
		}
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED",
			"refunds require an Idempotency-Key header (spec §14)", nil)
		return
	}

	id, _ := rbac.FromContext(r.Context())
	res, err := h.svc.Refund(r.Context(), paymentID, strings.TrimSpace(req.Reason), key)
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "payment not found", nil)
		return
	case errors.Is(err, ErrValidation):
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
		return
	case errors.Is(err, ErrNotRefundable):
		httpx.WriteError(w, r, http.StatusConflict, "NOT_REFUNDABLE", err.Error(), nil)
		return
	case errors.Is(err, ErrIdempotencyConflict):
		httpx.WriteError(w, r, http.StatusConflict, "IDEMPOTENCY_CONFLICT", err.Error(), nil)
		return
	case err != nil:
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not refund payment", nil)
		return
	}

	// Financially significant action: audit is mandatory (spec §1.2). Skipped
	// on idempotent replays — the original refund already wrote its row.
	if !res.Replayed {
		if err := h.auditor.RecordHTTP(r.Context(), r, audit.Entry{
			ActorID:    id.UserID,
			Action:     "payments.refund",
			EntityType: "payment",
			EntityID:   paymentID,
			Before:     map[string]string{"status": StatusSucceeded},
			After:      map[string]string{"status": res.Payment.Status, "reversal": res.ReversalReference},
		}); err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "refund applied but audit failed", nil)
			return
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"payment": res.Payment,
		"refund": map[string]string{
			"id":                 res.RefundID,
			"reversal_reference": res.ReversalReference,
		},
	})
}
