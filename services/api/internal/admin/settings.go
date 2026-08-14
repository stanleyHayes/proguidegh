package admin

// Phase 8 (P8-03, P8-04): versioned notification templates and the system
// settings (policy) editor. Templates are created as a new inactive
// version; activation supersedes the previous active version atomically.
// Both surfaces are settings.manage-gated and audited.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"proguidegh/api/internal/platform/audit"
	"proguidegh/api/internal/platform/httpx"
	"proguidegh/api/internal/platform/rbac"
)

// Template is one notification_templates row (P8-03).
type Template struct {
	ID        string    `json:"id"`
	Key       string    `json:"key"`
	Version   int       `json:"version"`
	Channel   string    `json:"channel"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
}

// Setting is one system_settings row (value kept as raw JSON).
type Setting struct {
	Key       string          `json:"key"`
	Value     json.RawMessage `json:"value"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// errTemplateNotFound — no template with that id.
var errTemplateNotFound = errors.New("admin: template not found")

// ListTemplates returns every template version, newest first per key.
func (r *Repository) ListTemplates(ctx context.Context) ([]Template, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, key, version, channel, subject, body, active, created_at
		 FROM notification_templates ORDER BY key, version DESC`)
	if err != nil {
		return nil, fmt.Errorf("admin: list templates: %w", err)
	}
	defer rows.Close()
	var out []Template
	for rows.Next() {
		var t Template
		if err := rows.Scan(&t.ID, &t.Key, &t.Version, &t.Channel, &t.Subject, &t.Body, &t.Active, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("admin: scan template: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// CreateTemplateVersion inserts the next version of a key, inactive.
func (r *Repository) CreateTemplateVersion(ctx context.Context, key, channel, subject, body string) (Template, error) {
	var t Template
	err := r.pool.QueryRow(ctx,
		`INSERT INTO notification_templates (key, version, channel, subject, body)
		 VALUES ($1, COALESCE((SELECT MAX(version) FROM notification_templates WHERE key = $1), 0) + 1, $2, $3, $4)
		 RETURNING id, key, version, channel, subject, body, active, created_at`,
		key, channel, subject, body).
		Scan(&t.ID, &t.Key, &t.Version, &t.Channel, &t.Subject, &t.Body, &t.Active, &t.CreatedAt)
	if err != nil {
		return Template{}, fmt.Errorf("admin: create template: %w", err)
	}
	return t, nil
}

// ActivateTemplate supersedes the key's active version atomically.
func (r *Repository) ActivateTemplate(ctx context.Context, id string) (Template, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Template{}, fmt.Errorf("admin: begin activate: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var key string
	if err := tx.QueryRow(ctx,
		`SELECT key FROM notification_templates WHERE id = $1`, id).Scan(&key); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Template{}, errTemplateNotFound
		}
		return Template{}, fmt.Errorf("admin: load template: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE notification_templates SET active = false WHERE key = $1 AND active`, key); err != nil {
		return Template{}, fmt.Errorf("admin: deactivate: %w", err)
	}
	var t Template
	if err := tx.QueryRow(ctx,
		`UPDATE notification_templates SET active = true WHERE id = $1
		 RETURNING id, key, version, channel, subject, body, active, created_at`, id).
		Scan(&t.ID, &t.Key, &t.Version, &t.Channel, &t.Subject, &t.Body, &t.Active, &t.CreatedAt); err != nil {
		return Template{}, fmt.Errorf("admin: activate: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Template{}, fmt.Errorf("admin: commit activate: %w", err)
	}
	return t, nil
}

// ListSettings returns all system settings, key-ordered.
func (r *Repository) ListSettings(ctx context.Context) ([]Setting, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT key, value_json, updated_at FROM system_settings ORDER BY key`)
	if err != nil {
		return nil, fmt.Errorf("admin: list settings: %w", err)
	}
	defer rows.Close()
	var out []Setting
	for rows.Next() {
		var s Setting
		if err := rows.Scan(&s.Key, &s.Value, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("admin: scan setting: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// PutSetting upserts one system setting.
func (r *Repository) PutSetting(ctx context.Context, key string, value json.RawMessage) (Setting, error) {
	var s Setting
	err := r.pool.QueryRow(ctx,
		`INSERT INTO system_settings (key, value_json) VALUES ($1, $2)
		 ON CONFLICT (key) DO UPDATE SET value_json = $2, version = system_settings.version + 1, updated_at = now()
		 RETURNING key, value_json, updated_at`, key, value).
		Scan(&s.Key, &s.Value, &s.UpdatedAt)
	if err != nil {
		return Setting{}, fmt.Errorf("admin: put setting: %w", err)
	}
	return s, nil
}

// ListAdminTemplates handles GET /api/v1/admin/notification-templates.
func (h *Handler) ListAdminTemplates(w http.ResponseWriter, r *http.Request) {
	templates, err := h.repo.ListTemplates(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not list templates", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"templates": templates})
}

type templateRequest struct {
	Key     string `json:"key"`
	Channel string `json:"channel"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// CreateTemplate handles POST /api/v1/admin/notification-templates — a new
// inactive version of the key (P8-03; audited).
func (h *Handler) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	var req templateRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "malformed JSON body", nil)
		return
	}
	req.Key = strings.TrimSpace(req.Key)
	req.Body = strings.TrimSpace(req.Body)
	switch req.Channel {
	case "email", "sms", "push", "in_app":
	default:
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "channel must be email|sms|push|in_app", nil)
		return
	}
	if req.Key == "" || req.Body == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "key and body are required", nil)
		return
	}
	t, err := h.repo.CreateTemplateVersion(r.Context(), req.Key, req.Channel, req.Subject, req.Body)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not create template", nil)
		return
	}
	h.recordAudit(r, "templates.version_created", "notification_template", t.ID,
		nil, map[string]any{"key": t.Key, "version": t.Version, "channel": t.Channel})
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"template": t})
}

