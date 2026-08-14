// Package observability wires structured JSON logging (log/slog) and the
// request-id / access-log HTTP middleware.
package observability

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"proguidegh/api/internal/platform/httpx"
)

// NewLogger builds a JSON slog logger writing to stdout. Level is debug in
// local environments, info elsewhere.
func NewLogger(appEnv string) *slog.Logger {
	level := slog.LevelInfo
	if appEnv == "local" {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}

// RequestIDMiddleware assigns each request a request ID: it reuses the
// inbound X-Request-ID header when present, otherwise generates one, stores
// it on the context (for the error envelope), and echoes it back on the
// response.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, httpx.WithRequestID(r, id))
	})
}

// statusRecorder captures the response status for access logs.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Hijack delegates http.Hijacker so WebSocket upgrades (Phase 5 realtime
// endpoints) survive the access-log wrapper; without it the upgrade fails
// with 501 before the handler runs.
func (s *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := s.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("observability: underlying writer is not a http.Hijacker")
	}
	return h.Hijack()
}

// AccessLogMiddleware emits one structured log line per request.
func AccessLogMiddleware(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", httpx.RequestID(r),
		)
	})
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(b[:])
}
