package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func buildRemoteFeatureMux(t *testing.T) http.Handler {
	return buildRemoteFeatureMuxWithConfigAndDaemonHandlers(t, nil, nil)
}

func buildRemoteFeatureMuxWithDaemonHandlers(t *testing.T, daemonHandlers map[string]http.HandlerFunc) http.Handler {
	return buildRemoteFeatureMuxWithConfigAndDaemonHandlers(t, nil, daemonHandlers)
}

func buildRemoteFeatureMuxWithConfigAndDaemonHandlers(t *testing.T, cfg *GatewayConfig, daemonHandlers map[string]http.HandlerFunc) http.Handler {
	t.Helper()
	t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(t.TempDir(), "instances.json"))
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))
	resetRemoteMetricsForTests()

	srv := newMockDaemon(daemonHandlers)
	t.Cleanup(srv.Close)
	dc := NewDaemonClient(srv.URL, "test-token", 5*time.Second)
	sessions := NewSessionStore("", 0, nil)
	t.Cleanup(sessions.Stop)
	downloads := NewDownloadStore("", nil)
	rl := NewGatewayRateLimiter(100, 1000, time.Minute, nil)
	onboard := NewOnboardStore()
	setup := NewSetupStore()
	gatewayCfg := cfg
	if gatewayCfg == nil {
		gatewayCfg = &GatewayConfig{
			APIToken:                  "test-gateway-token",
			MaxCommandBodyBytes:       64 * 1024,
			RemoteControlPlaneEnabled: true,
			RemoteChatEnabled:         true,
			ProviderBindingEnabled:    true,
		}
	} else {
		if strings.TrimSpace(gatewayCfg.APIToken) == "" {
			gatewayCfg.APIToken = "test-gateway-token"
		}
		if gatewayCfg.MaxCommandBodyBytes <= 0 {
			gatewayCfg.MaxCommandBodyBytes = 64 * 1024
		}
	}
	return buildGatewayMux(gatewayCfg, dc, sessions, downloads, rl, onboard, setup)
}

func runJSONRequest(t *testing.T, mux http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func decodeJSONMap(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var out map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode json: %v; body=%s", err, rec.Body.String())
	}
	return out
}

func extractHeredocPayload(command string) string {
	re := regexp.MustCompile(`(?s)<<'[^']+'\n(.*)\n[^\n]+$`)
	matches := re.FindStringSubmatch(command)
	if len(matches) != 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

func configureSSHRunner(t *testing.T, fn func(command string) remoteExecResult) {
	t.Helper()
	orig := sshExecRunner
	sshExecRunner = func(_ context.Context, args []string) (remoteExecResult, error) {
		if len(args) == 0 {
			return remoteExecResult{ExitCode: 1, Stderr: "missing ssh args"}, nil
		}
		cmd := unwrapSSHWrappedCommand(args[len(args)-1])
		result := fn(cmd)
		result.Command = cmd
		return result, nil
	}
	t.Cleanup(func() {
		sshExecRunner = orig
	})
}

func configureSSHStreamRunner(t *testing.T, fn func(command string, onChunk func(remoteStreamChunk)) remoteExecResult) {
	t.Helper()
	orig := sshExecStreamRunner
	sshExecStreamRunner = func(_ context.Context, args []string, onChunk func(remoteStreamChunk)) (remoteExecResult, error) {
		if len(args) == 0 {
			return remoteExecResult{ExitCode: 1, Stderr: "missing ssh args"}, nil
		}
		cmd := unwrapSSHWrappedCommand(args[len(args)-1])
		result := fn(cmd, onChunk)
		result.Command = cmd
		return result, nil
	}
	t.Cleanup(func() {
		sshExecStreamRunner = orig
	})
}

func unwrapSSHWrappedCommand(command string) string {
	const prefix = "bash -lc '"
	if !strings.HasPrefix(command, prefix) || !strings.HasSuffix(command, "'") {
		return command
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(command, prefix), "'")
	inner = strings.ReplaceAll(inner, "'\\''", "'")
	inner = strings.TrimPrefix(inner, "export LC_ALL=C LANG=C; ")
	inner = strings.TrimPrefix(inner, "export PATH=\"$HOME/.npm-global/bin:$HOME/.local/bin:$PATH\"; ")
	return inner
}

func createRemoteHostForTests(t *testing.T, mux http.Handler) string {
	t.Helper()
	rec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/hosts", `{
		"name":"test-host",
		"host":"127.0.0.1",
		"port":22,
		"user":"ubuntu",
		"authMode":"private_key",
		"keyPath":"~/.ssh/id_ed25519",
		"runtimeMode":"on_demand"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("create host status=%d body=%s", rec.Code, rec.Body.String())
	}
	payload := decodeJSONMap(t, rec)
	hostMap, _ := payload["host"].(map[string]interface{})
	hostID, _ := hostMap["id"].(string)
	if hostID == "" {
		t.Fatalf("missing host id in response: %v", payload)
	}
	return hostID
}

func TestRemoteHostsCRUDAndCheck(t *testing.T) {
	configureSSHRunner(t, func(command string) remoteExecResult {
		switch {
		case strings.Contains(command, "echo carrier-ssh-ok"):
			return remoteExecResult{ExitCode: 0, Stdout: "carrier-ssh-ok\n"}
		case strings.Contains(command, "command -v openclaw"):
			return remoteExecResult{ExitCode: 0}
		default:
			return remoteExecResult{ExitCode: 0}
		}
	})

	mux := buildRemoteFeatureMux(t)
	hostID := createRemoteHostForTests(t, mux)

	listRec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/remote/hosts", "")
	if listRec.Code != http.StatusOK {
		t.Fatalf("list hosts status=%d body=%s", listRec.Code, listRec.Body.String())
	}
	listPayload := decodeJSONMap(t, listRec)
	hosts, _ := listPayload["hosts"].([]interface{})
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host, got %d payload=%v", len(hosts), listPayload)
	}

	checkRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/hosts/"+hostID+"/check", `{}`)
	if checkRec.Code != http.StatusOK {
		t.Fatalf("check host status=%d body=%s", checkRec.Code, checkRec.Body.String())
	}

	patchRec := runJSONRequest(t, mux, http.MethodPatch, "/api/v1/remote/hosts/"+hostID, `{"name":"patched-host"}`)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("patch host status=%d body=%s", patchRec.Code, patchRec.Body.String())
	}
	patchPayload := decodeJSONMap(t, patchRec)
	hostMap, _ := patchPayload["host"].(map[string]interface{})
	if hostMap["name"] != "patched-host" {
		t.Fatalf("expected patched host name, payload=%v", patchPayload)
	}

	deleteRec := runJSONRequest(t, mux, http.MethodDelete, "/api/v1/remote/hosts/"+hostID, "")
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete host status=%d body=%s", deleteRec.Code, deleteRec.Body.String())
	}
}

