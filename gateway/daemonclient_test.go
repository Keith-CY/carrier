package gateway

import (
	"carrier/baseagent"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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
	srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agents" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("X-Carrier-Actor") != "actor1" {
			t.Errorf("missing actor header")
		}
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"agents": []map[string]interface{}{
				{"id": "a1", "installState": "installed"},
			},
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
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
	srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode([]map[string]interface{}{
			{"id": "a1"},
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
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
	srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "", 5*time.Second)
	err := dc.StartAgent(context.Background(), "a1", "actor", "req")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDaemonClient_InstallAgentWithOptionsIsolation(t *testing.T) {
	var got map[string]interface{}
	srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agents/myagent/install" || r.Method != http.MethodPost {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode install body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "", 5*time.Second)
	if err := dc.InstallAgentWithOptions(context.Background(), "myagent", InstallAgentOptions{Isolation: true}, "actor", "req"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["isolation"] != true {
		t.Fatalf("expected isolation=true payload, got %#v", got)
	}
}

func TestDaemonClient_StartAgentWithOptionsIsolation(t *testing.T) {
	var got map[string]interface{}
	srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agents/a1/start" || r.Method != http.MethodPost {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode start body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "", 5*time.Second)
	if err := dc.StartAgentWithOptions(context.Background(), "a1", StartAgentOptions{Isolation: true}, "actor", "req"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["isolation"] != true {
		t.Fatalf("expected isolation=true payload, got %#v", got)
	}
}

func TestDaemonClient_StopAgent(t *testing.T) {
	srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "", 5*time.Second)
	err := dc.StopAgent(context.Background(), "a1", "actor", "req")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDaemonClient_UninstallAgent(t *testing.T) {
	srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agents/a1/uninstall" || r.Method != http.MethodPost {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "", 5*time.Second)
	err := dc.UninstallAgent(context.Background(), "a1", "actor", "req")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDaemonClient_GetStatus(t *testing.T) {
	srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"statuses": []map[string]interface{}{
				{"id": "a1", "health": "healthy"},
			},
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
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

func TestDaemonClient_GetStatus_SingleObject(t *testing.T) {
	srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"id":           "a1",
			"runtimeState": "running",
			"health":       "healthy",
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}))
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "", 5*time.Second)
	statuses, err := dc.GetStatus(context.Background(), "a1", "actor", "req")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].ID != "a1" || statuses[0].Runtime != "running" {
		t.Fatalf("unexpected status payload: %+v", statuses[0])
	}
}

func TestDaemonClient_GetStatus_AllAgents(t *testing.T) {
	srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agents/status" {
			t.Errorf("expected /api/v1/agents/status, got %s", r.URL.Path)
		}
		if err := json.NewEncoder(w).Encode(map[string]interface{}{"statuses": []interface{}{}}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
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

func TestDaemonClient_GetAgentCapabilities(t *testing.T) {
	srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agents/a1/capabilities" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"skills": []map[string]interface{}{
				{"name": "go-testing", "enabled": true},
			},
			"mcp": map[string]interface{}{
				"servers": []map[string]interface{}{
					{"name": "repo", "health": "healthy"},
				},
			},
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}))
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "", 5*time.Second)
	summary, err := dc.GetAgentCapabilities(context.Background(), "a1", "actor", "req")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(summary.Skills) != 1 || summary.Skills[0].Name != "go-testing" {
		t.Fatalf("unexpected capabilities: %+v", summary)
	}
}

