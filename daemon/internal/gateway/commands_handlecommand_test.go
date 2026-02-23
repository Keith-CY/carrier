package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newMockDaemon creates an httptest.Server that mimics the daemon API.
// The handler map maps "METHOD /path" → handler func.
func newMockDaemon(handlers map[string]http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		// Try exact match first
		if h, ok := handlers[key]; ok {
			h(w, r)
			return
		}
		// Try prefix match for paths with query strings
		for k, h := range handlers {
			parts := strings.SplitN(k, " ", 2)
			if len(parts) == 2 && r.Method == parts[0] && strings.HasPrefix(r.URL.Path, parts[1]) {
				h(w, r)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{"code": "E_AGENT_NOT_FOUND", "message": "not found"},
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}))
}

func setupTestEnv(t *testing.T, handlers map[string]http.HandlerFunc) (*httptest.Server, *DaemonClient, *SessionStore, *DownloadStore, *OnboardStore) {
	t.Helper()
	srv := newMockDaemon(handlers)
	dc := NewDaemonClient(srv.URL, "test-token", 5*time.Second)
	sessions := NewSessionStore("", 0, nil)
	t.Cleanup(sessions.Stop)
	downloads := NewDownloadStore("", nil)
	onboard := NewOnboardStore()
	return srv, dc, sessions, downloads, onboard
}

func pairAndGetSession(sessions *SessionStore, provider, chatID string) string {
	s := sessions.CreateSession(provider, chatID)
	return s.SessionToken
}

func TestHandleCommand_Pair_Success(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, map[string]http.HandlerFunc{
		"POST /api/v1/pairing/verify-consume": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"code":"abc","consumed":true}`)
		},
	})
	defer srv.Close()

	cmd := &GatewayCommand{
		Provider: "telegram", ChatID: "123", RequestID: "r1",
		Name: CmdPair, Args: []string{"abc"},
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.Result != "ok" {
		t.Fatalf("expected ok, got %s: %s", resp.Result, resp.Message)
	}
	if resp.SessionToken == "" {
		t.Error("expected session token")
	}
}

func TestHandleCommand_Pair_NoArgs(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, nil)
	defer srv.Close()

	cmd := &GatewayCommand{
		Provider: "telegram", ChatID: "123", RequestID: "r1",
		Name: CmdPair, Args: []string{},
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.Result != "error" {
		t.Fatalf("expected error, got %s", resp.Result)
	}
	if resp.ErrorCode != "E_USAGE" {
		t.Errorf("expected E_USAGE, got %s", resp.ErrorCode)
	}
}

func TestHandleCommand_Pair_DaemonError(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, map[string]http.HandlerFunc{
		"POST /api/v1/pairing/verify-consume": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":{"code":"E_USAGE","message":"invalid code"}}`)
		},
	})
	defer srv.Close()

	cmd := &GatewayCommand{
		Provider: "telegram", ChatID: "123", RequestID: "r1",
		Name: CmdPair, Args: []string{"bad"},
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.Result != "error" {
		t.Fatalf("expected error, got %s", resp.Result)
	}
}