func TestRemoteHostCheckRejectsUnsupportedPlatform(t *testing.T) {
	configureSSHRunner(t, func(command string) remoteExecResult {
		switch {
		case strings.Contains(command, "echo carrier-ssh-ok"):
			return remoteExecResult{ExitCode: 0, Stdout: "carrier-ssh-ok\n"}
		case strings.Contains(command, "CARRIER_PLATFORM_PROBE"):
			return remoteExecResult{
				ExitCode: 0,
				Stdout:   "CARRIER_PLATFORM_PROBE\nOS=Linux\nDISTRO=alpine\nVERSION=3.19\n",
			}
		default:
			return remoteExecResult{ExitCode: 0}
		}
	})

	mux := buildRemoteFeatureMux(t)
	hostID := createRemoteHostForTests(t, mux)

	checkRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/hosts/"+hostID+"/check", `{}`)
	if checkRec.Code != http.StatusBadGateway {
		t.Fatalf("check host status=%d body=%s", checkRec.Code, checkRec.Body.String())
	}
	if !strings.Contains(strings.ToLower(checkRec.Body.String()), "unsupported remote platform") {
		t.Fatalf("expected unsupported platform error body=%s", checkRec.Body.String())
	}
}

func TestRemoteInstanceUninstallEndpoint(t *testing.T) {
	configureSSHRunner(t, func(command string) remoteExecResult {
		switch {
		case strings.Contains(command, "rm -rf \"$HOME/.openclaw/agents/main\""):
			return remoteExecResult{ExitCode: 0}
		default:
			return remoteExecResult{ExitCode: 0}
		}
	})
	mux := buildRemoteFeatureMux(t)
	hostID := createRemoteHostForTests(t, mux)

	rec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/hosts/"+hostID+"/instances/main/uninstall", "{}")
	if rec.Code != http.StatusOK {
		t.Fatalf("uninstall status=%d body=%s", rec.Code, rec.Body.String())
	}
	payload := decodeJSONMap(t, rec)
	uninstallMap, _ := payload["uninstall"].(map[string]interface{})
	if uninstallMap == nil {
		t.Fatalf("missing uninstall payload=%v", payload)
	}
	if uninstalled, _ := uninstallMap["uninstalled"].(bool); !uninstalled {
		t.Fatalf("expected uninstalled=true payload=%v", payload)
	}
}

func TestRemoteHostCheckDiscoversAndPullsPicoZeroClawConfigs(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "profiles-repo")
	t.Setenv("CARRIER_PROFILESYNC_REPO", repoRoot)

	configureSSHRunner(t, func(command string) remoteExecResult {
		switch {
		case strings.Contains(command, "echo carrier-ssh-ok"):
			return remoteExecResult{ExitCode: 0, Stdout: "carrier-ssh-ok\n"}
		case strings.Contains(command, "command -v openclaw"):
			return remoteExecResult{ExitCode: 1}
		case strings.Contains(command, "cat \"$HOME/.picoclaw/config.json\""):
			return remoteExecResult{ExitCode: 0, Stdout: `{"provider":"openai","model":"gpt-4.1-mini"}`}
		case strings.Contains(command, "cat \"$HOME/.zeroclaw/config.toml\""):
			return remoteExecResult{ExitCode: 0, Stdout: "api_key = \"zk-test\"\nmodel = \"gpt-4.1\""}
		case strings.Contains(command, "if [ -f \"$HOME/.picoclaw/config.json\" ]"):
			return remoteExecResult{ExitCode: 0, Stdout: "1\n"}
		case strings.Contains(command, "if [ -f \"$HOME/.zeroclaw/config.toml\" ]"):
			return remoteExecResult{ExitCode: 0, Stdout: "1\n"}
		default:
			return remoteExecResult{ExitCode: 0}
		}
	})

	mux := buildRemoteFeatureMux(t)
	hostID := createRemoteHostForTests(t, mux)

	checkRecNoPull := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/hosts/"+hostID+"/check", `{}`)
	if checkRecNoPull.Code != http.StatusOK {
		t.Fatalf("check(no pull) status=%d body=%s", checkRecNoPull.Code, checkRecNoPull.Body.String())
	}
	checkPayload := decodeJSONMap(t, checkRecNoPull)
	rawInstances, _ := checkPayload["instances"].([]interface{})
	if len(rawInstances) < 2 {
		t.Fatalf("expected discovered instances in check payload, got %+v", checkPayload)
	}
	rawPending, _ := checkPayload["pendingPullInstances"].([]interface{})
	if len(rawPending) < 2 {
		t.Fatalf("expected pending pull instances before confirmation, got %+v", checkPayload)
	}

	sanitize := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "..", "_")
	picoInstanceID := sanitize.Replace(remoteInstanceProfileID(hostID, "picoclaw"))
	zeroInstanceID := sanitize.Replace(remoteInstanceProfileID(hostID, "zeroclaw"))

	picoMetadataPath := filepath.Join(repoRoot, "instances", picoInstanceID, "metadata.json")
	if _, err := os.Stat(picoMetadataPath); err == nil {
		t.Fatalf("expected no local pull before confirmation, but found %s", picoMetadataPath)
	}

	checkRecPull := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/hosts/"+hostID+"/check", `{"pullNewInstances":true}`)
	if checkRecPull.Code != http.StatusOK {
		t.Fatalf("check(pull confirm) status=%d body=%s", checkRecPull.Code, checkRecPull.Body.String())
	}
	pullPayload := decodeJSONMap(t, checkRecPull)
	if required, _ := pullPayload["pullConfirmationRequired"].(bool); required {
		t.Fatalf("expected no pending pull after confirmation, payload=%v", pullPayload)
	}

	picoMetadataRaw, err := os.ReadFile(picoMetadataPath)
	if err != nil {
		t.Fatalf("read picoclaw metadata: %v", err)
	}
	var picoMetadata map[string]interface{}
	if err := json.Unmarshal(picoMetadataRaw, &picoMetadata); err != nil {
		t.Fatalf("parse picoclaw metadata: %v", err)
	}
	if strings.TrimSpace(anyToString(picoMetadata["agentId"])) != "picoclaw" {
		t.Fatalf("unexpected picoclaw metadata: %+v", picoMetadata)
	}

	zeroMetadataPath := filepath.Join(repoRoot, "instances", zeroInstanceID, "metadata.json")
	zeroMetadataRaw, err := os.ReadFile(zeroMetadataPath)
	if err != nil {
		t.Fatalf("read zeroclaw metadata: %v", err)
	}
	var zeroMetadata map[string]interface{}
	if err := json.Unmarshal(zeroMetadataRaw, &zeroMetadata); err != nil {
		t.Fatalf("parse zeroclaw metadata: %v", err)
	}
	if strings.TrimSpace(anyToString(zeroMetadata["agentId"])) != "zeroclaw" {
		t.Fatalf("unexpected zeroclaw metadata: %+v", zeroMetadata)
	}

	zeroProfilePath := filepath.Join(repoRoot, "instances", zeroInstanceID, "openclaw.json")
	zeroProfileRaw, err := os.ReadFile(zeroProfilePath)
	if err != nil {
		t.Fatalf("read zeroclaw canonical profile: %v", err)
	}
	var zeroProfile map[string]interface{}
	if err := json.Unmarshal(zeroProfileRaw, &zeroProfile); err != nil {
		t.Fatalf("parse zeroclaw canonical profile: %v", err)
	}
	if !strings.Contains(anyToString(zeroProfile["raw_toml"]), "api_key = \"zk-test\"") {
		t.Fatalf("expected zeroclaw raw_toml in canonical profile, got %+v", zeroProfile)
	}
}