func TestDaemonClient_SetAgentSkillEnabled(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if err := json.NewEncoder(w).Encode(map[string]any{
			"skills": []map[string]any{
				{"name": "go-testing", "enabled": false},
				{"name": "workspace-inspection", "enabled": false},
			},
			"skillSummary": map[string]any{
				"installedCount": 2,
				"enabledCount":   0,
				"disabledCount":  2,
			},
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "", 5*time.Second)
	summary, err := dc.SetAgentSkillEnabled(context.Background(), "a1", "go-testing", false, "actor", "req")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/api/v1/agents/a1/skills/go-testing" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
	if enabled, ok := gotBody["enabled"].(bool); !ok || enabled {
		t.Fatalf("unexpected toggle body: %+v", gotBody)
	}
	if summary.SkillSummary.DisabledCount != 2 || len(summary.Skills) != 2 || summary.Skills[0].Enabled {
		t.Fatalf("unexpected skill summary: %+v", summary)
	}
}

func TestDaemonClient_SetAgentMCPServerEnabled(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if err := json.NewEncoder(w).Encode(map[string]any{
			"mcp": map[string]any{
				"servers": []map[string]any{
					{"name": "repo", "health": "stopped", "enabled": false, "manageable": true, "visibleToolCount": 1, "hiddenToolCount": 0},
				},
				"visibleTools": []map[string]any{},
			},
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "", 5*time.Second)
	summary, err := dc.SetAgentMCPServerEnabled(context.Background(), "a1", "repo", false, "actor", "req")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/api/v1/agents/a1/mcp/repo" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
	if enabled, ok := gotBody["enabled"].(bool); !ok || enabled {
		t.Fatalf("unexpected toggle body: %+v", gotBody)
	}
	if len(summary.MCP.Servers) != 1 || summary.MCP.Servers[0].Enabled || summary.MCP.Servers[0].Health != "stopped" {
		t.Fatalf("unexpected mcp summary: %+v", summary)
	}
}

func TestDaemonClient_GetLogs(t *testing.T) {
	srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"lines":     []string{"line1", "line2"},
			"truncated": true,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
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

func TestDaemonClient_ChatAgent(t *testing.T) {
	var gotPath string
	var gotBody map[string]interface{}
	srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"agentId":   "openclaw",
			"sessionId": "sess-local-1",
			"message":   "hello local",
		})
	}))
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "", 5*time.Second)
	result, err := dc.ChatAgent(context.Background(), "openclaw", "openrouter", "hello", "sess-local-1", "flash", "google/gemini-2.0-flash-001", "actor", "req")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/api/v1/agents/openclaw/chat" {
		t.Fatalf("unexpected path %q", gotPath)
	}
	if gotBody["message"] != "hello" {
		t.Fatalf("unexpected message body: %#v", gotBody)
	}
	if gotBody["provider"] != "openrouter" {
		t.Fatalf("unexpected provider body: %#v", gotBody)
	}
	if gotBody["sessionId"] != "sess-local-1" {
		t.Fatalf("unexpected session body: %#v", gotBody)
	}
	if gotBody["modelAlias"] != "flash" {
		t.Fatalf("unexpected modelAlias body: %#v", gotBody)
	}
	if gotBody["model"] != "google/gemini-2.0-flash-001" {
		t.Fatalf("unexpected model body: %#v", gotBody)
	}
	if result.AgentID != "openclaw" || result.SessionID != "sess-local-1" || result.Message != "hello local" {
		t.Fatalf("unexpected chat result: %+v", result)
	}
}

func TestDaemonClient_DecomposeBaseAgent(t *testing.T) {
	var gotPath string
	var gotBody map[string]interface{}
	srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"tasks": []map[string]interface{}{
				{"id": "task-1", "input": "summarize logs"},
			},
		})
	}))
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "", 5*time.Second)
	tasks, err := dc.DecomposeBaseAgent(context.Background(), "analyze logs", "actor", "req")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/api/v1/base-agent/decompose" {
		t.Fatalf("unexpected path %q", gotPath)
	}
	if gotBody["goal"] != "analyze logs" {
		t.Fatalf("unexpected goal body: %#v", gotBody)
	}
	if len(tasks) != 1 || tasks[0].ID != "task-1" {
		t.Fatalf("unexpected tasks: %+v", tasks)
	}
}

func TestDaemonClient_DecomposeBaseAgentWithProvider(t *testing.T) {
	var gotBody map[string]interface{}
	srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"tasks": []map[string]interface{}{
				{"id": "task-1", "input": "summarize logs", "agentId": "zeroclaw"},
			},
		})
	}))
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "", 5*time.Second)
	tasks, err := dc.DecomposeBaseAgentWithProvider(context.Background(), "analyze logs", "openrouter", "actor", "req")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotBody["goal"] != "analyze logs" {
		t.Fatalf("unexpected goal body: %#v", gotBody)
	}
	if gotBody["provider"] != "openrouter" {
		t.Fatalf("unexpected provider body: %#v", gotBody)
	}
	if len(tasks) != 1 || tasks[0].ID != "task-1" {
		t.Fatalf("unexpected tasks: %+v", tasks)
	}
}

func TestDaemonClient_GetLogs_ClampsTail(t *testing.T) {
	var gotPath string
	srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		if err := json.NewEncoder(w).Encode(map[string]interface{}{"lines": []string{}, "truncated": false}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}))
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "", 5*time.Second)
	// tail=0 should default to 200
	if _, err := dc.GetLogs(context.Background(), "a1", 0, "actor", "req"); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/agents/a1/logs?tail=200" {
		t.Errorf("tail=0: got path %q", gotPath)
	}

	// tail=5000 should clamp to 1000
	if _, err := dc.GetLogs(context.Background(), "a1", 5000, "actor", "req"); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/agents/a1/logs?tail=1000" {
		t.Errorf("tail=5000: got path %q", gotPath)
	}
}