func TestHandleCommand_Pair_LegacyPairCodeInvalid(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, map[string]http.HandlerFunc{
		"POST /api/v1/pairing/verify-consume": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"pairing code is invalid or expired"}`)
		},
	})
	defer srv.Close()

	cmd := &GatewayCommand{
		Provider: "telegram", ChatID: "123", RequestID: "r1",
		Name: CmdPair, Args: []string{"expired"},
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.Result != "error" {
		t.Fatalf("expected error, got %s", resp.Result)
	}
	if resp.ErrorCode != "E_PAIR_CODE_INVALID" {
		t.Fatalf("errorCode = %q, want %q", resp.ErrorCode, "E_PAIR_CODE_INVALID")
	}
	if !strings.Contains(resp.Message, "pair code is invalid or expired") {
		t.Fatalf("message = %q, want pair code guidance", resp.Message)
	}
}

func TestHandleCommand_RequiresSession(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, nil)
	defer srv.Close()

	cmd := &GatewayCommand{
		Provider: "telegram", ChatID: "123", RequestID: "r1",
		Name: CmdAgents,
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.ErrorCode != "E_SESSION_REQUIRED" {
		t.Errorf("expected E_SESSION_REQUIRED, got %s", resp.ErrorCode)
	}
}

func TestHandleCommand_RequiresSessionToken(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, nil)
	defer srv.Close()

	pairAndGetSession(sessions, "telegram", "123")
	cmd := &GatewayCommand{
		Provider: "telegram", ChatID: "123", RequestID: "r1",
		Name: CmdAgents, SessionToken: "",
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.ErrorCode != "E_SESSION_TOKEN_MISSING" {
		t.Errorf("expected E_SESSION_TOKEN_MISSING, got %s", resp.ErrorCode)
	}
}

func TestHandleCommand_InvalidSessionToken(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, nil)
	defer srv.Close()

	pairAndGetSession(sessions, "telegram", "123")
	cmd := &GatewayCommand{
		Provider: "telegram", ChatID: "123", RequestID: "r1",
		Name: CmdAgents, SessionToken: "wrong-token",
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.ErrorCode != "E_SESSION_TOKEN_INVALID" {
		t.Errorf("expected E_SESSION_TOKEN_INVALID, got %s", resp.ErrorCode)
	}
}

func TestHandleCommand_Chat_Success(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, map[string]http.HandlerFunc{
		"POST /api/v1/base-agent/chat": func(w http.ResponseWriter, r *http.Request) {
			var req map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if req["provider"] != "telegram" {
				t.Fatalf("provider = %v, want telegram", req["provider"])
			}
			if req["chatId"] != "123" {
				t.Fatalf("chatId = %v, want 123", req["chatId"])
			}
			if req["message"] != "hello from terminal" {
				t.Fatalf("message = %v, want %q", req["message"], "hello from terminal")
			}
			if err := json.NewEncoder(w).Encode(map[string]interface{}{
				"message": "hello back",
			}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		},
	})
	defer srv.Close()

	tok := pairAndGetSession(sessions, "telegram", "123")
	cmd := &GatewayCommand{
		Provider: "telegram", ChatID: "123", RequestID: "r-chat",
		Name: CmdChat, Args: []string{"hello", "from", "terminal"}, SessionToken: tok,
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.Result != "ok" {
		t.Fatalf("expected ok, got %s: %s", resp.Result, resp.Message)
	}
	if resp.Message != "hello back" {
		t.Fatalf("message = %q, want %q", resp.Message, "hello back")
	}
}

func TestHandleCommand_Chat_NoArgs(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, nil)
	defer srv.Close()

	tok := pairAndGetSession(sessions, "telegram", "123")
	cmd := &GatewayCommand{
		Provider: "telegram", ChatID: "123", RequestID: "r-chat",
		Name: CmdChat, Args: []string{}, SessionToken: tok,
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.Result != "error" {
		t.Fatalf("expected error, got %s", resp.Result)
	}
	if resp.ErrorCode != "E_USAGE" {
		t.Fatalf("errorCode = %q, want %q", resp.ErrorCode, "E_USAGE")
	}
}

func TestHandleCommand_Agents_Success(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, map[string]http.HandlerFunc{
		"GET /api/v1/agents": func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewEncoder(w).Encode(map[string]interface{}{
				"agents": []map[string]interface{}{
					{"id": "a1", "installState": "installed"},
					{"id": "a2", "installState": "available"},
				},
			}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		},
	})
	defer srv.Close()

	tok := pairAndGetSession(sessions, "telegram", "123")
	cmd := &GatewayCommand{
		Provider: "telegram", ChatID: "123", RequestID: "r1",
		Name: CmdAgents, SessionToken: tok,
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.Result != "ok" {
		t.Fatalf("expected ok, got %s: %s", resp.Result, resp.Message)
	}
	if !strings.Contains(resp.Message, "2 agents") {
		t.Errorf("expected '2 agents' in message, got %q", resp.Message)
	}
	if !strings.Contains(resp.Message, "1 installed") {
		t.Errorf("expected '1 installed' in message, got %q", resp.Message)
	}
	if !strings.Contains(resp.Message, "a1") || !strings.Contains(resp.Message, "a2") {
		t.Errorf("expected detailed agent list in message, got %q", resp.Message)
	}
}

func TestHandleCommand_Install_GuiOnly(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, nil)
	defer srv.Close()

	tok := pairAndGetSession(sessions, "telegram", "123")
	cmd := &GatewayCommand{
		Provider: "telegram", ChatID: "123", RequestID: "r1",
		Name: CmdInstall, Args: []string{"myagent"}, SessionToken: tok,
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.Result != "error" {
		t.Fatalf("expected error, got %s: %s", resp.Result, resp.Message)
	}
	if resp.ErrorCode != "E_INSTALL_GUI_ONLY" {
		t.Fatalf("expected E_INSTALL_GUI_ONLY, got %s", resp.ErrorCode)
	}
	if !strings.Contains(resp.Message, "Carrier GUI") {
		t.Fatalf("expected GUI guidance, got: %q", resp.Message)
	}
}

func TestHandleCommand_Install_NoArgs(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, nil)
	defer srv.Close()

	tok := pairAndGetSession(sessions, "telegram", "123")
	cmd := &GatewayCommand{
		Provider: "telegram", ChatID: "123", RequestID: "r1",
		Name: CmdInstall, Args: []string{}, SessionToken: tok,
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.ErrorCode != "E_INSTALL_GUI_ONLY" {
		t.Errorf("expected E_INSTALL_GUI_ONLY, got %s", resp.ErrorCode)
	}
	if !strings.Contains(resp.Message, "Carrier GUI") {
		t.Fatalf("expected GUI guidance, got: %q", resp.Message)
	}
}

func TestHandleCommand_Onboard_GuiOnly(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, nil)
	defer srv.Close()

	tok := pairAndGetSession(sessions, "telegram", "123")
	cmd := &GatewayCommand{
		Provider: "telegram", ChatID: "123", RequestID: "r1",
		Name: CmdOnboard, Args: []string{}, SessionToken: tok,
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.Result != "error" {
		t.Fatalf("expected error, got %s: %s", resp.Result, resp.Message)
	}
	if resp.ErrorCode != "E_ONBOARD_GUI_ONLY" {
		t.Fatalf("expected E_ONBOARD_GUI_ONLY, got %s", resp.ErrorCode)
	}
	if !strings.Contains(resp.Message, "Carrier GUI") {
		t.Fatalf("expected GUI guidance, got: %q", resp.Message)
	}
}

func TestHandleCommand_Uninstall_Success(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, map[string]http.HandlerFunc{
		"POST /api/v1/agents/myagent/uninstall": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"status":"uninstalled"}`)
		},
	})
	defer srv.Close()

	tok := pairAndGetSession(sessions, "telegram", "123")
	cmd := &GatewayCommand{
		Provider: "telegram", ChatID: "123", RequestID: "r1",
		Name: CmdUninstall, Args: []string{"myagent"}, SessionToken: tok,
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.Result != "ok" {
		t.Fatalf("expected ok, got %s: %s", resp.Result, resp.Message)
	}
}