func TestRemoteSyncSupportsPicoAndZeroClaw(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "profiles-repo")
	t.Setenv("CARRIER_PROFILESYNC_REPO", repoRoot)

	configureSSHRunner(t, func(command string) remoteExecResult {
		switch {
		case strings.Contains(command, "cat \"$HOME/.picoclaw/config.json\""):
			return remoteExecResult{ExitCode: 0, Stdout: `{"provider":"openai","model":"gpt-4.1-mini"}`}
		case strings.Contains(command, "cat \"$HOME/.zeroclaw/config.toml\""):
			return remoteExecResult{ExitCode: 0, Stdout: "api_key = \"zk-sync\"\nmodel = \"gpt-4.1\""}
		default:
			return remoteExecResult{ExitCode: 0}
		}
	})

	mux := buildRemoteFeatureMux(t)
	hostID := createRemoteHostForTests(t, mux)

	for _, agentID := range []string{"picoclaw", "zeroclaw"} {
		syncRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/hosts/"+hostID+"/instances/"+agentID+"/sync", `{"mode":"pull_validate_push"}`)
		if syncRec.Code != http.StatusOK {
			t.Fatalf("sync %s status=%d body=%s", agentID, syncRec.Code, syncRec.Body.String())
		}
		syncPayload := decodeJSONMap(t, syncRec)
		syncMap, _ := syncPayload["sync"].(map[string]interface{})
		if strings.TrimSpace(anyToString(syncMap["status"])) != "in_sync" {
			t.Fatalf("expected in_sync for %s, payload=%v", agentID, syncPayload)
		}
	}

	sanitize := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "..", "_")
	picoInstanceID := sanitize.Replace(remoteInstanceProfileID(hostID, "picoclaw"))
	zeroInstanceID := sanitize.Replace(remoteInstanceProfileID(hostID, "zeroclaw"))

	picoMetadataPath := filepath.Join(repoRoot, "instances", picoInstanceID, "metadata.json")
	picoMetadataRaw, err := os.ReadFile(picoMetadataPath)
	if err != nil {
		t.Fatalf("read picoclaw metadata: %v", err)
	}
	var picoMetadata map[string]interface{}
	if err := json.Unmarshal(picoMetadataRaw, &picoMetadata); err != nil {
		t.Fatalf("parse picoclaw metadata: %v", err)
	}
	if strings.TrimSpace(anyToString(picoMetadata["agentId"])) != "picoclaw" {
		t.Fatalf("unexpected picoclaw metadata after sync: %+v", picoMetadata)
	}

	zeroProfilePath := filepath.Join(repoRoot, "instances", zeroInstanceID, "openclaw.json")
	zeroProfileRaw, err := os.ReadFile(zeroProfilePath)
	if err != nil {
		t.Fatalf("read zeroclaw canonical profile after sync: %v", err)
	}
	var zeroProfile map[string]interface{}
	if err := json.Unmarshal(zeroProfileRaw, &zeroProfile); err != nil {
		t.Fatalf("parse zeroclaw canonical profile after sync: %v", err)
	}
	if !strings.Contains(anyToString(zeroProfile["raw_toml"]), "api_key = \"zk-sync\"") {
		t.Fatalf("expected zeroclaw raw_toml after sync, got %+v", zeroProfile)
	}
}

func TestRemoteHostCheckSelectivePullNewInstances(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "profiles-repo")
	t.Setenv("CARRIER_PROFILESYNC_REPO", repoRoot)

	configureSSHRunner(t, func(command string) remoteExecResult {
		switch {
		case strings.Contains(command, "echo carrier-ssh-ok"):
			return remoteExecResult{ExitCode: 0, Stdout: "carrier-ssh-ok\n"}
		case strings.Contains(command, "command -v openclaw"):
			return remoteExecResult{ExitCode: 1}
		case strings.Contains(command, "cat \"$HOME/.picoclaw/config.json\""):
			return remoteExecResult{ExitCode: 0, Stdout: `{"provider":"openai","model":"gpt-4.1-mini"}`}
		case strings.Contains(command, "cat \"$HOME/.zeroclaw/config.toml\""):
			return remoteExecResult{ExitCode: 0, Stdout: "api_key = \"zk-selective\"\nmodel = \"gpt-4.1\""}
		case strings.Contains(command, "if [ -f \"$HOME/.picoclaw/config.json\" ]"):
			return remoteExecResult{ExitCode: 0, Stdout: "1\n"}
		case strings.Contains(command, "if [ -f \"$HOME/.zeroclaw/config.toml\" ]"):
			return remoteExecResult{ExitCode: 0, Stdout: "1\n"}
		default:
			return remoteExecResult{ExitCode: 0}
		}
	})

	mux := buildRemoteFeatureMux(t)
	hostID := createRemoteHostForTests(t, mux)

	checkRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/hosts/"+hostID+"/check", `{"pullNewInstances":true,"pullAgentIds":["picoclaw"]}`)
	if checkRec.Code != http.StatusOK {
		t.Fatalf("check selective pull status=%d body=%s", checkRec.Code, checkRec.Body.String())
	}
	payload := decodeJSONMap(t, checkRec)
	pending, _ := payload["pendingPullInstances"].([]interface{})
	if len(pending) != 1 {
		t.Fatalf("expected one pending instance after selective pull, got %+v", payload)
	}
	pendingEntry, _ := pending[0].(map[string]interface{})
	if strings.TrimSpace(strings.ToLower(anyToString(pendingEntry["agentId"]))) != "zeroclaw" {
		t.Fatalf("expected pending zeroclaw, got %+v", pendingEntry)
	}

	sanitize := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "..", "_")
	picoInstanceID := sanitize.Replace(remoteInstanceProfileID(hostID, "picoclaw"))
	zeroInstanceID := sanitize.Replace(remoteInstanceProfileID(hostID, "zeroclaw"))
	if _, err := os.Stat(filepath.Join(repoRoot, "instances", picoInstanceID, "metadata.json")); err != nil {
		t.Fatalf("expected picoclaw metadata pulled: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "instances", zeroInstanceID, "metadata.json")); err == nil {
		t.Fatalf("expected zeroclaw metadata not pulled in selective mode")
	}
}

