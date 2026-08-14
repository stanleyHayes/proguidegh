// Package httpx provides HTTP helpers: JSON responses and the standard
// error envelope used across the API.
package httpx

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
)

// Error is the standard API error envelope returned for all failures.
type Error struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Details   any    `json:"details,omitempty"`
	RequestID string `json:"request_id"`
}

type errorBody struct {
	Error Error `json:"error"`
}

// requestIDKey is the context key for the per-request ID. It lives here so
// both the middleware and the error envelope share one definition.
type requestIDKey struct{}

// WithRequestID returns a request whose context carries the given request ID.
// Used by the observability middleware.
func WithRequestID(r *http.Request, id string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id))
}

// RequestID returns the request ID stored on the request context by the
// observability middleware, or an empty string when absent.
func RequestID(r *http.Request) string {
	if v, ok := r.Context().Value(requestIDKey{}).(string); ok {
		return v
	}
	return ""
}

// WriteJSON writes v as a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("httpx: encode response", "error", err)
	}
}

// WriteError writes the standard error envelope with the given status code.
func WriteError(w http.ResponseWriter, r *http.Request, status int, code, message string, details any) {
	WriteJSON(w, status, errorBody{Error: Error{
		Code:      code,
		Message:   message,
		Details:   details,
		RequestID: RequestID(r),
	}})
}