func TestDaemonClient_GetMergedLogs(t *testing.T) {
	srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]interface{}{"lines": []string{"merged"}, "truncated": false}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
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
	srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"agentId": "a1", "fromVersion": "1.0", "toVersion": "2.0",
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
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
	srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]interface{}{"artifactRef": "/tmp/diag.tar.gz"}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
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
	srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "h1", "agentId": "a1", "consent": true,
			"status": "pending", "createdAt": time.Now().Format(time.RFC3339),
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
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
	srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{"code": "E_AGENT_NOT_FOUND", "message": "agent not found"},
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
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

func TestDaemonClient_ErrorResponse_LegacyStringPairCodeInvalid(t *testing.T) {
	srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "pairing code is invalid or expired",
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}))
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "", 5*time.Second)
	err := dc.VerifyPairCode(context.Background(), "bad", "actor", "req")
	if err == nil {
		t.Fatal("expected error")
	}
	de, ok := err.(*DaemonClientError)
	if !ok {
		t.Fatalf("expected DaemonClientError, got %T", err)
	}
	if de.Code != "E_PAIR_CODE_INVALID" {
		t.Fatalf("expected E_PAIR_CODE_INVALID, got %q (message=%q)", de.Code, de.Message)
	}
}

func TestNormalizeDaemonErrorCode(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		message string
		want    string
	}{
		{
			name:    "pair invalid from usage",
			code:    "E_USAGE",
			message: "pairing code is invalid or expired",
			want:    "E_PAIR_CODE_INVALID",
		},
		{
			name:    "pair invalid from fallback",
			code:    "E_COMMAND_FAILED",
			message: "pairing code is invalid or expired",
			want:    "E_PAIR_CODE_INVALID",
		},
		{
			name:    "keeps specific code",
			code:    "E_AGENT_NOT_FOUND",
			message: "agent not found",
			want:    "E_AGENT_NOT_FOUND",
		},
		{
			name:    "empty code fallback",
			code:    "",
			message: "unknown",
			want:    "E_COMMAND_FAILED",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeDaemonErrorCode(tc.code, tc.message)
			if got != tc.want {
				t.Fatalf("normalizeDaemonErrorCode(%q,%q)=%q want %q", tc.code, tc.message, got, tc.want)
			}
		})
	}
}

func TestDaemonClient_AuthHeader(t *testing.T) {
	var gotAuth string
	srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "secret", 5*time.Second)
	if err := dc.VerifyPairCode(context.Background(), "code", "actor", "req"); err != nil {
		t.Fatal(err)
	}
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
		t.Run(fmt.Sprintf("status_%d", tc.status), func(t *testing.T) {
			got := dc.statusToCode(tc.status)
			if got != tc.want {
				t.Errorf("statusToCode(%d) = %q, want %q", tc.status, got, tc.want)
			}
		})
	}
}