// ActivateTemplate handles POST /api/v1/admin/notification-templates/{id}/activate.
func (h *Handler) ActivateTemplate(w http.ResponseWriter, r *http.Request) {
	t, err := h.repo.ActivateTemplate(r.Context(), r.PathValue("id"))
	if errors.Is(err, errTemplateNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "template not found", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not activate template", nil)
		return
	}
	h.recordAudit(r, "templates.activated", "notification_template", t.ID,
		nil, map[string]any{"key": t.Key, "version": t.Version})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"template": t})
}

// ListSettings handles GET /api/v1/admin/settings (P8-04 policy config).
func (h *Handler) ListSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.repo.ListSettings(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not list settings", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"settings": settings})
}

type putSettingRequest struct {
	Value json.RawMessage `json:"value"`
}

// PutSetting handles PUT /api/v1/admin/settings/{key} {value: <any JSON>}
// (settings.manage; audited with before/after).
func (h *Handler) PutSetting(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	var req putSettingRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil || len(req.Value) == 0 {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "body must be {\"value\": <any JSON>}", nil)
		return
	}

	var before json.RawMessage
	_ = h.repo.pool.QueryRow(r.Context(),
		`SELECT value_json FROM system_settings WHERE key = $1`, key).Scan(&before)

	s, err := h.repo.PutSetting(r.Context(), key, req.Value)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not save setting", nil)
		return
	}
	h.recordAudit(r, "settings.updated", "system_setting", "",
		json.RawMessage(before), s.Value)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"setting": s})
}

// recordAudit is a small helper so the Phase 8 endpoints audit uniformly.
func (h *Handler) recordAudit(r *http.Request, action, entityType, entityID string, before, after any) {
	if h.audit == nil {
		return
	}
	identity, _ := rbac.FromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Entry{
		ActorID:    identity.UserID,
		Action:     action,
		EntityType: entityType,
		EntityID:   entityID,
		Before:     before,
		After:      after,
	})
}
