package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewDaemonClient_Defaults(t *testing.T) {
	dc := NewDaemonClient("", "", 0)
	if dc.baseURL != defaultDaemonBaseURL {
		t.Errorf("expected default base URL %q, got %q", defaultDaemonBaseURL, dc.baseURL)
	}
	if dc.httpClient.Timeout != defaultDaemonTimeout {
		t.Errorf("expected default timeout %v, got %v", defaultDaemonTimeout, dc.httpClient.Timeout)
	}
}

func TestNewDaemonClient_Custom(t *testing.T) {
	dc := NewDaemonClient("http://localhost:1234", "mytoken", 10*time.Second)
	if dc.baseURL != "http://localhost:1234" {
		t.Errorf("unexpected base URL: %q", dc.baseURL)
	}
	if dc.token != "mytoken" {
		t.Errorf("unexpected token: %q", dc.token)
	}
}

func TestDaemonClient_ListAgents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agents" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("X-Carrier-Actor") != "actor1" {
			t.Errorf("missing actor header")
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"agents": []map[string]interface{}{
				{"id": "a1", "installState": "installed"},
			},
		})
	}))
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "", 5*time.Second)
	agents, err := dc.ListAgents(context.Background(), "actor1", "req1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(agents) != 1 || agents[0].ID != "a1" {
		t.Errorf("unexpected agents: %+v", agents)
	}
}

func TestDaemonClient_ListAgents_DirectArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{"id": "a1"},
		})
	}))
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "", 5*time.Second)
	agents, err := dc.ListAgents(context.Background(), "actor", "req")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(agents))
	}
}

func TestDaemonClient_InstallAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agents/myagent/install" || r.Method != http.MethodPost {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "tok", 5*time.Second)
	err := dc.InstallAgent(context.Background(), "myagent", "actor", "req")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDaemonClient_StartAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "", 5*time.Second)
	err := dc.StartAgent(context.Background(), "a1", "actor", "req")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDaemonClient_StopAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "", 5*time.Second)
	err := dc.StopAgent(context.Background(), "a1", "actor", "req")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDaemonClient_GetStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"statuses": []map[string]interface{}{
				{"id": "a1", "health": "healthy"},
			},
		})
	}))
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "", 5*time.Second)
	statuses, err := dc.GetStatus(context.Background(), "a1", "actor", "req")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(statuses) != 1 {
		t.Errorf("expected 1 status, got %d", len(statuses))
	}
}

func TestDaemonClient_GetStatus_AllAgents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agents/status" {
			t.Errorf("expected /api/v1/agents/status, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"statuses": []interface{}{}})
	}))
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "", 5*time.Second)
	statuses, err := dc.GetStatus(context.Background(), "", "actor", "req")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(statuses) != 0 {
		t.Errorf("expected 0 statuses, got %d", len(statuses))
	}
}

func TestDaemonClient_GetLogs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"lines":     []string{"line1", "line2"},
			"truncated": true,
		})
	}))
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "", 5*time.Second)
	result, err := dc.GetLogs(context.Background(), "a1", 100, "actor", "req")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Lines) != 2 || !result.Truncated {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestDaemonClient_GetLogs_ClampsTail(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		json.NewEncoder(w).Encode(map[string]interface{}{"lines": []string{}, "truncated": false})
	}))
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "", 5*time.Second)
	// tail=0 should default to 200
	_, _ = dc.GetLogs(context.Background(), "a1", 0, "actor", "req")
	if gotPath != "/api/v1/agents/a1/logs?tail=200" {
		t.Errorf("tail=0: got path %q", gotPath)
	}

	// tail=5000 should clamp to 1000
	_, _ = dc.GetLogs(context.Background(), "a1", 5000, "actor", "req")
	if gotPath != "/api/v1/agents/a1/logs?tail=1000" {
		t.Errorf("tail=5000: got path %q", gotPath)
	}
}

