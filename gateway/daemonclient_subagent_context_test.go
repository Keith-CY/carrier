package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"carrier/baseagent"
)

func TestDaemonClient_SubagentContextRequests(t *testing.T) {
	srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agents/a1/subagents/subagent-1/context-requests":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"requests": []map[string]any{
					{"requestId": "ctx-1", "question": "Need logs", "status": "pending"},
				},
			})
		case "/api/v1/agents/a1/subagents/subagent-1/context-requests/ctx-1/respond":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body["summary"] != "logs attached" {
				t.Fatalf("unexpected respond body: %+v", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"requestId": "ctx-1",
				"status":    "fulfilled",
				"summary":   "logs attached",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "", 5*time.Second)
	requests, err := dc.GetAgentSubagentContextRequests(context.Background(), "a1", "subagent-1", "actor", "req")
	if err != nil {
		t.Fatalf("GetAgentSubagentContextRequests error: %v", err)
	}
	if len(requests) != 1 || requests[0].RequestID != "ctx-1" {
		t.Fatalf("unexpected requests: %+v", requests)
	}
	resolved, err := dc.RespondAgentSubagentContextRequest(context.Background(), "a1", "subagent-1", "ctx-1", baseagent.DelegationContextResponse{
		Summary: "logs attached",
	}, "actor", "req")
	if err != nil {
		t.Fatalf("RespondAgentSubagentContextRequest error: %v", err)
	}
	if resolved.RequestID != "ctx-1" || resolved.Summary != "logs attached" {
		t.Fatalf("unexpected resolved response: %+v", resolved)
	}
}
