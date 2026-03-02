package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlePairingSessionsValidateParams(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.URL.RawQuery = ""
	rec := httptest.NewRecorder()
	handlePairingSessions(rec, req, "r1", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for nil store, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	handlePairingSessions(rec, req, "r2", &SessionStore{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing provider, got %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/?provider=foo", nil)
	rec = httptest.NewRecorder()
	handlePairingSessions(rec, req, "r3", &SessionStore{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unsupported provider, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandlePairingSessionsSummarizesSessions(t *testing.T) {
	s := NewSessionStore("", 0, nil)
	defer s.Stop()
	s.CreateSession("telegram", "chat-1")
	s.CreateSession("telegram", "chat-2")
	s.CreateSession("discord", "chat-3")
	// add an invalid/nil session record in internal map to ensure nil branch is skipped.
	s.mu.Lock()
	s.sessions["telegram:"] = nil
	s.mu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/?provider=%20telegram%20", nil)
	rec := httptest.NewRecorder()
	handlePairingSessions(rec, req, "r4", s)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		RequestID string                  `json:"requestId"`
		Result    string                  `json:"result"`
		Provider  string                  `json:"provider"`
		Sessions  []pairingSessionSummary `json:"sessions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response error: %v", err)
	}
	if resp.RequestID != "r4" || resp.Result != "ok" {
		t.Fatalf("unexpected response metadata: %#v", resp)
	}
	if resp.Provider != "telegram" {
		t.Fatalf("expected normalized provider telegram, got %q", resp.Provider)
	}
	if len(resp.Sessions) != 2 {
		t.Fatalf("expected 2 telegram sessions, got %d", len(resp.Sessions))
	}
}