func TestHandleCommand_Uninstall_NoArgs(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, nil)
	defer srv.Close()

	tok := pairAndGetSession(sessions, "telegram", "123")
	cmd := &GatewayCommand{
		Provider: "telegram", ChatID: "123", RequestID: "r1",
		Name: CmdUninstall, Args: []string{}, SessionToken: tok,
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.ErrorCode != "E_USAGE" {
		t.Errorf("expected E_USAGE, got %s", resp.ErrorCode)
	}
}

func TestHandleCommand_Start_Success(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, map[string]http.HandlerFunc{
		"POST /api/v1/agents/myagent/start": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"status":"started"}`)
		},
	})
	defer srv.Close()

	tok := pairAndGetSession(sessions, "telegram", "123")
	cmd := &GatewayCommand{
		Provider: "telegram", ChatID: "123", RequestID: "r1",
		Name: CmdStart, Args: []string{"myagent"}, SessionToken: tok,
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.Result != "ok" {
		t.Fatalf("expected ok, got %s: %s", resp.Result, resp.Message)
	}
}

func TestHandleCommand_Start_NoArgs(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, nil)
	defer srv.Close()

	tok := pairAndGetSession(sessions, "telegram", "123")
	cmd := &GatewayCommand{
		Provider: "telegram", ChatID: "123", RequestID: "r1",
		Name: CmdStart, Args: []string{}, SessionToken: tok,
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.ErrorCode != "E_USAGE" {
		t.Errorf("expected E_USAGE, got %s", resp.ErrorCode)
	}
}

func TestHandleCommand_Stop_Success(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, map[string]http.HandlerFunc{
		"POST /api/v1/agents/myagent/stop": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"status":"stopped"}`)
		},
	})
	defer srv.Close()

	tok := pairAndGetSession(sessions, "telegram", "123")
	cmd := &GatewayCommand{
		Provider: "telegram", ChatID: "123", RequestID: "r1",
		Name: CmdStop, Args: []string{"myagent"}, SessionToken: tok,
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.Result != "ok" {
		t.Fatalf("expected ok, got %s: %s", resp.Result, resp.Message)
	}
}