func TestDaemonClient_GetMergedLogs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"lines": []string{"merged"}, "truncated": false})
	}))
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "", 5*time.Second)
	result, err := dc.GetMergedLogs(context.Background(), 100, "actor", "req")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Lines) != 1 || result.Lines[0] != "merged" {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestDaemonClient_UpgradeAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"agentId": "a1", "fromVersion": "1.0", "toVersion": "2.0",
		})
	}))
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "", 5*time.Second)
	result, err := dc.UpgradeAgent(context.Background(), "a1", "actor", "req")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.FromVersion != "1.0" || result.ToVersion != "2.0" {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestDaemonClient_DiagnoseAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"artifactRef": "/tmp/diag.tar.gz"})
	}))
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "", 5*time.Second)
	result, err := dc.DiagnoseAgent(context.Background(), "a1", "actor", "req")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ArtifactRef != "/tmp/diag.tar.gz" {
		t.Errorf("unexpected artifact ref: %q", result.ArtifactRef)
	}
}

func TestDaemonClient_CreateHandoff(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "h1", "agentId": "a1", "consent": true,
			"status": "pending", "createdAt": time.Now().Format(time.RFC3339),
		})
	}))
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "", 5*time.Second)
	result, err := dc.CreateHandoff(context.Background(), "a1", true, "actor", "req")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID != "h1" || result.Status != HandoffPending {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestDaemonClient_VerifyPairCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "", 5*time.Second)
	err := dc.VerifyPairCode(context.Background(), "abc", "actor", "req")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDaemonClient_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{"code": "E_AGENT_NOT_FOUND", "message": "agent not found"},
		})
	}))
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "", 5*time.Second)
	err := dc.InstallAgent(context.Background(), "missing", "actor", "req")
	if err == nil {
		t.Fatal("expected error")
	}
	de, ok := err.(*DaemonClientError)
	if !ok {
		t.Fatalf("expected DaemonClientError, got %T", err)
	}
	if de.Code != "E_AGENT_NOT_FOUND" {
		t.Errorf("expected E_AGENT_NOT_FOUND, got %s", de.Code)
	}
}

func TestDaemonClient_AuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "secret", 5*time.Second)
	_ = dc.VerifyPairCode(context.Background(), "code", "actor", "req")
	if gotAuth != "Bearer secret" {
		t.Errorf("expected 'Bearer secret', got %q", gotAuth)
	}
}

func TestDaemonClientError(t *testing.T) {
	e := &DaemonClientError{Code: "E_TEST", Message: "test msg"}
	if e.Error() != "E_TEST: test msg" {
		t.Errorf("unexpected error string: %q", e.Error())
	}
}

func TestIsRemoteDiagNotNeeded(t *testing.T) {
	if IsRemoteDiagNotNeeded(fmt.Errorf("generic")) {
		t.Error("generic error should not match")
	}
	if !IsRemoteDiagNotNeeded(&DaemonClientError{Code: "E_REMOTE_DIAG_NOT_NEEDED"}) {
		t.Error("should match E_REMOTE_DIAG_NOT_NEEDED")
	}
	if IsRemoteDiagNotNeeded(&DaemonClientError{Code: "E_OTHER"}) {
		t.Error("should not match E_OTHER")
	}
}

func TestDaemonClient_StatusToCode(t *testing.T) {
	dc := &DaemonClient{}
	tests := []struct {
		status int
		want   string
	}{
		{400, "E_USAGE"},
		{401, "E_SESSION_REQUIRED"},
		{403, "E_SESSION_REQUIRED"},
		{404, "E_AGENT_NOT_FOUND"},
		{500, "E_COMMAND_FAILED"},
	}
	for _, tc := range tests {
		got := dc.statusToCode(tc.status)
		if got != tc.want {
			t.Errorf("statusToCode(%d) = %q, want %q", tc.status, got, tc.want)
		}
	}
}