func TestProviderProfilesPatchAndBinding(t *testing.T) {
	configureSSHRunner(t, func(command string) remoteExecResult {
		switch {
		case strings.Contains(command, "cat \"$HOME/.openclaw/openclaw.json\""):
			return remoteExecResult{ExitCode: 0, Stdout: `{"agents":{"defaults":{"provider":"openai","model":"gpt-4.1"}}}`}
		case strings.Contains(command, "cat > \"$HOME/.openclaw/openclaw.json\""):
			return remoteExecResult{ExitCode: 0}
		default:
			return remoteExecResult{ExitCode: 0}
		}
	})

	mux := buildRemoteFeatureMux(t)
	hostID := createRemoteHostForTests(t, mux)

	createProfileRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/provider-profiles", `{
		"name":"openai-gpt5",
		"provider":"openai",
		"model":"gpt-5",
		"enabled":false
	}`)
	if createProfileRec.Code != http.StatusOK {
		t.Fatalf("create profile status=%d body=%s", createProfileRec.Code, createProfileRec.Body.String())
	}
	createPayload := decodeJSONMap(t, createProfileRec)
	profile, _ := createPayload["profile"].(map[string]interface{})
	profileID, _ := profile["id"].(string)
	if profileID == "" {
		t.Fatalf("missing profile id: %v", createPayload)
	}
	if enabled, _ := profile["enabled"].(bool); enabled {
		t.Fatalf("expected profile enabled=false at creation")
	}

	patchModelRec := runJSONRequest(t, mux, http.MethodPatch, "/api/v1/provider-profiles/"+profileID, `{"model":"gpt-5-mini"}`)
	if patchModelRec.Code != http.StatusOK {
		t.Fatalf("patch profile model status=%d body=%s", patchModelRec.Code, patchModelRec.Body.String())
	}
	patchModelPayload := decodeJSONMap(t, patchModelRec)
	patchModelProfile, _ := patchModelPayload["profile"].(map[string]interface{})
	if enabled, _ := patchModelProfile["enabled"].(bool); enabled {
		t.Fatalf("expected enabled to remain false when omitted from patch")
	}

	patchEnabledRec := runJSONRequest(t, mux, http.MethodPatch, "/api/v1/provider-profiles/"+profileID, `{"enabled":true}`)
	if patchEnabledRec.Code != http.StatusOK {
		t.Fatalf("patch profile enabled status=%d body=%s", patchEnabledRec.Code, patchEnabledRec.Body.String())
	}
	patchEnabledPayload := decodeJSONMap(t, patchEnabledRec)
	patchEnabledProfile, _ := patchEnabledPayload["profile"].(map[string]interface{})
	if enabled, _ := patchEnabledProfile["enabled"].(bool); !enabled {
		t.Fatalf("expected enabled=true after explicit patch")
	}

	bindRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/provider-bindings", `{
		"profileId":"`+profileID+`",
		"targetType":"host",
		"targetId":"`+hostID+`",
		"syncMode":"always_push"
	}`)
	if bindRec.Code != http.StatusOK {
		t.Fatalf("create binding status=%d body=%s", bindRec.Code, bindRec.Body.String())
	}

	listBindingsRec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/provider-bindings", "")
	if listBindingsRec.Code != http.StatusOK {
		t.Fatalf("list bindings status=%d body=%s", listBindingsRec.Code, listBindingsRec.Body.String())
	}
	listBindingsPayload := decodeJSONMap(t, listBindingsRec)
	bindings, _ := listBindingsPayload["bindings"].([]interface{})
	if len(bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d payload=%v", len(bindings), listBindingsPayload)
	}
	bindingMap, _ := bindings[0].(map[string]interface{})
	bindingID, _ := bindingMap["id"].(string)
	if bindingID == "" {
		t.Fatalf("expected binding id in payload=%v", listBindingsPayload)
	}

	deleteBindingRec := runJSONRequest(t, mux, http.MethodDelete, "/api/v1/provider-bindings/"+bindingID, "")
	if deleteBindingRec.Code != http.StatusOK {
		t.Fatalf("delete binding status=%d body=%s", deleteBindingRec.Code, deleteBindingRec.Body.String())
	}

	listBindingsAfterDeleteRec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/provider-bindings", "")
	if listBindingsAfterDeleteRec.Code != http.StatusOK {
		t.Fatalf("list bindings after delete status=%d body=%s", listBindingsAfterDeleteRec.Code, listBindingsAfterDeleteRec.Body.String())
	}
	listAfterDeletePayload := decodeJSONMap(t, listBindingsAfterDeleteRec)
	bindingsAfterDelete, _ := listAfterDeletePayload["bindings"].([]interface{})
	if len(bindingsAfterDelete) != 0 {
		t.Fatalf("expected 0 bindings after delete, got %d payload=%v", len(bindingsAfterDelete), listAfterDeletePayload)
	}
}

func TestRemoteChatStreamSSE(t *testing.T) {
	configureSSHRunner(t, func(command string) remoteExecResult {
		switch {
		case strings.Contains(command, "command -v openclaw"):
			return remoteExecResult{ExitCode: 0}
		case strings.Contains(command, "openclaw agent --local"):
			return remoteExecResult{ExitCode: 0, Stdout: `{"message":"hello from remote","sessionId":"sess-123"}`}
		default:
			return remoteExecResult{ExitCode: 0}
		}
	})

	mux := buildRemoteFeatureMux(t)
	hostID := createRemoteHostForTests(t, mux)

	rec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/chat/stream", `{
		"hostId":"`+hostID+`",
		"agentId":"main",
		"message":"hello"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("remote chat status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"type":"text-delta"`) {
		t.Fatalf("expected text-delta event in stream body: %s", body)
	}
	if !strings.Contains(body, `"type":"session"`) {
		t.Fatalf("expected session event in stream body: %s", body)
	}
	if !strings.Contains(body, `"type":"finish"`) {
		t.Fatalf("expected finish event in stream body: %s", body)
	}

	unifiedRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/chat/stream", `{
		"target":"remote",
		"hostId":"`+hostID+`",
		"agentId":"main",
		"message":"hello"
	}`)
	if unifiedRec.Code != http.StatusOK {
		t.Fatalf("unified remote chat status=%d body=%s", unifiedRec.Code, unifiedRec.Body.String())
	}
	unifiedBody := unifiedRec.Body.String()
	if !strings.Contains(unifiedBody, `"type":"text-delta"`) || !strings.Contains(unifiedBody, `"type":"finish"`) {
		t.Fatalf("expected sse events in unified remote stream body: %s", unifiedBody)
	}
}

func TestRemoteInstallStreamSSE(t *testing.T) {
	var installCommand string
	configureSSHStreamRunner(t, func(command string, onChunk func(remoteStreamChunk)) remoteExecResult {
		if strings.Contains(command, "install.sh") {
			installCommand = command
			if onChunk != nil {
				onChunk(remoteStreamChunk{Stream: "stdout", Text: "download complete"})
				onChunk(remoteStreamChunk{Stream: "stdout", Text: "install complete"})
			}
			return remoteExecResult{ExitCode: 0, Stdout: "download complete\ninstall complete"}
		}
		return remoteExecResult{ExitCode: 0}
	})

	mux := buildRemoteFeatureMux(t)
	hostID := createRemoteHostForTests(t, mux)

	rec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/hosts/"+hostID+"/instances/main/install/stream", `{"isolation":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("install stream status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"type":"start"`) {
		t.Fatalf("expected start event in stream body: %s", body)
	}
	if !strings.Contains(body, `"type":"log"`) {
		t.Fatalf("expected log event in stream body: %s", body)
	}
	if !strings.Contains(body, `"line":"download complete"`) {
		t.Fatalf("expected streamed install output in body: %s", body)
	}
	if !strings.Contains(body, `"type":"result"`) || !strings.Contains(body, `"installed":true`) {
		t.Fatalf("expected result event with installed=true in body: %s", body)
	}
	if !strings.Contains(body, `"type":"finish"`) {
		t.Fatalf("expected finish event in stream body: %s", body)
	}
	if !strings.Contains(installCommand, "--isolation") {
		t.Fatalf("expected install command to include --isolation, got %q", installCommand)
	}
}