func TestHandleCommand_Stop_NoArgs(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, nil)
	defer srv.Close()

	tok := pairAndGetSession(sessions, "telegram", "123")
	cmd := &GatewayCommand{
		Provider: "telegram", ChatID: "123", RequestID: "r1",
		Name: CmdStop, Args: []string{}, SessionToken: tok,
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.ErrorCode != "E_USAGE" {
		t.Errorf("expected E_USAGE, got %s", resp.ErrorCode)
	}
}

func TestHandleCommand_Status_AllAgents(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, map[string]http.HandlerFunc{
		"GET /api/v1/agents/status": func(w http.ResponseWriter, r *http.Request) {
			started := time.Now().Add(-1 * time.Hour).Format(time.RFC3339Nano)
			if err := json.NewEncoder(w).Encode(map[string]interface{}{
				"statuses": []map[string]interface{}{
					{"id": "a1", "health": "healthy", "runtimeState": "running", "version": "1.0", "ports": []int{8080}, "restartCount": 0, "startedAt": started},
				},
			}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		},
	})
	defer srv.Close()

	tok := pairAndGetSession(sessions, "telegram", "123")
	cmd := &GatewayCommand{
		Provider: "telegram", ChatID: "123", RequestID: "r1",
		Name: CmdStatus, Args: []string{}, SessionToken: tok,
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.Result != "ok" {
		t.Fatalf("expected ok, got %s: %s", resp.Result, resp.Message)
	}
	if !strings.Contains(resp.Message, "a1") {
		t.Errorf("expected agent id in message, got %q", resp.Message)
	}
}

func TestHandleCommand_Status_SingleAgent(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, map[string]http.HandlerFunc{
		"GET /api/v1/agents/a1/status": func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewEncoder(w).Encode(map[string]interface{}{
				"statuses": []map[string]interface{}{
					{"id": "a1", "health": "healthy", "runtimeState": "stopped", "version": "1.0", "restartCount": 2},
				},
			}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		},
	})
	defer srv.Close()

	tok := pairAndGetSession(sessions, "telegram", "123")
	cmd := &GatewayCommand{
		Provider: "telegram", ChatID: "123", RequestID: "r1",
		Name: CmdStatus, Args: []string{"a1"}, SessionToken: tok,
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.Result != "ok" {
		t.Fatalf("expected ok, got %s: %s", resp.Result, resp.Message)
	}
}

func TestHandleCommand_Status_Empty(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, map[string]http.HandlerFunc{
		"GET /api/v1/agents/status": func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewEncoder(w).Encode(map[string]interface{}{"statuses": []interface{}{}}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		},
	})
	defer srv.Close()

	tok := pairAndGetSession(sessions, "telegram", "123")
	cmd := &GatewayCommand{
		Provider: "telegram", ChatID: "123", RequestID: "r1",
		Name: CmdStatus, Args: []string{}, SessionToken: tok,
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.Result != "ok" {
		t.Fatalf("expected ok, got %s: %s", resp.Result, resp.Message)
	}
	if !strings.Contains(resp.Message, "no agent status") {
		t.Errorf("expected 'no agent status' in message, got %q", resp.Message)
	}
}

func TestHandleCommand_Logs_WithAgent(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, map[string]http.HandlerFunc{
		"GET /api/v1/agents/a1/logs": func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewEncoder(w).Encode(map[string]interface{}{
				"lines":     []string{"line1", "line2"},
				"truncated": false,
			}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		},
	})
	defer srv.Close()

	tok := pairAndGetSession(sessions, "telegram", "123")
	cmd := &GatewayCommand{
		Provider: "telegram", ChatID: "123", RequestID: "r1",
		Name: CmdLogs, Args: []string{"a1"}, SessionToken: tok,
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.Result != "ok" {
		t.Fatalf("expected ok, got %s: %s", resp.Result, resp.Message)
	}
	if !strings.Contains(resp.Message, "2 log lines") {
		t.Errorf("expected '2 log lines' in message, got %q", resp.Message)
	}
}

func TestHandleCommand_Logs_MergedWithTail(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, map[string]http.HandlerFunc{
		"GET /api/v1/logs": func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewEncoder(w).Encode(map[string]interface{}{
				"lines":     []string{},
				"truncated": false,
			}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		},
	})
	defer srv.Close()

	tok := pairAndGetSession(sessions, "telegram", "123")
	cmd := &GatewayCommand{
		Provider: "telegram", ChatID: "123", RequestID: "r1",
		Name: CmdLogs, Args: []string{"50"}, SessionToken: tok,
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.Result != "ok" {
		t.Fatalf("expected ok, got %s: %s", resp.Result, resp.Message)
	}
	if !strings.Contains(resp.Message, "no logs") {
		t.Errorf("expected 'no logs' in message, got %q", resp.Message)
	}
}

func TestHandleCommand_Logs_MergedDefault(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, map[string]http.HandlerFunc{
		"GET /api/v1/logs": func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewEncoder(w).Encode(map[string]interface{}{
				"lines":     []string{"hello"},
				"truncated": false,
			}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		},
	})
	defer srv.Close()

	tok := pairAndGetSession(sessions, "telegram", "123")
	cmd := &GatewayCommand{
		Provider: "telegram", ChatID: "123", RequestID: "r1",
		Name: CmdLogs, Args: []string{}, SessionToken: tok,
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.Result != "ok" {
		t.Fatalf("expected ok, got %s: %s", resp.Result, resp.Message)
	}
}

func TestHandleCommand_Upgrade_Success(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, map[string]http.HandlerFunc{
		"POST /api/v1/agents/a1/upgrade": func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewEncoder(w).Encode(map[string]interface{}{
				"agentId":      "a1",
				"fromVersion":  "1.0",
				"toVersion":    "2.0",
				"backupPath":   "/tmp/backup",
				"rollbackHint": "run rollback",
			}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		},
	})
	defer srv.Close()

	tok := pairAndGetSession(sessions, "telegram", "123")
	cmd := &GatewayCommand{
		Provider: "telegram", ChatID: "123", RequestID: "r1",
		Name: CmdUpgrade, Args: []string{"a1"}, SessionToken: tok,
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.Result != "ok" {
		t.Fatalf("expected ok, got %s: %s", resp.Result, resp.Message)
	}
	if !strings.Contains(resp.Message, "1.0 -> 2.0") {
		t.Errorf("expected version info in message, got %q", resp.Message)
	}
	if !strings.Contains(resp.Message, "backup") {
		t.Errorf("expected backup info in message, got %q", resp.Message)
	}
}

func TestHandleCommand_Upgrade_NoArgs(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, nil)
	defer srv.Close()

	tok := pairAndGetSession(sessions, "telegram", "123")
	cmd := &GatewayCommand{
		Provider: "telegram", ChatID: "123", RequestID: "r1",
		Name: CmdUpgrade, Args: []string{}, SessionToken: tok,
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.ErrorCode != "E_USAGE" {
		t.Errorf("expected E_USAGE, got %s", resp.ErrorCode)
	}
}

func TestHandleCommand_Diagnose_Success(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, map[string]http.HandlerFunc{
		"POST /api/v1/agents/a1/diagnose": func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewEncoder(w).Encode(map[string]interface{}{
				"artifactRef": "/tmp/diag.tar.gz",
			}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		},
	})
	defer srv.Close()

	tok := pairAndGetSession(sessions, "telegram", "123")
	cmd := &GatewayCommand{
		Provider: "telegram", ChatID: "123", RequestID: "r1",
		Name: CmdDiagnose, Args: []string{"a1"}, SessionToken: tok,
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.Result != "ok" {
		t.Fatalf("expected ok, got %s: %s", resp.Result, resp.Message)
	}
}

func TestHandleCommand_Diagnose_NoArgs(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, nil)
	defer srv.Close()

	tok := pairAndGetSession(sessions, "telegram", "123")
	cmd := &GatewayCommand{
		Provider: "telegram", ChatID: "123", RequestID: "r1",
		Name: CmdDiagnose, Args: []string{}, SessionToken: tok,
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.ErrorCode != "E_USAGE" {
		t.Errorf("expected E_USAGE, got %s", resp.ErrorCode)
	}
}

func TestHandleCommand_DiagnoseConsent_Success(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, map[string]http.HandlerFunc{
		"POST /api/v1/diagnosis/handoffs": func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewEncoder(w).Encode(map[string]interface{}{
				"id":          "h1",
				"agentId":     "a1",
				"consent":     true,
				"artifactRef": "/tmp/art.tar.gz",
				"status":      "pending",
				"createdAt":   time.Now().Format(time.RFC3339),
			}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		},
	})
	defer srv.Close()

	tok := pairAndGetSession(sessions, "telegram", "123")
	cmd := &GatewayCommand{
		Provider: "telegram", ChatID: "123", RequestID: "r1",
		Name: CmdDiagnoseConsent, Args: []string{"a1", "yes"}, SessionToken: tok,
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.Result != "ok" {
		t.Fatalf("expected ok, got %s: %s", resp.Result, resp.Message)
	}
	if resp.HandoffID != "h1" {
		t.Errorf("expected handoff id h1, got %q", resp.HandoffID)
	}
}

func TestHandleCommand_DiagnoseConsent_NoArgs(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, nil)
	defer srv.Close()

	tok := pairAndGetSession(sessions, "telegram", "123")
	cmd := &GatewayCommand{
		Provider: "telegram", ChatID: "123", RequestID: "r1",
		Name: CmdDiagnoseConsent, Args: []string{}, SessionToken: tok,
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.ErrorCode != "E_USAGE" {
		t.Errorf("expected E_USAGE, got %s", resp.ErrorCode)
	}
}

func TestHandleCommand_DiagnoseConsent_InvalidConsent(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, nil)
	defer srv.Close()

	tok := pairAndGetSession(sessions, "telegram", "123")
	cmd := &GatewayCommand{
		Provider: "telegram", ChatID: "123", RequestID: "r1",
		Name: CmdDiagnoseConsent, Args: []string{"a1", "maybe"}, SessionToken: tok,
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.ErrorCode != "E_CONSENT_FLAG_INVALID" {
		t.Errorf("expected E_CONSENT_FLAG_INVALID, got %s", resp.ErrorCode)
	}
}

func TestHandleCommand_DiagnoseConsent_RemoteDiagNotNeeded(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, map[string]http.HandlerFunc{
		"POST /api/v1/diagnosis/handoffs": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			if err := json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{"code": "E_REMOTE_DIAG_NOT_NEEDED", "message": "not needed"},
			}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		},
	})
	defer srv.Close()

	tok := pairAndGetSession(sessions, "telegram", "123")
	cmd := &GatewayCommand{
		Provider: "telegram", ChatID: "123", RequestID: "r1",
		Name: CmdDiagnoseConsent, Args: []string{"a1", "yes"}, SessionToken: tok,
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.ErrorCode != "E_REMOTE_DIAG_NOT_NEEDED" {
		t.Errorf("expected E_REMOTE_DIAG_NOT_NEEDED, got %s", resp.ErrorCode)
	}
}

