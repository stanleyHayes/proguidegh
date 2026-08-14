package privacy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"proguidegh/api/internal/platform/audit"
	"proguidegh/api/internal/platform/httpx"
	"proguidegh/api/internal/platform/rbac"
)

// MarketingSettingKey is the system_settings row the marketing site reads and
// admin-web writes. Reusing system_settings rather than adding a CMS table is
// deliberate: the admin settings endpoints are already permission-gated,
// audited and versioned, and marketing copy needs exactly those properties.
const MarketingSettingKey = "marketing.site"

// MarketingContent returns the published marketing document.
//
// Public and unauthenticated by necessity — it feeds a public website. Reading
// only this one key (rather than exposing system_settings generally) keeps
// pricing rules, feature flags and provider configuration out of reach.
func (r *Repository) MarketingContent(ctx context.Context) (json.RawMessage, error) {
	var raw json.RawMessage
	err := r.pool.QueryRow(ctx,
		`SELECT value_json FROM system_settings WHERE key = $1`, MarketingSettingKey).
		Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("privacy: marketing content: %w", err)
	}
	return raw, nil
}

// MarketingContentHandler serves GET /api/v1/content/marketing.
//
// A missing row is 200 with a null document, not 404: the site falls back to
// its built-in launch copy, and a cold environment should render a correct
// marketing page rather than an error.
func (h *Handler) MarketingContentHandler(w http.ResponseWriter, r *http.Request) {
	raw, err := h.repo.MarketingContent(r.Context())
	if errors.Is(err, ErrNotFound) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"content": nil})
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL",
			"could not load marketing content", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"content": raw})
}

// ---------------------------------------------------------------------------
// Legal document administration
// ---------------------------------------------------------------------------

// PublishLegalVersion inserts a NEW version of a document.
//
// Never an UPDATE. consent_records references (document, version), so a user
// who accepted privacy@2026-08-14 accepted that specific text; rewriting the
// row in place would silently re-point their recorded consent at different
// words. New text is a new version, and the old one stays readable.
func (r *Repository) PublishLegalVersion(ctx context.Context, doc, version, url, summary, body string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO legal_documents (document, version, url, summary, body, approved)
		VALUES ($1, $2, $3, $4, $5, false)`, doc, version, url, summary, body)
	if err != nil {
		return fmt.Errorf("privacy: publish legal version: %w", err)
	}
	return nil
}

// ApproveLegalVersion marks one version as counsel-approved, which is what
// removes the draft banner from the public page.
func (r *Repository) ApproveLegalVersion(ctx context.Context, doc, version, approverID string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE legal_documents
		SET approved = true, approved_at = now(), approved_by = $3
		WHERE document = $1 AND version = $2 AND approved = false`,
		doc, version, approverID)
	if err != nil {
		return fmt.Errorf("privacy: approve legal version: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AllLegalVersions lists every version of every document, newest first, for
// the admin editor. Includes bodies so an editor can diff against the text
// they are replacing.
func (r *Repository) AllLegalVersions(ctx context.Context) ([]LegalDocument, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT document, version, url, summary, body, approved, approved_at, published_at
		FROM legal_documents ORDER BY document, published_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("privacy: all legal versions: %w", err)
	}
	defer rows.Close()
	out := []LegalDocument{}
	for rows.Next() {
		var d LegalDocument
		if err := rows.Scan(&d.Document, &d.Version, &d.URL, &d.Summary, &d.Body,
			&d.Approved, &d.ApprovedAt, &d.PublishedAt); err != nil {
			return nil, fmt.Errorf("privacy: scan legal version: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

type publishLegalRequest struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	Summary string `json:"summary"`
	Body    string `json:"body"`
}

var legalDocuments = map[string]bool{"terms": true, "privacy": true, "location": true}

// AdminListLegal handles GET /api/v1/admin/legal — every version, for the editor.
func (h *Handler) AdminListLegal(w http.ResponseWriter, r *http.Request) {
	docs, err := h.repo.AllLegalVersions(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL",
			"could not load legal documents", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"documents": docs})
}

// AdminPublishLegal handles POST /api/v1/admin/legal/{document} — a new version.
func (h *Handler) AdminPublishLegal(w http.ResponseWriter, r *http.Request) {
	doc := r.PathValue("document")
	if !legalDocuments[doc] {
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "unknown legal document", nil)
		return
	}
	var req publishLegalRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "malformed JSON body", nil)
		return
	}
	req.Version = strings.TrimSpace(req.Version)
	if req.Version == "" || strings.TrimSpace(req.Body) == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION",
			"version and body are required", nil)
		return
	}
	if req.URL == "" {
		req.URL = "https://proguidegh.com/legal/" + doc
	}

	if err := h.repo.PublishLegalVersion(r.Context(), doc, req.Version, req.URL,
		req.Summary, req.Body); err != nil {
		// A duplicate version is a conflict, not a server fault: the editor
		// must pick a new version rather than silently overwrite text that
		// users have already consented to.
		httpx.WriteError(w, r, http.StatusConflict, "VERSION_EXISTS",
			"that version already exists; publish under a new version instead", nil)
		return
	}

	id, _ := rbac.FromContext(r.Context())
	if err := h.auditor.RecordHTTP(r.Context(), r, audit.Entry{
		ActorID:    id.UserID,
		Action:     "legal.publish",
		EntityType: "legal_document",
		// No EntityID: the column is uuid and this entity is keyed by
		// (document, version), which goes in the payload instead.
		After: map[string]any{
			"document": doc,
			"version":  req.Version,
			"bytes":    len(req.Body),
		},
	}); err != nil {
		slog.Error("audit legal publish", "document", doc, "error", err)
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"status": "published"})
}

// AdminApproveLegal handles POST /api/v1/admin/legal/{document}/{version}/approve.
// Approval is what removes the draft banner from the public page, so it is a
// privileged, audited action distinct from publishing the text.
func (h *Handler) AdminApproveLegal(w http.ResponseWriter, r *http.Request) {
	doc, version := r.PathValue("document"), r.PathValue("version")
	if !legalDocuments[doc] {
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "unknown legal document", nil)
		return
	}
	id, _ := rbac.FromContext(r.Context())
	if err := h.repo.ApproveLegalVersion(r.Context(), doc, version, id.UserID); err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND",
				"unknown version, or it is already approved", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL",
			"could not approve document", nil)
		return
	}
	if err := h.auditor.RecordHTTP(r.Context(), r, audit.Entry{
		ActorID:    id.UserID,
		Action:     "legal.approve",
		EntityType: "legal_document",
		After:      map[string]any{"document": doc, "version": version},
	}); err != nil {
		slog.Error("audit legal approve", "document", doc, "error", err)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "approved"})
}