func TestDaemonClient_GetStatus_DirectArrayAndInvalidPayload(t *testing.T) {
	t.Run("direct array response", func(t *testing.T) {
		srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`[{"id":"a2","health":"healthy"}]`))
		}))
		defer srv.Close()

		dc := NewDaemonClient(srv.URL, "", 5*time.Second)
		statuses, err := dc.GetStatus(context.Background(), "a2", "actor", "req")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(statuses) != 1 || statuses[0].ID != "a2" {
			t.Fatalf("unexpected statuses: %+v", statuses)
		}
	})

	t.Run("invalid payload returns parse error", func(t *testing.T) {
		srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{invalid`))
		}))
		defer srv.Close()

		dc := NewDaemonClient(srv.URL, "", 5*time.Second)
		_, err := dc.GetStatus(context.Background(), "a2", "actor", "req")
		if err == nil || !strings.Contains(err.Error(), "status response") {
			t.Fatalf("expected status parse error, got %v", err)
		}
	})
}

func TestDaemonClient_GetMergedLogs_ClampsTail(t *testing.T) {
	var gotPath string
	srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		_, _ = w.Write([]byte(`{"lines":[],"truncated":false}`))
	}))
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "", 5*time.Second)
	if _, err := dc.GetMergedLogs(context.Background(), 0, "actor", "req"); err != nil {
		t.Fatalf("tail=0 unexpected error: %v", err)
	}
	if gotPath != "/api/v1/logs?tail=200" {
		t.Fatalf("tail=0 path = %q, want %q", gotPath, "/api/v1/logs?tail=200")
	}
	if _, err := dc.GetMergedLogs(context.Background(), 9999, "actor", "req"); err != nil {
		t.Fatalf("tail=9999 unexpected error: %v", err)
	}
	if gotPath != "/api/v1/logs?tail=1000" {
		t.Fatalf("tail=9999 path = %q, want %q", gotPath, "/api/v1/logs?tail=1000")
	}
}

func TestDaemonClient_ScheduleCronJob_SendsOnlyAcceptedFields(t *testing.T) {
	var rawBody map[string]any
	srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/base-agent/cron/schedule" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&rawBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		_, _ = w.Write([]byte(`{"id":"cron-1","agentId":"picoclaw","prompt":"check launcher","lastResult":"scheduled"}`))
	}))
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "", 5*time.Second)
	resp, err := dc.ScheduleCronJob(context.Background(), baseagent.CronJob{
		AgentID:    "picoclaw",
		SessionKey: "openrouter:cron-ui-smoke",
		Prompt:     "check launcher",
		NextRunAt:  time.Now().UTC().Add(time.Hour),
		LastResult: "should-not-send",
	}, "actor", "req")
	if err != nil {
		t.Fatalf("ScheduleCronJob error: %v", err)
	}
	if resp == nil || resp.ID != "cron-1" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	for _, forbidden := range []string{"lastResult", "lastRunAt", "cancelledAt"} {
		if _, exists := rawBody[forbidden]; exists {
			t.Fatalf("unexpected field %q in payload: %#v", forbidden, rawBody)
		}
	}
	if got := strings.TrimSpace(rawBody["sessionKey"].(string)); got != "openrouter:cron-ui-smoke" {
		t.Fatalf("sessionKey=%q want openrouter:cron-ui-smoke", got)
	}
	if got := strings.TrimSpace(rawBody["prompt"].(string)); got != "check launcher" {
		t.Fatalf("prompt=%q want check launcher", got)
	}
}

func TestDaemonClient_UpgradeDiagnoseAndChatParseErrors(t *testing.T) {
	tests := []struct {
		name string
		path string
		run  func(*DaemonClient) error
		want string
	}{
		{
			name: "upgrade parse error",
			path: "/api/v1/agents/a1/upgrade",
			run: func(c *DaemonClient) error {
				_, err := c.UpgradeAgent(context.Background(), "a1", "actor", "req")
				return err
			},
			want: "upgrade response",
		},
		{
			name: "diagnose parse error",
			path: "/api/v1/agents/a1/diagnose",
			run: func(c *DaemonClient) error {
				_, err := c.DiagnoseAgent(context.Background(), "a1", "actor", "req")
				return err
			},
			want: "diagnose response",
		},
		{
			name: "chat parse error",
			path: "/api/v1/base-agent/chat",
			run: func(c *DaemonClient) error {
				_, err := c.ChatBaseAgent(context.Background(), "openai", "chat-1", "req", "hello", nil, "actor")
				return err
			},
			want: "base-agent chat response",
		},
		{
			name: "decompose parse error",
			path: "/api/v1/base-agent/decompose",
			run: func(c *DaemonClient) error {
				_, err := c.DecomposeBaseAgent(context.Background(), "goal", "actor", "req")
				return err
			},
			want: "base-agent decompose response",
		},
		{
			name: "agent chat parse error",
			path: "/api/v1/agents/openclaw/chat",
			run: func(c *DaemonClient) error {
				_, err := c.ChatAgent(context.Background(), "openclaw", "openrouter", "hello", "sess-1", "", "", "actor", "req")
				return err
			},
			want: "agent chat response",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path {
					t.Fatalf("unexpected path: %s", r.URL.Path)
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{invalid`))
			}))
			defer srv.Close()

			dc := NewDaemonClient(srv.URL, "", 5*time.Second)
			err := tc.run(dc)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestDaemonClient_RequestMarshalError(t *testing.T) {
	dc := NewDaemonClient("http://127.0.0.1:1", "", 5*time.Second)
	_, err := dc.request(context.Background(), http.MethodPost, "/api/v1/test", map[string]interface{}{"bad": make(chan int)}, "actor", "req")
	if err == nil || !strings.Contains(err.Error(), "marshal request") {
		t.Fatalf("expected marshal error, got %v", err)
	}
}