func TestHandleCommand_RateLimited(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, map[string]http.HandlerFunc{
		"GET /api/v1/agents": func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewEncoder(w).Encode(map[string]interface{}{"agents": []interface{}{}}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		},
	})
	defer srv.Close()

	// Create a very restrictive rate limiter
	rl := NewGatewayRateLimiter(1, 100, 1*time.Minute, nil)
	tok := pairAndGetSession(sessions, "telegram", "123")

	cmd := &GatewayCommand{
		Provider: "telegram", ChatID: "123", RequestID: "r1",
		Name: CmdAgents, SessionToken: tok,
	}
	// First should pass
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, rl, onboard)
	if resp.Result != "ok" {
		t.Fatalf("first request should pass, got %s: %s", resp.Result, resp.Message)
	}
	// Second should be rate limited
	cmd.RequestID = "r2"
	resp = HandleCommand(context.Background(), cmd, dc, sessions, downloads, rl, onboard)
	if resp.Result != "error" {
		t.Fatalf("second request should be rate limited, got %s", resp.Result)
	}
}

func TestSafeHandleCommand_ParseError(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, nil)
	defer srv.Close()

	resp := SafeHandleCommand(context.Background(), "bad input", dc, sessions, downloads, nil, onboard)
	if resp.Result != "error" {
		t.Fatalf("expected error, got %s", resp.Result)
	}
	if resp.ErrorCode != "E_PARSE" {
		t.Errorf("expected E_PARSE, got %s", resp.ErrorCode)
	}
}

