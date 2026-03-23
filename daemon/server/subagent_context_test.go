package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"carrier/baseagent"
	"carrier/daemon/internal/api"
	"carrier/daemon/internal/ratelimit"
)

func TestAgentSubagentContextEndpoints(t *testing.T) {
	svc := newTestServiceWithAgent(t)
	ready := &atomic.Bool{}
	ready.Store(true)
	rt := &fakeBaseAgentRuntime{
		subagentContextRequests: []baseagent.DelegationContextRequest{
			{RequestID: "ctx-1", Question: "Need repo history", Status: baseagent.DelegationContextStatusPending},
		},
		subagentContextResponse: baseagent.DelegationContextResponse{
			RequestID: "ctx-1",
			Status:    baseagent.DelegationContextStatusFulfilled,
			Summary:   "history attached",
		},
	}
	mux := buildHTTPMuxWithBaseAgent(svc, rt, ready, api.NewPairingCodeStore(nil), ratelimit.New())

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/agents/test-agent/subagents/subagent-3/context-requests", nil)
	listRec := httptest.NewRecorder()
	mux.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected context request list 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	if rt.lastSubagentContextJobID != "subagent-3" {
		t.Fatalf("expected context request job forwarding, got %q", rt.lastSubagentContextJobID)
	}
	if !strings.Contains(listRec.Body.String(), `"ctx-1"`) {
		t.Fatalf("unexpected context request list body: %s", listRec.Body.String())
	}

	body, _ := json.Marshal(map[string]any{
		"status":  "fulfilled",
		"summary": "history attached",
	})
	respondReq := httptest.NewRequest(http.MethodPost, "/api/v1/agents/test-agent/subagents/subagent-3/context-requests/ctx-1/respond", bytes.NewReader(body))
	respondRec := httptest.NewRecorder()
	mux.ServeHTTP(respondRec, respondReq)
	if respondRec.Code != http.StatusOK {
		t.Fatalf("expected context request respond 200, got %d body=%s", respondRec.Code, respondRec.Body.String())
	}
	if rt.lastSubagentContextRequestID != "ctx-1" {
		t.Fatalf("expected request id forwarding, got %q", rt.lastSubagentContextRequestID)
	}
	if !strings.Contains(respondRec.Body.String(), `"history attached"`) {
		t.Fatalf("unexpected context request respond body: %s", respondRec.Body.String())
	}
}
