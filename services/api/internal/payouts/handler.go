package payouts

import (
	"encoding/base64"
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

// Handler serves the guide wallet endpoints and the admin finance
// endpoints. Permission scoping is applied at the router
// (payouts.read / payouts.manage / reports.read for admin).
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

// --- Guide-facing (spec §8.1) --------------------------------------------

// Wallet handles GET /api/v1/me/guide/wallet (P7-01).
func (h *Handler) Wallet(w http.ResponseWriter, r *http.Request) {
	wallet, err := h.svc.Wallet(r.Context(), actorID(r))
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not compute wallet", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"wallet": wallet})
}

// Statement handles GET /api/v1/me/guide/statement?cursor&limit — newest
// first, keyset paginated (P7-01).
func (h *Handler) Statement(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))

	var before *time.Time
	var beforeID string
	if raw := strings.TrimSpace(q.Get("cursor")); raw != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "invalid cursor", nil)
			return
		}
		parts := strings.SplitN(string(decoded), "|", 2)
		if len(parts) != 2 {
			httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "invalid cursor", nil)
			return
		}
		t, err := time.Parse(time.RFC3339Nano, parts[0])
		if err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "invalid cursor", nil)
			return
		}
		before = &t
		beforeID = parts[1]
	}

	entries, err := h.svc.Statement(r.Context(), actorID(r), before, beforeID, limit)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load statement", nil)
		return
	}
	var next *string
	if len(entries) > 0 {
		last := entries[len(entries)-1]
		c := base64.RawURLEncoding.EncodeToString([]byte(last.At.Format(time.RFC3339Nano) + "|" + last.ID))
		next = &c
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entries": entries, "next_cursor": next})
}

// GetPayoutAccount handles GET /api/v1/me/guide/payout-account — masked
// destination only (P7-02).
func (h *Handler) GetPayoutAccount(w http.ResponseWriter, r *http.Request) {
	account, err := h.svc.PayoutAccount(r.Context(), actorID(r))
	if errors.Is(err, ErrNotFound) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"account": nil})
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load payout account", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"account": account})
}

type payoutAccountRequest struct {
	Provider   string `json:"provider"`
	Network    string `json:"network"`
	AccountRef string `json:"account_ref"`
}

// PutPayoutAccount handles PUT /api/v1/me/guide/payout-account (P7-02).
func (h *Handler) PutPayoutAccount(w http.ResponseWriter, r *http.Request) {
	var req payoutAccountRequest
	if !decodeBody(w, r, &req) {
		return
	}
	account, err := h.svc.PutPayoutAccount(r.Context(), actorID(r),
		req.Provider, req.Network, req.AccountRef, actorID(r))
	if errors.Is(err, ErrValidation) {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not save payout account", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"account": account})
}

// --- Admin finance (spec §8.4) -------------------------------------------

// List handles GET /api/v1/admin/payouts?status&scheduled_for&limit&offset.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	payouts, total, err := h.svc.ListPayouts(r.Context(),
		strings.TrimSpace(q.Get("status")), strings.TrimSpace(q.Get("scheduled_for")), limit, offset)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not list payouts", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"payouts": payouts, "total": total})
}

type batchRequest struct {
	ScheduledFor string `json:"scheduled_for"`
}

// Batch handles POST /api/v1/admin/payouts/batch {scheduled_for?} — defaults
// to today; idempotent per (guide, scheduled_for) (P7-03, P7-07).
func (h *Handler) Batch(w http.ResponseWriter, r *http.Request) {
	var req batchRequest
	if r.ContentLength > 0 && !decodeBody(w, r, &req) {
		return
	}
	scheduledFor := strings.TrimSpace(req.ScheduledFor)
	if scheduledFor == "" {
		scheduledFor = time.Now().Format("2006-01-02")
	}
	created, eligible, err := h.svc.RunBatch(r.Context(), scheduledFor, actorID(r))
	if errors.Is(err, ErrValidation) {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not run payout batch", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"scheduled_for": scheduledFor, "eligible_guides": eligible, "created": created,
	})
}

type transitionRequest struct {
	To                string `json:"to"`
	FailureReason     string `json:"failure_reason"`
	ProviderReference string `json:"provider_reference"`
}

// Transition handles POST /api/v1/admin/payouts/{id}/transition
// {to, failure_reason?, provider_reference?} (P7-05).
func (h *Handler) Transition(w http.ResponseWriter, r *http.Request) {
	if !uuidRe.MatchString(r.PathValue("id")) {
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "payout not found", nil)
		return
	}
	var req transitionRequest
	if !decodeBody(w, r, &req) {
		return
	}
	payout, err := h.svc.Transition(r.Context(), r.PathValue("id"),
		strings.TrimSpace(req.To), req.FailureReason, req.ProviderReference, actorID(r))
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "payout not found", nil)
	case errors.Is(err, ErrIllegalTransition):
		httpx.WriteError(w, r, http.StatusConflict, "ILLEGAL_TRANSITION", err.Error(), nil)
	case errors.Is(err, ErrValidation):
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
	case err != nil:
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not transition payout", nil)
	default:
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"payout": payout})
	}
}

// Export handles GET /api/v1/admin/payouts/export?scheduled_for=YYYY-MM-DD —
// the finance transfer CSV with decrypted destination refs (P7-04).
func (h *Handler) Export(w http.ResponseWriter, r *http.Request) {
	scheduledFor := strings.TrimSpace(r.URL.Query().Get("scheduled_for"))
	csv, err := h.svc.ExportCSV(r.Context(), scheduledFor, actorID(r))
	if errors.Is(err, ErrValidation) {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not export payouts", nil)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="payouts-`+scheduledFor+`.csv"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(csv))
}

// VerifyAccount handles POST /api/v1/admin/payout-accounts/{id}/verify (P7-02).
func (h *Handler) VerifyAccount(w http.ResponseWriter, r *http.Request) {
	if !uuidRe.MatchString(r.PathValue("id")) {
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "payout account not found", nil)
		return
	}
	account, err := h.svc.VerifyAccount(r.Context(), r.PathValue("id"), actorID(r))
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "payout account not found", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not verify payout account", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"account": account})
}

// LevyReport handles GET /api/v1/admin/reports/tourism-levy?from&to
// (YYYY-MM-DD bounds; P7-06).
func (h *Handler) LevyReport(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var from, to *time.Time
	if raw := strings.TrimSpace(q.Get("from")); raw != "" {
		t, err := time.Parse("2006-01-02", raw)
		if err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "from must be YYYY-MM-DD", nil)
			return
		}
		from = &t
	}
	if raw := strings.TrimSpace(q.Get("to")); raw != "" {
		t, err := time.Parse("2006-01-02", raw)
		if err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "to must be YYYY-MM-DD", nil)
			return
		}
		t = t.AddDate(0, 0, 1) // inclusive end date
		to = &t
	}
	balance, credits, debits, err := h.svc.LevyReport(r.Context(), from, to)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not build levy report", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"report": map[string]any{
		"balance_minor":        balance,
		"period_credits_minor": credits,
		"period_debits_minor":  debits,
		"currency":             "GHS",
	}})
}
