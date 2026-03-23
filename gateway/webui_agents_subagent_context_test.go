package gateway

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleWebUIAgent_SubagentContextRequests(t *testing.T) {
	_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
		"GET /api/v1/agents/agent-alpha/subagents/subagent-1/context-requests": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"requests": []map[string]any{
					{"requestId": "ctx-1", "question": "Need timeline", "status": "pending"},
				},
			})
		},
		"POST /api/v1/agents/agent-alpha/subagents/subagent-1/context-requests/ctx-1/respond": func(w http.ResponseWriter, r *http.Request) {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body["summary"] != "timeline attached" {
				t.Fatalf("unexpected response body: %+v", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"requestId": "ctx-1",
				"status":    "fulfilled",
				"summary":   "timeline attached",
			})
		},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent-alpha/subagents/subagent-1/context-requests", nil)
	handleWebUIAgent(rec, req, "req-subagents-context-list", daemon)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"ctx-1"`) {
		t.Fatalf("expected context request list body, got %s", rec.Body.String())
	}

	body, _ := json.Marshal(map[string]any{"summary": "timeline attached"})
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent-alpha/subagents/subagent-1/context-requests/ctx-1/respond", bytes.NewReader(body))
	handleWebUIAgent(rec, req, "req-subagents-context-respond", daemon)
	if rec.Code != http.StatusOK {
		t.Fatalf("respond status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"timeline attached"`) {
		t.Fatalf("expected context request respond body, got %s", rec.Body.String())
	}
}
