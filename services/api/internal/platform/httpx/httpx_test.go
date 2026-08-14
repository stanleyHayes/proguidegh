package httpx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteErrorEnvelopeShape(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req = req.WithContext(context.WithValue(req.Context(), requestIDKey{}, "req-123"))

	rec := httptest.NewRecorder()
	WriteError(rec, req, http.StatusUnprocessableEntity, "VALIDATION_FAILED", "bad input", map[string]string{"field": "name"})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}

	var body map[string]map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	e, ok := body["error"]
	if !ok {
		t.Fatal("missing top-level \"error\" key")
	}
	if e["code"] != "VALIDATION_FAILED" {
		t.Errorf("code = %v, want VALIDATION_FAILED", e["code"])
	}
	if e["message"] != "bad input" {
		t.Errorf("message = %v, want %q", e["message"], "bad input")
	}
	if e["request_id"] != "req-123" {
		t.Errorf("request_id = %v, want req-123", e["request_id"])
	}
	details, ok := e["details"].(map[string]any)
	if !ok || details["field"] != "name" {
		t.Errorf("details = %v, want map[field:name]", e["details"])
	}
}

func TestWriteErrorWithoutDetailsOmitsField(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	WriteError(rec, req, http.StatusNotFound, "NOT_FOUND", "missing", nil)

	var body map[string]map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, present := body["error"]["details"]; present {
		t.Error("details should be omitted when nil")
	}
	if body["error"]["request_id"] != "" {
		t.Errorf("request_id = %v, want empty without middleware", body["error"]["request_id"])
	}
}
