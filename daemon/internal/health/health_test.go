package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type mockCounter struct{ count int }

func (m mockCounter) RunningAgentsCount() int { return m.count }

func TestHealthzReturns200(t *testing.T) {
	s := NewServer(mockCounter{count: 3})
	s.SetReady(true)

	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %v", body["status"])
	}
	// /healthz now returns only {"status":"ok"} — no agents, version, or uptime
	if len(body) != 1 {
		t.Errorf("expected 1 field in response, got %d: %v", len(body), body)
	}
}

func TestReadyzNotReady(t *testing.T) {
	s := NewServer(mockCounter{})

	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestReadyzReady(t *testing.T) {
	s := NewServer(mockCounter{})
	s.SetReady(true)

	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHealthzNilCounter(t *testing.T) {
	s := NewServer(nil)
	s.SetReady(true)

	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %v", body["status"])
	}
}
