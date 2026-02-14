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

	var body healthzResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("expected status ok, got %s", body.Status)
	}
	if body.RunningAgents != 3 {
		t.Errorf("expected 3 running agents, got %d", body.RunningAgents)
	}
	if body.Version != "dev" {
		t.Errorf("expected version dev, got %s", body.Version)
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
	var body healthzResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.RunningAgents != 0 {
		t.Errorf("expected 0 agents, got %d", body.RunningAgents)
	}
}