func TestSafeHandleCommand_Success(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, map[string]http.HandlerFunc{
		"POST /api/v1/pairing/verify-consume": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"code":"abc","consumed":true}`)
		},
	})
	defer srv.Close()

	resp := SafeHandleCommand(context.Background(), "telegram 123 r1 /pair abc", dc, sessions, downloads, nil, onboard)
	if resp.Result != "ok" {
		t.Fatalf("expected ok, got %s: %s", resp.Result, resp.Message)
	}
}

// Test helper functions
func TestUsageResp(t *testing.T) {
	resp := usageResp("r1", "/pair <code>")
	if resp.ErrorCode != "E_USAGE" {
		t.Errorf("expected E_USAGE, got %s", resp.ErrorCode)
	}
	if !strings.Contains(resp.Message, "/pair <code>") {
		t.Errorf("expected usage in message, got %q", resp.Message)
	}
}

func TestDaemonErrResp_DaemonClientError(t *testing.T) {
	err := &DaemonClientError{Code: "E_TEST", Message: "test error"}
	resp := daemonErrResp("r1", err)
	if resp.ErrorCode != "E_TEST" {
		t.Errorf("expected E_TEST, got %s", resp.ErrorCode)
	}
}

func TestDaemonErrResp_GenericError(t *testing.T) {
	err := fmt.Errorf("something went wrong")
	resp := daemonErrResp("r1", err)
	if resp.ErrorCode != "E_COMMAND_FAILED" {
		t.Errorf("expected E_COMMAND_FAILED, got %s", resp.ErrorCode)
	}
}
