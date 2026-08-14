// Package audit provides the append-only writer for privileged and
// financially significant actions (spec §1.2 decision 8, §22).
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Recorder writes rows to audit_logs. The table is append-only by policy:
// this package exposes no update or delete path.
type Recorder struct {
	pool *pgxpool.Pool
}

// NewRecorder builds a Recorder.
func NewRecorder(pool *pgxpool.Pool) *Recorder { return &Recorder{pool: pool} }

// Entry is one auditable action.
type Entry struct {
	ActorID    string // user performing the action; empty for system actors
	Action     string // e.g. "admin.users.roles.update"
	EntityType string // e.g. "user"
	EntityID   string // target entity id; empty when not applicable
	Before     any    // state before the mutation (nil when not applicable)
	After      any    // state after the mutation (nil when not applicable)
	IP         string // client IP; empty when unknown
}

// Record appends one audit row. Failures are logged and returned; callers
// performing privileged mutations should treat an error as fatal to the
// operation rather than silently skipping the audit trail.
func (r *Recorder) Record(ctx context.Context, e Entry) error {
	var before, after []byte
	var err error
	if e.Before != nil {
		if before, err = json.Marshal(e.Before); err != nil {
			return fmt.Errorf("audit: marshal before: %w", err)
		}
	}
	if e.After != nil {
		if after, err = json.Marshal(e.After); err != nil {
			return fmt.Errorf("audit: marshal after: %w", err)
		}
	}

	var actor, entity any
	if e.ActorID != "" {
		actor = e.ActorID
	}
	if e.EntityID != "" {
		entity = e.EntityID
	}
	var ip any
	if parsed := net.ParseIP(strings.TrimSpace(e.IP)); parsed != nil {
		ip = parsed.String()
	}

	_, err = r.pool.Exec(ctx, `
		INSERT INTO audit_logs (actor_id, action, entity_type, entity_id, before_json, after_json, ip)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		actor, e.Action, e.EntityType, entity, before, after, ip)
	if err != nil {
		return fmt.Errorf("audit: insert audit_logs: %w", err)
	}
	return nil
}

// RecordHTTP is Record with the client IP derived from the request
// (X-Forwarded-For first hop, falling back to RemoteAddr).
func (r *Recorder) RecordHTTP(ctx context.Context, req *http.Request, e Entry) error {
	e.IP = ClientIP(req)
	if err := r.Record(ctx, e); err != nil {
		slog.Error("audit record failed", "action", e.Action, "error", err)
		return err
	}
	return nil
}

// ClientIP extracts the client IP from the request.
func ClientIP(req *http.Request) string {
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return req.RemoteAddr
	}
	return host
}
