package privacy

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"proguidegh/api/internal/platform/audit"
	"proguidegh/api/internal/platform/httpx"
	"proguidegh/api/internal/platform/rbac"
)

// ObjectDeleter removes private objects from R2 after an erasure commits.
type ObjectDeleter interface {
	Delete(ctx context.Context, key string) error
}

// Handler serves the data-subject endpoints. All are self-scoped: a caller can
// only ever act on their own record, so no extra permission is required and
// none is granted for acting on someone else's.
type Handler struct {
	repo    *Repository
	auditor *audit.Recorder
	objects ObjectDeleter
}

// NewHandler builds the handler. objects may be nil in environments without
// private storage configured; document rows are still removed.
func NewHandler(repo *Repository, auditor *audit.Recorder, objects ObjectDeleter) *Handler {
	return &Handler{repo: repo, auditor: auditor, objects: objects}
}

// Policies handles GET /api/v1/legal/policies (public).
//
// Public on purpose: both stores require the privacy policy to be reachable
// without an account, and the sign-up screen has to show it before the user
// has one.
func (h *Handler) Policies(w http.ResponseWriter, r *http.Request) {
	docs, err := h.repo.CurrentPolicies(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load policies", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"policies": docs})
}

type consentRequest struct {
	Document string `json:"document"`
	Version  string `json:"version"`
}

var consentDocuments = map[string]bool{"terms": true, "privacy": true, "location": true}

// Consent handles POST /api/v1/me/consent — records one acceptance.
func (h *Handler) Consent(w http.ResponseWriter, r *http.Request) {
	var req consentRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "malformed JSON body", nil)
		return
	}
	req.Document = strings.TrimSpace(req.Document)
	req.Version = strings.TrimSpace(req.Version)
	if !consentDocuments[req.Document] {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION",
			"document must be terms, privacy or location", nil)
		return
	}
	if req.Version == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "version is required", nil)
		return
	}

	id, _ := rbac.FromContext(r.Context())
	if err := h.repo.RecordConsent(r.Context(), id.UserID, req.Document, req.Version, "app"); err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not record consent", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"status": "recorded"})
}

// Export handles GET /api/v1/me/export — Act 843 s.32 subject access.
//
// Served inline as JSON rather than emailed: the store reviewers must be able
// to see the right exercised end-to-end inside the app, and an email round
// trip cannot be demonstrated in a review session.
func (h *Handler) Export(w http.ResponseWriter, r *http.Request) {
	id, _ := rbac.FromContext(r.Context())
	data, err := h.repo.BuildExport(r.Context(), id.UserID)
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "account not found", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not build export", nil)
		return
	}

	// Subject access is a privileged read of a full personal-data set; §22
	// wants it on the record.
	if err := h.auditor.RecordHTTP(r.Context(), r, audit.Entry{
		ActorID:    id.UserID,
		Action:     "privacy.export",
		EntityType: "user",
		EntityID:   id.UserID,
	}); err != nil {
		slog.Error("audit privacy export", "user_id", id.UserID, "error", err)
	}

	w.Header().Set("Content-Disposition", `attachment; filename="proguidegh-my-data.json"`)
	httpx.WriteJSON(w, http.StatusOK, data)
}

// DeletionPreview handles GET /api/v1/me/deletion — what would happen, and
// whether anything currently blocks it. Lets the app show the real
// consequences before the user confirms, rather than after.
func (h *Handler) DeletionPreview(w http.ResponseWriter, r *http.Request) {
	id, _ := rbac.FromContext(r.Context())
	blockers, err := h.repo.DeletionBlockers(r.Context(), id.UserID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not check account", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"can_delete": len(blockers) == 0,
		"blockers":   blockers,
		"retained": []string{
			"Payment records, receipts and ledger entries are kept for tax and " +
				"tourism-levy reconciliation as required by Ghanaian law.",
			"Reviews you wrote stay visible but are no longer linked to your name.",
		},
		"removed": []string{
			"Your name, email address, phone number and password.",
			"Your emergency contact details.",
			"Any verification documents you uploaded.",
			"Your payout account details.",
			"Your location history.",
		},
	})
}

// Delete handles DELETE /api/v1/me — irreversible account erasure.
//
// Apple 5.1.1(v) requires this to be reachable in-app and to actually delete,
// not merely deactivate. It is not undoable and the response says so.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, _ := rbac.FromContext(r.Context())

	blockers, err := h.repo.DeletionBlockers(r.Context(), id.UserID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not check account", nil)
		return
	}
	if len(blockers) > 0 {
		if err := h.repo.RecordBlocked(r.Context(), id.UserID, blockers[0].Reason); err != nil {
			slog.Error("record blocked deletion", "user_id", id.UserID, "error", err)
		}
		httpx.WriteError(w, r, http.StatusConflict, "DELETION_BLOCKED",
			blockers[0].Message, map[string]any{"blockers": blockers})
		return
	}

	result, err := h.repo.Anonymize(r.Context(), id.UserID)
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "account not found", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not delete account", nil)
		return
	}

	// Audit before the object sweep: the erasure is already committed, and a
	// storage failure must not lose the record that it happened.
	if err := h.auditor.RecordHTTP(r.Context(), r, audit.Entry{
		ActorID:    id.UserID,
		Action:     "privacy.account.delete",
		EntityType: "user",
		EntityID:   id.UserID,
		After:      map[string]any{"cleared": result.Cleared},
	}); err != nil {
		slog.Error("audit account deletion", "user_id", id.UserID, "error", err)
	}

	// Private documents live in R2, outside the transaction. A failure here
	// leaves an orphaned object with no row pointing at it and no signed URL
	// that can reach it; it is logged for the retention sweep rather than
	// failing a deletion the user has already been told is done.
	if h.objects != nil {
		for _, key := range result.ObjectKeys {
			if err := h.objects.Delete(r.Context(), key); err != nil {
				slog.Error("delete private object after erasure",
					"object_key", key, "error", err)
			}
		}
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":  "deleted",
		"cleared": result.Cleared,
		"message": "Your account and personal data have been deleted. " +
			"This cannot be undone.",
	})
}