func TestRemoteChatDisabledWhenRemoteControlFlagOff(t *testing.T) {
	mux := buildRemoteFeatureMuxWithConfigAndDaemonHandlers(t, &GatewayConfig{
		APIToken:                  "test-gateway-token",
		MaxCommandBodyBytes:       64 * 1024,
		RemoteControlPlaneEnabled: false,
		RemoteChatEnabled:         true, // intentionally true; should be normalized off.
		ProviderBindingEnabled:    true,
	}, nil)

	rec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/chat/stream", `{
		"hostId":"host-1",
		"agentId":"main",
		"message":"hello"
	}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected remote chat disabled status=404, got status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "remote chat is disabled") {
		t.Fatalf("expected disabled message, got body=%s", rec.Body.String())
	}

	unifiedRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/chat/stream", `{
		"target":"remote",
		"hostId":"host-1",
		"agentId":"main",
		"message":"hello"
	}`)
	if unifiedRec.Code != http.StatusNotFound {
		t.Fatalf("expected unified remote chat disabled status=404, got status=%d body=%s", unifiedRec.Code, unifiedRec.Body.String())
	}
}

func TestUnifiedChatStreamLocal(t *testing.T) {
	mux := buildRemoteFeatureMuxWithDaemonHandlers(t, map[string]http.HandlerFunc{
		"POST /api/v1/base-agent/chat": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"message":"hello from local base-agent","action":"chat"}`))
		},
	})

	rec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/chat/stream", `{
		"target":"local",
		"message":"what is the status",
		"sessionId":"local-session-1",
		"provider":"webui",
		"chatId":"local-chat-1"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("unified local chat status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"type":"text-delta"`) {
		t.Fatalf("expected text-delta event in stream body: %s", body)
	}
	if !strings.Contains(body, `"sessionId":"local-session-1"`) {
		t.Fatalf("expected session event in stream body: %s", body)
	}
	if !strings.Contains(body, `"type":"finish"`) {
		t.Fatalf("expected finish event in stream body: %s", body)
	}
}

func TestUnifiedChatStreamLocalInstance(t *testing.T) {
	mux := buildRemoteFeatureMuxWithDaemonHandlers(t, map[string]http.HandlerFunc{
		"POST /api/v1/agents/openclaw/chat": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"agentId":"openclaw","sessionId":"sess-local-inst-1","message":"hello from local instance"}`))
		},
	})

	rec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/chat/stream", `{
		"target":"local",
		"agentId":"openclaw",
		"message":"hello local instance",
		"sessionId":"sess-local-inst-1"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("unified local instance chat status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"type":"text-delta"`) {
		t.Fatalf("expected text-delta event in stream body: %s", body)
	}
	if !strings.Contains(body, `"sessionId":"sess-local-inst-1"`) {
		t.Fatalf("expected session event in stream body: %s", body)
	}
	if !strings.Contains(body, `"type":"finish"`) {
		t.Fatalf("expected finish event in stream body: %s", body)
	}
}

func TestRemoteMetricsEndpointTracksOperations(t *testing.T) {
	chatCalls := 0
	configureSSHRunner(t, func(command string) remoteExecResult {
		switch {
		case strings.Contains(command, "echo carrier-ssh-ok"):
			return remoteExecResult{ExitCode: 0, Stdout: "carrier-ssh-ok\n"}
		case strings.Contains(command, "command -v openclaw"):
			return remoteExecResult{ExitCode: 0}
		case strings.Contains(command, "openclaw agent --local"):
			chatCalls++
			if chatCalls == 1 {
				return remoteExecResult{ExitCode: 0, Stdout: `{"message":"ok","sessionId":"sess-1"}`}
			}
			return remoteExecResult{ExitCode: 1, Stderr: "chat failed"}
		default:
			return remoteExecResult{ExitCode: 0}
		}
	})

	mux := buildRemoteFeatureMux(t)
	hostID := createRemoteHostForTests(t, mux)

	checkRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/hosts/"+hostID+"/check", `{}`)
	if checkRec.Code != http.StatusOK {
		t.Fatalf("check host status=%d body=%s", checkRec.Code, checkRec.Body.String())
	}

	chatOK := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/chat/stream", `{
		"hostId":"`+hostID+`",
		"agentId":"main",
		"message":"hello"
	}`)
	if chatOK.Code != http.StatusOK {
		t.Fatalf("chat success status=%d body=%s", chatOK.Code, chatOK.Body.String())
	}

	chatFail := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/chat/stream", `{
		"hostId":"`+hostID+`",
		"agentId":"main",
		"message":"hello again"
	}`)
	if chatFail.Code != http.StatusBadGateway {
		t.Fatalf("chat failure status=%d body=%s", chatFail.Code, chatFail.Body.String())
	}

	metricsRec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/remote/metrics", "")
	if metricsRec.Code != http.StatusOK {
		t.Fatalf("metrics status=%d body=%s", metricsRec.Code, metricsRec.Body.String())
	}
	payload := decodeJSONMap(t, metricsRec)
	metrics, _ := payload["metrics"].(map[string]interface{})
	if metrics == nil {
		t.Fatalf("missing metrics payload: %v", payload)
	}
	ops, _ := metrics["operations"].(map[string]interface{})
	if ops == nil {
		t.Fatalf("missing operations metrics: %v", metrics)
	}
	hostCheckRaw, _ := ops["host_check"].(map[string]interface{})
	if hostCheckRaw == nil || int(hostCheckRaw["total"].(float64)) < 1 {
		t.Fatalf("expected host_check total >= 1, got %v", hostCheckRaw)
	}

	chatRaw, _ := metrics["chatStream"].(map[string]interface{})
	if chatRaw == nil {
		t.Fatalf("missing chatStream metrics: %v", metrics)
	}
	if int(chatRaw["total"].(float64)) != 2 {
		t.Fatalf("expected chat total=2, got %v", chatRaw)
	}
	if int(chatRaw["failure"].(float64)) != 1 {
		t.Fatalf("expected chat failure=1, got %v", chatRaw)
	}
	if chatRaw["failureRate"].(float64) <= 0 {
		t.Fatalf("expected positive chat failure rate, got %v", chatRaw)
	}

	rolloutRaw, _ := metrics["rollout"].(map[string]interface{})
	if rolloutRaw == nil {
		t.Fatalf("missing rollout metrics: %v", metrics)
	}
	if rolloutRaw["state"] != "canary" {
		t.Fatalf("expected rollout state canary for chat failure sample, got %v", rolloutRaw)
	}
	if canPromote, ok := rolloutRaw["canPromote"].(bool); !ok || canPromote {
		t.Fatalf("expected canPromote=false for canary state, got %v", rolloutRaw)
	}
}

func TestRemoteMetricsEndpointMethodGuard(t *testing.T) {
	mux := buildRemoteFeatureMux(t)

	rec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/metrics", `{}`)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRemoteSessionActionsRejectInvalidSessionID(t *testing.T) {
	sshCalls := 0
	configureSSHRunner(t, func(command string) remoteExecResult {
		sshCalls++
		return remoteExecResult{ExitCode: 0}
	})

	mux := buildRemoteFeatureMux(t)
	hostID := createRemoteHostForTests(t, mux)

	archiveRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/hosts/"+hostID+"/sessions/bad*session/archive?agentId=main", `{}`)
	if archiveRec.Code != http.StatusBadRequest {
		t.Fatalf("archive invalid session status=%d body=%s", archiveRec.Code, archiveRec.Body.String())
	}
	if sshCalls != 0 {
		t.Fatalf("expected no ssh command for invalid archive session id, calls=%d", sshCalls)
	}

	deleteRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/hosts/"+hostID+"/sessions/bad*session/delete?agentId=main", `{}`)
	if deleteRec.Code != http.StatusBadRequest {
		t.Fatalf("delete invalid session status=%d body=%s", deleteRec.Code, deleteRec.Body.String())
	}
	if sshCalls != 0 {
		t.Fatalf("expected no ssh command for invalid delete session id, calls=%d", sshCalls)
	}
}

func TestRemotePatchConfigUsesExpandedSnapshotPath(t *testing.T) {
	var writeCommand string
	configureSSHRunner(t, func(command string) remoteExecResult {
		switch {
		case strings.Contains(command, "cat \"$HOME/.openclaw/openclaw.json\""):
			return remoteExecResult{ExitCode: 0, Stdout: `{"agents":{"defaults":{"model":"gpt-4.1"}}}`}
		case strings.Contains(command, "cat > \"$HOME/.openclaw/openclaw.json\""):
			writeCommand = command
			return remoteExecResult{ExitCode: 0}
		default:
			return remoteExecResult{ExitCode: 0}
		}
	})

	host := RemoteHost{
		ID:          "host-1",
		Host:        "127.0.0.1",
		Port:        22,
		User:        "ubuntu",
		AuthMode:    RemoteAuthModePrivateKey,
		KeyPath:     "~/.ssh/id_ed25519",
		RuntimeMode: RemoteRuntimeModeOnDemand,
	}
	_, snapshotPath, _, err := remotePatchConfig(context.Background(), host, map[string]interface{}{
		"agents": map[string]interface{}{
			"defaults": map[string]interface{}{
				"model": "gpt-5-mini",
			},
		},
	})
	if err != nil {
		t.Fatalf("remotePatchConfig failed: %v", err)
	}
	if !strings.Contains(snapshotPath, "$HOME/.openclaw/snapshots/openclaw-") {
		t.Fatalf("unexpected snapshot path: %q", snapshotPath)
	}
	if !strings.Contains(writeCommand, "snapshot_path=\"$HOME/.openclaw/snapshots/openclaw-") {
		t.Fatalf("expected shell-expanded snapshot path assignment, command=%s", writeCommand)
	}
	if strings.Contains(writeCommand, "'$HOME/.openclaw/snapshots/") {
		t.Fatalf("snapshot path should not be single-quoted with literal $HOME, command=%s", writeCommand)
	}
}

func TestProviderBindingsAcceptExtendedSyncModes(t *testing.T) {
	configureSSHRunner(t, func(command string) remoteExecResult {
		switch {
		case strings.Contains(command, "cat \"$HOME/.openclaw/openclaw.json\""):
			return remoteExecResult{ExitCode: 0, Stdout: `{"agents":{"defaults":{"provider":"openai","model":"gpt-4.1"}}}`}
		case strings.Contains(command, "cat > \"$HOME/.openclaw/openclaw.json\""):
			return remoteExecResult{ExitCode: 0}
		default:
			return remoteExecResult{ExitCode: 0}
		}
	})

	mux := buildRemoteFeatureMux(t)
	hostID := createRemoteHostForTests(t, mux)

	createProfileRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/provider-profiles", `{
		"name":"sync-mode-test",
		"provider":"openai",
		"model":"gpt-5"
	}`)
	if createProfileRec.Code != http.StatusOK {
		t.Fatalf("create profile status=%d body=%s", createProfileRec.Code, createProfileRec.Body.String())
	}
	createPayload := decodeJSONMap(t, createProfileRec)
	profile, _ := createPayload["profile"].(map[string]interface{})
	profileID, _ := profile["id"].(string)
	if profileID == "" {
		t.Fatalf("missing profile id in payload=%v", createPayload)
	}

	for _, syncMode := range []string{"pull_validate_push", "manual"} {
		rec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/provider-bindings", `{
			"profileId":"`+profileID+`",
			"targetType":"host",
			"targetId":"`+hostID+`",
			"syncMode":"`+syncMode+`"
		}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("syncMode=%s expected 200 got status=%d body=%s", syncMode, rec.Code, rec.Body.String())
		}
		payload := decodeJSONMap(t, rec)
		binding, _ := payload["binding"].(map[string]interface{})
		if got, _ := binding["syncMode"].(string); got != syncMode {
			t.Fatalf("syncMode=%s got binding.syncMode=%q payload=%v", syncMode, got, payload)
		}
	}
}

