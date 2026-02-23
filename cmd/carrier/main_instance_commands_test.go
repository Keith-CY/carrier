package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseCarrierCommandRoutesInstanceCommands(t *testing.T) {
	cases := []struct {
		args    []string
		wantCmd string
		wantArg string
	}{
		{args: []string{"carrier", "start", "openclaw"}, wantCmd: "start", wantArg: "openclaw"},
		{args: []string{"carrier", "stop", "openclaw"}, wantCmd: "stop", wantArg: "openclaw"},
		{args: []string{"carrier", "status", "openclaw"}, wantCmd: "status", wantArg: "openclaw"},
		{args: []string{"carrier", "upgrade", "openclaw"}, wantCmd: "upgrade", wantArg: "openclaw"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.wantCmd, func(t *testing.T) {
			cmd, gotArgs, err := parseCarrierCommand(tc.args)
			if err != nil {
				t.Fatalf("parseCarrierCommand(%v) error: %v", tc.args, err)
			}
			if cmd != tc.wantCmd {
				t.Fatalf("command = %q, want %q", cmd, tc.wantCmd)
			}
			if len(gotArgs) != 1 || gotArgs[0] != tc.wantArg {
				t.Fatalf("args = %v, want [%s]", gotArgs, tc.wantArg)
			}
		})
	}
}

func TestResolveManagedInstanceTarget(t *testing.T) {
	instances := []managedAgentInstance{
		{ID: "openclaw-abcd1234", Name: "openclaw", AgentID: "openclaw", Type: "openclaw"},
		{ID: "picoclaw-ffff1111", Name: "picoclaw", AgentID: "picoclaw", Type: "picoclaw"},
	}

	_, idx, err := resolveManagedInstanceTarget(instances, "openclaw")
	if err != nil {
		t.Fatalf("resolve by name error: %v", err)
	}
	if idx != 0 {
		t.Fatalf("resolve by name idx=%d, want 0", idx)
	}

	_, idx, err = resolveManagedInstanceTarget(instances, "picoclaw-ffff1111")
	if err != nil {
		t.Fatalf("resolve by id error: %v", err)
	}
	if idx != 1 {
		t.Fatalf("resolve by id idx=%d, want 1", idx)
	}
}

func TestResolveManagedInstanceTargetAmbiguous(t *testing.T) {
	instances := []managedAgentInstance{
		{ID: "openclaw-1", Name: "openclaw", AgentID: "openclaw", Type: "openclaw"},
		{ID: "openclaw-2", Name: "openclaw", AgentID: "openclaw", Type: "openclaw"},
	}
	_, _, err := resolveManagedInstanceTarget(instances, "openclaw")
	if err == nil {
		t.Fatal("expected ambiguous target error")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestManagedInstanceLifecycleCommandsSupportIDOrName(t *testing.T) {
	tmp := t.TempDir()
	storePath := filepath.Join(tmp, "instances.json")
	t.Setenv("CARRIER_INSTANCE_STORE", storePath)

	instances := []managedAgentInstance{
		{
			ID:           "openclaw-abcd1234",
			Name:         "openclaw",
			Type:         "openclaw",
			AgentID:      "openclaw",
			GatewayURL:   "http://127.0.0.1:8787",
			RuntimeState: "stopped",
			CreatedAt:    "2026-02-23T00:00:00Z",
			UpdatedAt:    "2026-02-23T00:00:00Z",
		},
	}
	if err := saveManagedInstances(storePath, instances); err != nil {
		t.Fatalf("saveManagedInstances: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.URL.Path == "/api/v1/agents/openclaw/logs":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"lines":[]}`))
		case r.URL.Path == "/api/v1/agents/openclaw/start" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"started":true}`))
		case r.URL.Path == "/api/v1/agents/openclaw/stop" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"stopped":true}`))
		case r.URL.Path == "/api/v1/agents/openclaw/upgrade" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"upgraded":true}`))
		case r.URL.Path == "/api/v1/agents/openclaw/status" && r.Method == http.MethodGet:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"statuses":[{"id":"openclaw","installState":"installed","runtimeState":"running"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	configureDaemonProbeEnvForTest(t, server.URL)

	var out bytes.Buffer
	if err := runStartInstance(&out, "openclaw"); err != nil {
		t.Fatalf("runStartInstance by name: %v", err)
	}
	out.Reset()

	if err := runStatusInstance(&out, "openclaw-abcd1234"); err != nil {
		t.Fatalf("runStatusInstance by id: %v", err)
	}
	if !strings.Contains(out.String(), "install=installed runtime=running") {
		t.Fatalf("status output = %q", out.String())
	}
	out.Reset()

	if err := runUpgradeInstance(&out, "openclaw"); err != nil {
		t.Fatalf("runUpgradeInstance by name: %v", err)
	}
	out.Reset()

	if err := runStopInstance(&out, "openclaw"); err != nil {
		t.Fatalf("runStopInstance by name: %v", err)
	}
	updated, _, err := loadManagedInstances()
	if err != nil {
		t.Fatalf("loadManagedInstances: %v", err)
	}
	if got := strings.ToLower(strings.TrimSpace(updated[0].RuntimeState)); got != "stopped" {
		t.Fatalf("runtime state = %q, want stopped", got)
	}
}

func TestRunListInstancesShowsAllManagedInstances(t *testing.T) {
	tmp := t.TempDir()
	storePath := filepath.Join(tmp, "instances.json")
	t.Setenv("CARRIER_INSTANCE_STORE", storePath)

	instances := []managedAgentInstance{
		{
			ID:           "openclaw-abcd1234",
			Name:         "openclaw",
			Type:         "openclaw",
			AgentID:      "openclaw",
			GatewayURL:   "http://127.0.0.1:8787",
			RuntimeState: "running",
			CreatedAt:    "2026-02-23T00:00:00Z",
			UpdatedAt:    "2026-02-23T00:00:00Z",
		},
		{
			ID:           "picoclaw-ffff1111",
			Name:         "picoclaw",
			Type:         "picoclaw",
			AgentID:      "picoclaw",
			GatewayURL:   "http://127.0.0.1:8787",
			RuntimeState: "stopped",
			CreatedAt:    "2026-02-23T00:00:00Z",
			UpdatedAt:    "2026-02-23T00:00:00Z",
		},
	}
	if err := saveManagedInstances(storePath, instances); err != nil {
		t.Fatalf("saveManagedInstances: %v", err)
	}

	var out bytes.Buffer
	if err := runListInstances(&out); err != nil {
		t.Fatalf("runListInstances: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Managed agent instances:") {
		t.Fatalf("missing list header in output: %q", got)
	}
	if !strings.Contains(got, "id=openclaw-abcd1234") || !strings.Contains(got, "id=picoclaw-ffff1111") {
		t.Fatalf("expected all instances in output: %q", got)
	}
}