func TestRemoteInstanceSyncDiagnoseAndReconcile(t *testing.T) {
	configPatchWrites := 0
	configureSSHRunner(t, func(command string) remoteExecResult {
		switch {
		case strings.Contains(command, "cat \"$HOME/.openclaw/openclaw.json\""):
			return remoteExecResult{ExitCode: 0, Stdout: `{"agents":{"defaults":{"provider":"openai","model":"gpt-4.1"}}}`}
		case strings.Contains(command, "cat > \"$HOME/.openclaw/openclaw.json\""):
			configPatchWrites++
			return remoteExecResult{ExitCode: 0}
		default:
			return remoteExecResult{ExitCode: 0}
		}
	})

	mux := buildRemoteFeatureMux(t)
	hostID := createRemoteHostForTests(t, mux)

	syncRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/hosts/"+hostID+"/instances/main/sync", `{"mode":"pull_validate_push"}`)
	if syncRec.Code != http.StatusOK {
		t.Fatalf("sync status=%d body=%s", syncRec.Code, syncRec.Body.String())
	}
	syncPayload := decodeJSONMap(t, syncRec)
	syncMap, _ := syncPayload["sync"].(map[string]interface{})
	if syncMap == nil {
		t.Fatalf("missing sync payload: %v", syncPayload)
	}
	if got, _ := syncMap["status"].(string); got != "in_sync" {
		t.Fatalf("expected sync status=in_sync got=%q payload=%v", got, syncPayload)
	}
	if got, _ := syncMap["mode"].(string); got != "pull_validate_push" {
		t.Fatalf("expected sync mode pull_validate_push got=%q payload=%v", got, syncPayload)
	}

	statusRec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/remote/hosts/"+hostID+"/instances/main/sync/status", "")
	if statusRec.Code != http.StatusOK {
		t.Fatalf("sync status api status=%d body=%s", statusRec.Code, statusRec.Body.String())
	}
	statusPayload := decodeJSONMap(t, statusRec)
	statusMap, _ := statusPayload["status"].(map[string]interface{})
	if statusMap == nil {
		t.Fatalf("missing status payload: %v", statusPayload)
	}
	if got, _ := statusMap["driftState"].(string); got != "in_sync" {
		t.Fatalf("expected driftState=in_sync got=%q payload=%v", got, statusPayload)
	}

	diagnoseRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/hosts/"+hostID+"/instances/main/diagnose", `{}`)
	if diagnoseRec.Code != http.StatusOK {
		t.Fatalf("diagnose status=%d body=%s", diagnoseRec.Code, diagnoseRec.Body.String())
	}
	diagnosePayload := decodeJSONMap(t, diagnoseRec)
	diagnoseMap, _ := diagnosePayload["diagnose"].(map[string]interface{})
	if diagnoseMap == nil {
		t.Fatalf("missing diagnose payload: %v", diagnosePayload)
	}
	if got, _ := diagnoseMap["result"].(string); got != "healthy" {
		t.Fatalf("expected diagnose result healthy got=%q payload=%v", got, diagnosePayload)
	}

	reconcileRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/hosts/"+hostID+"/instances/main/reconcile", `{}`)
	if reconcileRec.Code != http.StatusOK {
		t.Fatalf("reconcile status=%d body=%s", reconcileRec.Code, reconcileRec.Body.String())
	}
	reconcilePayload := decodeJSONMap(t, reconcileRec)
	reconcileMap, _ := reconcilePayload["reconcile"].(map[string]interface{})
	if reconcileMap == nil {
		t.Fatalf("missing reconcile payload: %v", reconcilePayload)
	}
	if reconciled, _ := reconcileMap["reconciled"].(bool); !reconciled {
		t.Fatalf("expected reconciled=true payload=%v", reconcilePayload)
	}
	if configPatchWrites < 1 {
		t.Fatalf("expected at least one config patch write during reconcile, writes=%d", configPatchWrites)
	}
}

func TestRemoteRollbackRestoresConfigFromGitProfile(t *testing.T) {
	t.Setenv("CARRIER_PROFILESYNC_REPO", t.TempDir())

	remoteConfig := `{"agents":{"defaults":{"provider":"openai","model":"gpt-4.1"}}}`
	configureSSHRunner(t, func(command string) remoteExecResult {
		switch {
		case strings.Contains(command, "cat \"$HOME/.openclaw/openclaw.json\""):
			return remoteExecResult{ExitCode: 0, Stdout: remoteConfig}
		case strings.Contains(command, "cat > \"$HOME/.openclaw/openclaw.json\""):
			payload := extractHeredocPayload(command)
			if strings.TrimSpace(payload) != "" {
				remoteConfig = payload
			}
			return remoteExecResult{ExitCode: 0}
		default:
			return remoteExecResult{ExitCode: 0}
		}
	})

	mux := buildRemoteFeatureMux(t)
	hostID := createRemoteHostForTests(t, mux)

	syncRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/hosts/"+hostID+"/instances/main/sync", `{"mode":"pull_validate_push"}`)
	if syncRec.Code != http.StatusOK {
		t.Fatalf("sync status=%d body=%s", syncRec.Code, syncRec.Body.String())
	}
	statusRec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/remote/hosts/"+hostID+"/instances/main/sync/status", "")
	if statusRec.Code != http.StatusOK {
		t.Fatalf("sync status api status=%d body=%s", statusRec.Code, statusRec.Body.String())
	}
	statusPayload := decodeJSONMap(t, statusRec)
	statusMap, _ := statusPayload["status"].(map[string]interface{})
	if statusMap == nil {
		t.Fatalf("missing status payload: %v", statusPayload)
	}
	commit, _ := statusMap["lastLocalCommit"].(string)
	if commit == "" {
		t.Fatalf("expected lastLocalCommit in sync status payload=%v", statusPayload)
	}

	patchRec := runJSONRequest(t, mux, http.MethodPatch, "/api/v1/remote/hosts/"+hostID+"/config", `{
		"patch":{"agents":{"defaults":{"model":"gpt-5"}}}
	}`)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("config patch status=%d body=%s", patchRec.Code, patchRec.Body.String())
	}

	diagnoseRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/hosts/"+hostID+"/instances/main/diagnose", `{}`)
	if diagnoseRec.Code != http.StatusOK {
		t.Fatalf("diagnose status=%d body=%s", diagnoseRec.Code, diagnoseRec.Body.String())
	}
	diagnosePayload := decodeJSONMap(t, diagnoseRec)
	diagnoseMap, _ := diagnosePayload["diagnose"].(map[string]interface{})
	if got, _ := diagnoseMap["result"].(string); got != "drift_detected" {
		t.Fatalf("expected diagnose result drift_detected got=%q payload=%v", got, diagnosePayload)
	}

	rollbackRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/hosts/"+hostID+"/instances/main/rollback", `{"commit":"`+commit+`"}`)
	if rollbackRec.Code != http.StatusOK {
		t.Fatalf("rollback status=%d body=%s", rollbackRec.Code, rollbackRec.Body.String())
	}
	rollbackPayload := decodeJSONMap(t, rollbackRec)
	rollbackMap, _ := rollbackPayload["rollback"].(map[string]interface{})
	if rollbackMap == nil {
		t.Fatalf("missing rollback payload=%v", rollbackPayload)
	}
	if rolledBack, _ := rollbackMap["rolledBack"].(bool); !rolledBack {
		t.Fatalf("expected rolledBack=true payload=%v", rollbackPayload)
	}

	configRec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/remote/hosts/"+hostID+"/config", "")
	if configRec.Code != http.StatusOK {
		t.Fatalf("config read status=%d body=%s", configRec.Code, configRec.Body.String())
	}
	configPayload := decodeJSONMap(t, configRec)
	configMap, _ := configPayload["config"].(map[string]interface{})
	agents, _ := configMap["agents"].(map[string]interface{})
	defaults, _ := agents["defaults"].(map[string]interface{})
	model, _ := defaults["model"].(string)
	if model != "gpt-4.1" {
		t.Fatalf("expected model restored to gpt-4.1, got %q payload=%v", model, configPayload)
	}
}

func TestRemotePatchConfigCreatesMissingConfigFile(t *testing.T) {
	remoteConfig := ""
	configureSSHRunner(t, func(command string) remoteExecResult {
		switch {
		case strings.HasPrefix(command, "if [ -f \"$HOME/.openclaw/openclaw.json\" ]"):
			if strings.TrimSpace(remoteConfig) == "" {
				return remoteExecResult{ExitCode: 0, Stdout: "{}"}
			}
			return remoteExecResult{ExitCode: 0, Stdout: remoteConfig}
		case strings.Contains(command, "cat > \"$HOME/.openclaw/openclaw.json\""):
			payload := extractHeredocPayload(command)
			if strings.TrimSpace(payload) != "" {
				remoteConfig = payload
			}
			return remoteExecResult{ExitCode: 0}
		default:
			return remoteExecResult{ExitCode: 0}
		}
	})

	mux := buildRemoteFeatureMux(t)
	hostID := createRemoteHostForTests(t, mux)

	patchRec := runJSONRequest(t, mux, http.MethodPatch, "/api/v1/remote/hosts/"+hostID+"/config", `{
		"patch":{"agents":{"defaults":{"provider":"openai","model":"gpt-4.1"}}}
	}`)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("config patch status=%d body=%s", patchRec.Code, patchRec.Body.String())
	}

	configRec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/remote/hosts/"+hostID+"/config", "")
	if configRec.Code != http.StatusOK {
		t.Fatalf("config read status=%d body=%s", configRec.Code, configRec.Body.String())
	}
	configPayload := decodeJSONMap(t, configRec)
	configMap, _ := configPayload["config"].(map[string]interface{})
	agents, _ := configMap["agents"].(map[string]interface{})
	defaults, _ := agents["defaults"].(map[string]interface{})
	if got, _ := defaults["provider"].(string); got != "openai" {
		t.Fatalf("expected provider openai, got %q payload=%v", got, configPayload)
	}
	if got, _ := defaults["model"].(string); got != "gpt-4.1" {
		t.Fatalf("expected model gpt-4.1, got %q payload=%v", got, configPayload)
	}
}

func TestRemoteReadConfigReturnsCanonicalRawForJSON5(t *testing.T) {
	const rawJSON5 = `{
  // openclaw json5
  agents: { defaults: { model: "openai/gpt-4.1", }, },
}`
	configureSSHRunner(t, func(command string) remoteExecResult {
		if strings.Contains(command, "cat \"$HOME/.openclaw/openclaw.json\"") {
			return remoteExecResult{ExitCode: 0, Stdout: rawJSON5}
		}
		return remoteExecResult{ExitCode: 0}
	})

	host := RemoteHost{
		ID:          "host-json5",
		Host:        "127.0.0.1",
		Port:        22,
		User:        "ubuntu",
		AuthMode:    RemoteAuthModePrivateKey,
		KeyPath:     "~/.ssh/id_ed25519",
		RuntimeMode: RemoteRuntimeModeOnDemand,
	}
	cfg, raw, _, err := remoteReadConfig(context.Background(), host)
	if err != nil {
		t.Fatalf("remoteReadConfig(json5) error: %v", err)
	}
	if strings.TrimSpace(raw) == "" || !strings.Contains(raw, "openclaw json5") {
		t.Fatalf("expected raw json5 text, got %q", raw)
	}
	if got := strings.TrimSpace(anyToString(cfg["raw_json5"])); got == "" {
		t.Fatalf("expected canonical raw_json5 in config map, got %+v", cfg)
	}
}

func TestRemotePatchConfigUsesConfigSetWhenRemoteConfigIsJSON5(t *testing.T) {
	const rawJSON5 = `{
  // openclaw json5
  agents: { defaults: { model: "openai/gpt-4.1", }, },
}`
	usedConfigSet := 0
	configureSSHRunner(t, func(command string) remoteExecResult {
		switch {
		case strings.Contains(command, "cat \"$HOME/.openclaw/openclaw.json\""):
			return remoteExecResult{ExitCode: 0, Stdout: rawJSON5}
		case strings.Contains(command, "openclaw config set"):
			usedConfigSet++
			return remoteExecResult{ExitCode: 0}
		default:
			return remoteExecResult{ExitCode: 0}
		}
	})

	host := RemoteHost{
		ID:          "host-json5",
		Host:        "127.0.0.1",
		Port:        22,
		User:        "ubuntu",
		AuthMode:    RemoteAuthModePrivateKey,
		KeyPath:     "~/.ssh/id_ed25519",
		RuntimeMode: RemoteRuntimeModeOnDemand,
	}
	_, snapshotPath, _, err := remotePatchConfig(context.Background(), host, map[string]interface{}{
		"channels": map[string]interface{}{
			"telegram": map[string]interface{}{
				"enabled":  true,
				"botToken": "tg-123",
			},
		},
	})
	if err != nil {
		t.Fatalf("remotePatchConfig(json5) error: %v", err)
	}
	if usedConfigSet == 0 {
		t.Fatal("expected openclaw config set to be used for json5 patch path")
	}
	if !strings.Contains(snapshotPath, "$HOME/.openclaw/snapshots/openclaw-") {
		t.Fatalf("unexpected snapshot path: %q", snapshotPath)
	}
}

func TestRemoteCodeAgentLifecycleAndAudit(t *testing.T) {
	auditPath := filepath.Join(t.TempDir(), "gateway-audit.jsonl")
	t.Setenv("CARRIER_GATEWAY_AUDIT_LOG", auditPath)

	sshCalls := 0
	configureSSHRunner(t, func(command string) remoteExecResult {
		sshCalls++
		switch {
		case strings.Contains(command, "command -v codex"):
			return remoteExecResult{ExitCode: 0}
		case strings.Contains(command, "command -v opencode"):
			return remoteExecResult{ExitCode: 0}
		case strings.Contains(command, "codex") && strings.Contains(command, "--version"):
			return remoteExecResult{ExitCode: 0, Stdout: "codex 1.0.0"}
		case strings.Contains(command, "opencode") && strings.Contains(command, "--version"):
			return remoteExecResult{ExitCode: 0, Stdout: "opencode 1.0.0"}
		case strings.Contains(command, "codex") && strings.Contains(command, "exec"):
			return remoteExecResult{ExitCode: 0, Stdout: `{"ok":true}`}
		default:
			return remoteExecResult{ExitCode: 0}
		}
	})

	mux := buildRemoteFeatureMux(t)
	hostID := createRemoteHostForTests(t, mux)

	installRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/hosts/"+hostID+"/instances/main/codeagent/install", `{
		"backend":"codex",
		"workspaceRoot":"/workspace"
	}`)
	if installRec.Code != http.StatusOK {
		t.Fatalf("codeagent install status=%d body=%s", installRec.Code, installRec.Body.String())
	}

	versionRec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/remote/hosts/"+hostID+"/instances/main/codeagent/version?backend=opencode", "")
	if versionRec.Code != http.StatusOK {
		t.Fatalf("codeagent version status=%d body=%s", versionRec.Code, versionRec.Body.String())
	}
	versionPayload := decodeJSONMap(t, versionRec)
	versionMap, _ := versionPayload["version"].(map[string]interface{})
	if backend, _ := versionMap["backend"].(string); backend != "opencode" {
		t.Fatalf("expected backend opencode in version payload=%v", versionPayload)
	}

	runRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/hosts/"+hostID+"/instances/main/codeagent/run", `{
		"backend":"codex",
		"workspaceRoot":"/workspace",
		"capability":"run_shell",
		"command":"ls -la"
	}`)
	if runRec.Code != http.StatusOK {
		t.Fatalf("codeagent run status=%d body=%s", runRec.Code, runRec.Body.String())
	}
	runPayload := decodeJSONMap(t, runRec)
	runMap, _ := runPayload["run"].(map[string]interface{})
	resultMap, _ := runMap["result"].(map[string]interface{})
	if ok, _ := resultMap["ok"].(bool); !ok {
		t.Fatalf("expected codeagent run ok=true payload=%v", runPayload)
	}
	if decision, _ := resultMap["policy_decision"].(string); decision != "allow" {
		t.Fatalf("expected policy allow payload=%v", runPayload)
	}
	if estimate, _ := resultMap["cost_estimate_usd"].(float64); estimate <= 0 {
		t.Fatalf("expected positive cost estimate payload=%v", runPayload)
	}

	beforeDangerCallCount := sshCalls
	denyRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/hosts/"+hostID+"/instances/main/codeagent/run", `{
		"backend":"codex",
		"workspaceRoot":"/workspace",
		"capability":"run_shell",
		"command":"rm -rf /"
	}`)
	if denyRec.Code != http.StatusOK {
		t.Fatalf("codeagent deny run status=%d body=%s", denyRec.Code, denyRec.Body.String())
	}
	denyPayload := decodeJSONMap(t, denyRec)
	denyRun, _ := denyPayload["run"].(map[string]interface{})
	denyResult, _ := denyRun["result"].(map[string]interface{})
	if decision, _ := denyResult["policy_decision"].(string); decision != "deny" {
		t.Fatalf("expected deny decision payload=%v", denyPayload)
	}
	if sshCalls != beforeDangerCallCount {
		t.Fatalf("expected denied command to avoid ssh execution, before=%d after=%d", beforeDangerCallCount, sshCalls)
	}

	rawAudit, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("read audit file failed: %v", err)
	}
	text := string(rawAudit)
	if !strings.Contains(text, `"action":"remote_codeagent_run"`) {
		t.Fatalf("expected remote_codeagent_run audit entry, audit=%s", text)
	}
}
