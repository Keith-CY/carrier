package gateway

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"carrier/shared/openclawcfg"
)

func TestExtractChatResponseText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload map[string]interface{}
		want    string
	}{
		{
			name:    "direct message",
			payload: map[string]interface{}{"message": "hello"},
			want:    "hello",
		},
		{
			name: "payload text array",
			payload: map[string]interface{}{
				"payloads": []interface{}{
					map[string]interface{}{"text": "hey BOSS 👋"},
				},
			},
			want: "hey BOSS 👋",
		},
		{
			name: "choices message content",
			payload: map[string]interface{}{
				"choices": []interface{}{
					map[string]interface{}{
						"message": map[string]interface{}{"content": "from choices"},
					},
				},
			},
			want: "from choices",
		},
		{
			name: "nested output content parts",
			payload: map[string]interface{}{
				"output": map[string]interface{}{
					"content": []interface{}{
						map[string]interface{}{"type": "text", "text": "line one"},
						map[string]interface{}{"type": "text", "text": "line two"},
					},
				},
			},
			want: "line one\nline two",
		},
		{
			name: "no chat text fields",
			payload: map[string]interface{}{
				"meta": map[string]interface{}{"model": "gpt-5.3-codex"},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractChatResponseText(tt.payload)
			if got != tt.want {
				t.Fatalf("extractChatResponseText()=%q want %q", got, tt.want)
			}
		})
	}
}

func TestParseRemoteHostPlatform(t *testing.T) {
	t.Parallel()

	got := parseRemoteHostPlatform("CARRIER_PLATFORM_PROBE\nOS=Linux\nDISTRO=ubuntu\nVERSION=22.04\n")
	if got.OS != "Linux" {
		t.Fatalf("os=%q, want Linux", got.OS)
	}
	if got.Distro != "ubuntu" {
		t.Fatalf("distro=%q, want ubuntu", got.Distro)
	}
	if got.Version != "22.04" {
		t.Fatalf("version=%q, want 22.04", got.Version)
	}
}

func TestDetectRemoteHostPlatformRejectsAlpine(t *testing.T) {
	configureSSHRunner(t, func(command string) remoteExecResult {
		switch {
		case strings.Contains(command, "CARRIER_PLATFORM_PROBE"):
			return remoteExecResult{
				ExitCode: 0,
				Stdout:   "CARRIER_PLATFORM_PROBE\nOS=Linux\nDISTRO=alpine\nVERSION=3.19\n",
			}
		default:
			return remoteExecResult{ExitCode: 0}
		}
	})

	platform, _, err := detectRemoteHostPlatform(context.Background(), RemoteHost{
		ID:      "h1",
		Name:    "h1",
		Host:    "127.0.0.1",
		Port:    22,
		User:    "carrier",
		KeyPath: "~/.ssh/id_ed25519",
	})
	if err != nil {
		t.Fatalf("detectRemoteHostPlatform() error: %v", err)
	}
	if platform.Supported {
		t.Fatalf("expected unsupported platform for alpine, got %+v", platform)
	}
	if !strings.Contains(strings.ToLower(platform.Reason), "alpine") {
		t.Fatalf("expected alpine reason, got %q", platform.Reason)
	}
}

func TestWrapRemoteCommandWithMemoryContractPreservesHomeExpansion(t *testing.T) {
	t.Parallel()

	contract := remoteMemoryRuntimeContract{
		ContractID:     "cid",
		ContractDigest: "digest",
		MemoryPath:     "$HOME/.openclaw/agents/main/memory",
	}
	got := wrapRemoteCommandWithMemoryContract("echo ok", contract)
	if !strings.Contains(got, `mem_path="$HOME"/'.openclaw/agents/main/memory'`) {
		t.Fatalf("expected HOME expansion-safe assignment, got: %s", got)
	}
	if strings.Contains(got, `mem_path='$HOME/.openclaw/agents/main/memory'`) {
		t.Fatalf("expected not to single-quote full $HOME path, got: %s", got)
	}
}

func TestShellPathValuePreservingHomeExpansion(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: "''"},
		{name: "whitespace", in: "  ", want: "''"},
		{name: "home only", in: "$HOME", want: `"$HOME"`},
		{name: "home with slash", in: "$HOME/", want: `"$HOME"`},
		{name: "home subpath", in: "$HOME/.picoclaw/memory", want: `"$HOME"/'.picoclaw/memory'`},
		{name: "home subpath with single quote", in: "$HOME/foo'bar", want: `"$HOME"/'foo'\''bar'`},
		{name: "plain path", in: "/tmp/memory path", want: `'/tmp/memory path'`},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := shellPathValuePreservingHomeExpansion(tc.in); got != tc.want {
				t.Fatalf("shellPathValuePreservingHomeExpansion(%q)=%q want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRunRemoteRsyncBuildsSSHTransportForPrivateKeyHost(t *testing.T) {
	orig := remoteRsyncRunner
	defer func() { remoteRsyncRunner = orig }()

	var captured []string
	remoteRsyncRunner = func(_ context.Context, args []string) (remoteExecResult, error) {
		captured = append([]string{}, args...)
		return remoteExecResult{ExitCode: 0}, nil
	}

	host := RemoteHost{
		ID:       "h1",
		Name:     "h1",
		Host:     "198.51.100.10",
		Port:     2222,
		User:     "ubuntu",
		AuthMode: RemoteAuthModePrivateKey,
		KeyPath:  "~/.ssh/id_ed25519",
	}
	if _, err := runRemoteRsync(context.Background(), host, "/tmp/local-secrets.json", remoteOpenClawCarrierSecretsPath); err != nil {
		t.Fatalf("runRemoteRsync error: %v", err)
	}
	if len(captured) == 0 {
		t.Fatal("expected rsync args to be captured")
	}
	joined := strings.Join(captured, " ")
	if !strings.Contains(joined, "--rsync-path") || !strings.Contains(joined, "mkdir -p") {
		t.Fatalf("expected --rsync-path mkdir command in rsync args, got %q", joined)
	}
	expectedRemotePath := normalizeRemoteRsyncPath(remoteOpenClawCarrierSecretsPath)
	if !strings.Contains(joined, "ubuntu@198.51.100.10:"+expectedRemotePath) {
		t.Fatalf("expected destination target in rsync args, got %q", joined)
	}
	if !strings.Contains(joined, "IdentitiesOnly=yes") || !strings.Contains(joined, "id_ed25519") {
		t.Fatalf("expected private key ssh options in rsync args, got %q", joined)
	}
}

func TestRemoteWriteOpenClawCarrierSecretsUsesRsyncPayload(t *testing.T) {
	orig := remoteRsyncRunner
	defer func() { remoteRsyncRunner = orig }()

	var (
		capturedArgs []string
		payload      map[string]interface{}
	)
	remoteRsyncRunner = func(_ context.Context, args []string) (remoteExecResult, error) {
		capturedArgs = append([]string{}, args...)
		if len(args) < 2 {
			t.Fatalf("unexpected rsync args: %v", args)
		}
		raw, err := os.ReadFile(args[len(args)-2])
		if err != nil {
			t.Fatalf("read rsync source file: %v", err)
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("parse rsync payload json: %v", err)
		}
		return remoteExecResult{ExitCode: 0}, nil
	}

	host := RemoteHost{
		ID:            "h2",
		Name:          "h2",
		Host:          "203.0.113.5",
		User:          "carrier",
		AuthMode:      RemoteAuthModeSSHConfig,
		SSHConfigHost: "carrier-h2",
	}
	input := map[string]interface{}{
		"providers": map[string]interface{}{
			"openai": map[string]interface{}{
				"apiKey": "sk-openai-rsync",
			},
		},
	}
	res, err := remoteWriteOpenClawCarrierSecrets(context.Background(), host, input)
	if err != nil {
		t.Fatalf("remoteWriteOpenClawCarrierSecrets error: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %+v", res)
	}
	if len(capturedArgs) == 0 {
		t.Fatal("expected rsync invocation")
	}
	dest := capturedArgs[len(capturedArgs)-1]
	if !strings.HasSuffix(dest, ":"+normalizeRemoteRsyncPath(remoteOpenClawCarrierSecretsPath)) {
		t.Fatalf("unexpected rsync destination: %q", dest)
	}
	providers, _ := payload["providers"].(map[string]interface{})
	openai, _ := providers["openai"].(map[string]interface{})
	if strings.TrimSpace(anyToString(openai["apiKey"])) != "sk-openai-rsync" {
		t.Fatalf("unexpected secrets payload: %+v", payload)
	}
}

func TestBuildRemoteProviderProfilePatchWritesCompleteOpenClawProviderConfig(t *testing.T) {
	patch := buildRemoteProviderProfilePatch(ProviderProfile{
		Provider: "openrouter",
		Model:    "openai/gpt-4o-mini",
		AuthRef:  "sk-openrouter-live",
	}, "assistant")

	agents, ok := patch["agents"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected agents patch, got %#v", patch["agents"])
	}
	defaults, ok := agents["defaults"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected agents.defaults patch, got %#v", agents["defaults"])
	}
	defaultModel, ok := defaults["model"].(map[string]interface{})
	if !ok || strings.TrimSpace(anyToString(defaultModel["primary"])) != "openai/gpt-4o-mini" {
		t.Fatalf("unexpected default model patch: %#v", defaults["model"])
	}
	overrides, ok := agents["overrides"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected agent overrides patch, got %#v", agents["overrides"])
	}
	override, ok := overrides["assistant"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected assistant override, got %#v", overrides)
	}
	overrideModel, ok := override["model"].(map[string]interface{})
	if !ok || strings.TrimSpace(anyToString(overrideModel["primary"])) != "openai/gpt-4o-mini" {
		t.Fatalf("unexpected override model patch: %#v", override["model"])
	}

	models, ok := patch["models"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected models patch, got %#v", patch["models"])
	}
	providers, ok := models["providers"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected models.providers patch, got %#v", models["providers"])
	}
	openrouter, ok := providers["openrouter"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected openrouter provider patch, got %#v", providers["openrouter"])
	}
	if got := strings.TrimSpace(anyToString(openrouter["baseUrl"])); got != "https://openrouter.ai/api/v1" {
		t.Fatalf("openrouter baseUrl=%q, want https://openrouter.ai/api/v1", got)
	}
	providerModels, ok := openrouter["models"].([]interface{})
	if !ok || len(providerModels) != 1 {
		t.Fatalf("expected one provider model entry, got %#v", openrouter["models"])
	}
	providerModel, _ := providerModels[0].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(providerModel["id"])); got != "gpt-4o-mini" {
		t.Fatalf("provider model id=%q, want gpt-4o-mini", got)
	}
	apiKeyRef, ok := openrouter["apiKey"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected apiKey object, got %#v", openrouter["apiKey"])
	}
	if got := strings.TrimSpace(anyToString(apiKeyRef["provider"])); got != "carrier_file" {
		t.Fatalf("apiKey.provider=%q, want carrier_file", got)
	}

	secrets, ok := patch["secrets"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected secrets patch, got %#v", patch["secrets"])
	}
	secretProviders, ok := secrets["providers"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected secrets.providers patch, got %#v", secrets["providers"])
	}
	carrierFile, ok := secretProviders["carrier_file"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected carrier_file provider patch, got %#v", secretProviders["carrier_file"])
	}
	if got := strings.TrimSpace(anyToString(carrierFile["path"])); got != "./carrier-secrets.json" {
		t.Fatalf("carrier_file.path=%q, want ./carrier-secrets.json", got)
	}

	secretsPatch, ok := patch[openclawcfg.CarrierSecretFilePatchKey].(map[string]interface{})
	if !ok {
		t.Fatalf("expected carrier secret file patch, got %#v", patch[openclawcfg.CarrierSecretFilePatchKey])
	}
	rawProviders, ok := secretsPatch["providers"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected secret providers patch, got %#v", secretsPatch["providers"])
	}
	rawOpenRouter, ok := rawProviders["openrouter"].(map[string]interface{})
	if !ok || strings.TrimSpace(anyToString(rawOpenRouter["apiKey"])) != "sk-openrouter-live" {
		t.Fatalf("unexpected openrouter secret patch: %#v", rawProviders["openrouter"])
	}
}

func TestNormalizeRemoteRsyncPathConvertsHomePrefix(t *testing.T) {
	t.Parallel()

	if got := normalizeRemoteRsyncPath("$HOME/.openclaw/workspace/carrier-secrets.json"); got != "~/.openclaw/workspace/carrier-secrets.json" {
		t.Fatalf("normalizeRemoteRsyncPath()=%q, want ~/.openclaw/workspace/carrier-secrets.json", got)
	}
	if got := normalizeRemoteRsyncPath("/var/lib/carrier-secrets.json"); got != "/var/lib/carrier-secrets.json" {
		t.Fatalf("normalizeRemoteRsyncPath()=%q, want /var/lib/carrier-secrets.json", got)
	}
}

func TestRemoteInstallReleaseTagPrefersManagedVersionAndFallsBack(t *testing.T) {
	resetManagedCompatLockForTests()
	t.Cleanup(func() {
		resetManagedCompatLockForTests()
	})

	lockPath := filepath.Join(t.TempDir(), "upstreams.lock.json")
	lock := `{"schema_version":1,"agents":{"zeroclaw":{"repository":"zeroclaw-labs/zeroclaw","recommended_version":"1.2.3","supported_renderers":[{"id":"zeroclaw.toml.v1","version_range":">=0.1.0","config_format":"toml","config_path":"~/.zeroclaw/config.toml"}]}}}`
	if err := os.WriteFile(lockPath, []byte(lock), 0o600); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	t.Setenv("CARRIER_UPSTREAM_LOCK_PATH", lockPath)

	managedVersion := remoteInstallReleaseTag("zeroclaw", "v9.9.9")
	if managedVersion != "v1.2.3" {
		t.Fatalf("expected v-prefixed managed version, got %q", managedVersion)
	}

	fallbackVersion := remoteInstallReleaseTag("ghost-claw", "v9.9.9")
	if fallbackVersion != "v9.9.9" {
		t.Fatalf("expected fallback version for unknown agent, got %q", fallbackVersion)
	}
}

func TestRemoteInstallPicoClawAndStreaming(t *testing.T) {
	configureSSHRunner(t, func(command string) remoteExecResult {
		if strings.Contains(command, "picoclaw_Linux") {
			return remoteExecResult{ExitCode: 0}
		}
		return remoteExecResult{ExitCode: 0}
	})

	res, err := remoteInstallPicoClaw(context.Background(), RemoteHost{
		Host:        "127.0.0.1",
		Port:        22,
		User:        "carrier",
		AuthMode:    RemoteAuthModePrivateKey,
		KeyPath:     "/tmp/id_ed25519",
		RuntimeMode: RemoteRuntimeModeOnDemand,
	}, "h1", "picoclaw", false)
	if err != nil {
		t.Fatalf("remoteInstallPicoClaw()=%v", err)
	}
	if !res.Installed {
		t.Fatal("expected picoclaw install installed=true")
	}

	chunkCalls := 0
	configureSSHStreamRunner(t, func(command string, onChunk func(remoteStreamChunk)) remoteExecResult {
		if strings.Contains(command, "picoclaw_Linux") && onChunk != nil {
			onChunk(remoteStreamChunk{Stream: "stdout", Text: "download"})
			onChunk(remoteStreamChunk{Stream: "stdout", Text: "done"})
		}
		return remoteExecResult{ExitCode: 0, Stdout: "download\ndone"}
	})
	streamRes, streamErr := remoteInstallPicoClawStreaming(
		context.Background(),
		RemoteHost{Host: "127.0.0.1", Port: 22, User: "carrier", AuthMode: RemoteAuthModePrivateKey, KeyPath: "/tmp/id_ed25519", RuntimeMode: RemoteRuntimeModeOnDemand},
		"h1",
		"picoclaw",
		false,
		func(_ remoteStreamChunk) {
			chunkCalls++
		},
	)
	if streamErr != nil {
		t.Fatalf("remoteInstallPicoClawStreaming()=%v", streamErr)
	}
	if !streamRes.Installed {
		t.Fatal("expected streaming install installed=true")
	}
	if chunkCalls != 2 {
		t.Fatalf("expected 2 stream chunks for picoclaw install, got %d", chunkCalls)
	}
}

func TestRemoteInstallZeroClawAndStreaming(t *testing.T) {
	configureSSHRunner(t, func(command string) remoteExecResult {
		if strings.Contains(command, "zeroclaw-") {
			return remoteExecResult{ExitCode: 0}
		}
		return remoteExecResult{ExitCode: 0}
	})

	res, err := remoteInstallZeroClaw(context.Background(), RemoteHost{
		Host:        "127.0.0.1",
		Port:        22,
		User:        "carrier",
		AuthMode:    RemoteAuthModePrivateKey,
		KeyPath:     "/tmp/id_ed25519",
		RuntimeMode: RemoteRuntimeModeOnDemand,
	}, "h1", "zeroclaw", false)
	if err != nil {
		t.Fatalf("remoteInstallZeroClaw()=%v", err)
	}
	if !res.Installed {
		t.Fatal("expected zeroclaw install installed=true")
	}

	chunkCalls := 0
	configureSSHStreamRunner(t, func(command string, onChunk func(remoteStreamChunk)) remoteExecResult {
		if strings.Contains(command, "zeroclaw_") || strings.Contains(command, "zeroclaw") {
			if onChunk != nil {
				onChunk(remoteStreamChunk{Stream: "stdout", Text: "installed"})
			}
		}
		return remoteExecResult{ExitCode: 0}
	})
	streamRes, streamErr := remoteInstallZeroClawStreaming(context.Background(), RemoteHost{
		Host:        "127.0.0.1",
		Port:        22,
		User:        "carrier",
		AuthMode:    RemoteAuthModePrivateKey,
		KeyPath:     "/tmp/id_ed25519",
		RuntimeMode: RemoteRuntimeModeOnDemand,
	}, "h1", "zeroclaw", false, func(_ remoteStreamChunk) {
		chunkCalls++
	})
	if streamErr != nil {
		t.Fatalf("remoteInstallZeroClawStreaming()=%v", streamErr)
	}
	if !streamRes.Installed {
		t.Fatal("expected zero claw streaming install installed=true")
	}
	if chunkCalls != 1 {
		t.Fatalf("expected 1 stream chunk for zeroclaw install, got %d", chunkCalls)
	}
}

func TestRemoteInstallZeroClawStreamingAcceptsNilCallback(t *testing.T) {
	configureSSHRunner(t, func(command string) remoteExecResult {
		return remoteExecResult{ExitCode: 0, Stdout: "ok"}
	})
	configureSSHStreamRunner(t, func(command string, onChunk func(remoteStreamChunk)) remoteExecResult {
		if strings.Contains(command, "zeroclaw_") || strings.Contains(command, "zeroclaw") {
			if onChunk != nil {
				t.Fatal("expected nil callback to be accepted")
			}
		}
		return remoteExecResult{ExitCode: 0, Stdout: "ok"}
	})

	streamRes, streamErr := remoteInstallZeroClawStreaming(context.Background(), RemoteHost{
		Host:        "127.0.0.1",
		Port:        22,
		User:        "carrier",
		AuthMode:    RemoteAuthModePrivateKey,
		KeyPath:     "/tmp/id_ed25519",
		RuntimeMode: RemoteRuntimeModeOnDemand,
	}, "h1", "zeroclaw", false, nil)
	if streamErr != nil {
		t.Fatalf("remoteInstallZeroClawStreaming()=%v", streamErr)
	}
	if !streamRes.Installed {
		t.Fatal("expected zero claw streaming install installed=true")
	}
}

func TestRemoteGetLogsClampTailBounds(t *testing.T) {
	var captured []string
	configureSSHRunner(t, func(command string) remoteExecResult {
		captured = append(captured, command)
		switch {
		case strings.Contains(command, "tail -n 200"):
			return remoteExecResult{ExitCode: 0, Stdout: ""}
		case strings.Contains(command, "tail -n 2000"):
			return remoteExecResult{ExitCode: 0, Stdout: "logs"}
		default:
			return remoteExecResult{ExitCode: 1, Stderr: "unexpected command"}
		}
	})

	host := RemoteHost{Host: "127.0.0.1", Port: 22, User: "carrier", AuthMode: RemoteAuthModePrivateKey, KeyPath: "/tmp/id_ed25519", RuntimeMode: RemoteRuntimeModeOnDemand}

	if _, _, err := remoteGetLogs(context.Background(), host, "main", 0); err != nil {
		t.Fatalf("remoteGetLogs default-tail returned error: %v", err)
	}

	if _, _, err := remoteGetLogs(context.Background(), host, "main", 3000); err != nil {
		t.Fatalf("remoteGetLogs max-tail returned error: %v", err)
	}
	if len(captured) != 2 {
		t.Fatalf("expected two log commands, got %d", len(captured))
	}
	if !strings.Contains(captured[0], "tail -n 200") {
		t.Fatalf("expected default tail command to clamp to 200, command=%q", captured[0])
	}
	if !strings.Contains(captured[1], "tail -n 2000") {
		t.Fatalf("expected max tail command to clamp to 2000, command=%q", captured[1])
	}
}

func TestRemoteGetLogsTailClampsLowerBoundForGatewayOnly(t *testing.T) {
	var capturedTail []string
	configureSSHRunner(t, func(command string) remoteExecResult {
		capturedTail = append(capturedTail, command)
		return remoteExecResult{ExitCode: 0, Stdout: "logs"}
	})

	host := RemoteHost{Host: "127.0.0.1", Port: 22, User: "carrier", AuthMode: RemoteAuthModePrivateKey, KeyPath: "/tmp/id_ed25519", RuntimeMode: RemoteRuntimeModeOnDemand}

	if _, _, err := remoteGetLogs(context.Background(), host, "main", -1); err != nil {
		t.Fatalf("remoteGetLogs default-tail returned error: %v", err)
	}
	if len(capturedTail) == 0 {
		t.Fatal("expected captured command for tail clamp")
	}
	if !strings.Contains(capturedTail[len(capturedTail)-1], "tail -n 200") {
		t.Fatalf("expected lower bound clamp to 200, command=%q", capturedTail[len(capturedTail)-1])
	}
}

func TestRemoteGetLogsFailurePropagatesCommandError(t *testing.T) {
	configureSSHRunner(t, func(command string) remoteExecResult {
		return remoteExecResult{ExitCode: 1, Stderr: "failed"}
	})

	_, _, err := remoteGetLogs(context.Background(), RemoteHost{
		Host:        "127.0.0.1",
		Port:        22,
		User:        "carrier",
		AuthMode:    RemoteAuthModePrivateKey,
		KeyPath:     "/tmp/id_ed25519",
		RuntimeMode: RemoteRuntimeModeOnDemand,
	}, "main", 10)
	if err == nil {
		t.Fatal("expected remoteGetLogs to fail")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "fetch remote logs") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRemotePatchPicoClawConfigMergesAndWritesSnapshot(t *testing.T) {
	writeCommand := ""
	configureSSHRunner(t, func(command string) remoteExecResult {
		switch {
		case strings.Contains(command, "cat \"$HOME/.picoclaw/config.json\""):
			return remoteExecResult{ExitCode: 0, Stdout: `{"provider":"openai","nested":{"a":"1"}}`}
		case strings.Contains(command, "cat > \"$HOME/.picoclaw/config.json\""):
			writeCommand = command
			return remoteExecResult{ExitCode: 0}
		default:
			return remoteExecResult{ExitCode: 0}
		}
	})

	merged, snapshot, _, err := remotePatchPicoClawConfig(context.Background(), RemoteHost{
		Host:        "127.0.0.1",
		Port:        22,
		User:        "carrier",
		AuthMode:    RemoteAuthModePrivateKey,
		KeyPath:     "/tmp/id_ed25519",
		RuntimeMode: RemoteRuntimeModeOnDemand,
	}, map[string]interface{}{
		"nested":   map[string]interface{}{"b": "2"},
		"provider": "openai",
	})
	if err != nil {
		t.Fatalf("remotePatchPicoClawConfig()=%v", err)
	}
	if !strings.HasPrefix(snapshot, "$HOME/.picoclaw/snapshots/picoclaw-") {
		t.Fatalf("unexpected snapshot path %q", snapshot)
	}
	if writeCommand == "" {
		t.Fatal("expected picoclaw write command")
	}
	nestedRaw, ok := merged["nested"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested merged map, got %#v", merged["nested"])
	}
	if nestedRaw["a"] != "1" || nestedRaw["b"] != "2" {
		t.Fatalf("unexpected merged nested config: %#v", nestedRaw)
	}
}

func TestRemotePatchZeroClawConfigRequiresRawTomlAndWrites(t *testing.T) {
	configureSSHRunner(t, func(command string) remoteExecResult {
		switch {
		case strings.Contains(command, "cat \"$HOME/.zeroclaw/config.toml\""):
			return remoteExecResult{ExitCode: 0, Stdout: "api_key = \"old\""}
		case strings.Contains(command, "cat > \"$HOME/.zeroclaw/config.toml\""):
			return remoteExecResult{ExitCode: 0}
		default:
			return remoteExecResult{ExitCode: 0}
		}
	})

	patched, snapshot, _, err := remotePatchZeroClawConfig(context.Background(), RemoteHost{
		Host:        "127.0.0.1",
		Port:        22,
		User:        "carrier",
		AuthMode:    RemoteAuthModePrivateKey,
		KeyPath:     "/tmp/id_ed25519",
		RuntimeMode: RemoteRuntimeModeOnDemand,
	}, map[string]interface{}{"raw_toml": "api_key = \"new\""})
	if err != nil {
		t.Fatalf("remotePatchZeroClawConfig()=%v", err)
	}
	if !strings.HasPrefix(snapshot, "$HOME/.zeroclaw/snapshots/zeroclaw-") {
		t.Fatalf("unexpected snapshot path: %q", snapshot)
	}
	if strings.TrimSpace(anyToString(patched["raw_toml"])) != "api_key = \"new\"" {
		t.Fatalf("unexpected patched zeroclaw raw_toml: %#v", patched)
	}

	if _, _, _, err := remotePatchZeroClawConfig(context.Background(), RemoteHost{
		Host:        "127.0.0.1",
		Port:        22,
		User:        "carrier",
		AuthMode:    RemoteAuthModePrivateKey,
		KeyPath:     "/tmp/id_ed25519",
		RuntimeMode: RemoteRuntimeModeOnDemand,
	}, map[string]interface{}{"foo": "bar"}); err == nil {
		t.Fatal("expected remotePatchZeroClawConfig to fail when raw_toml missing")
	}
}

func TestRemoteListSessionsParsesSessionLines(t *testing.T) {
	configureSSHRunner(t, func(command string) remoteExecResult {
		return remoteExecResult{ExitCode: 0, Stdout: "s1\tsessions\t12\t1700000000\t/home/main/sessions/s1.jsonl\ninvalidline\ns2\tsessions_archive\t5\t1700000001\t/home/main/sessions_archive/s2.jsonl"}
	})

	entries, steps, err := remoteListSessions(context.Background(), RemoteHost{
		Host:        "127.0.0.1",
		Port:        22,
		User:        "carrier",
		AuthMode:    RemoteAuthModePrivateKey,
		KeyPath:     "/tmp/id_ed25519",
		RuntimeMode: RemoteRuntimeModeOnDemand,
	}, "main")
	if err != nil {
		t.Fatalf("remoteListSessions()=%v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 parsed entries, got %d", len(entries))
	}
	if len(steps) == 0 {
		t.Fatal("expected steps to include command result")
	}

	configureSSHRunner(t, func(command string) remoteExecResult {
		return remoteExecResult{ExitCode: 1, Stderr: "bad"}
	})
	if _, _, err := remoteListSessions(context.Background(), RemoteHost{
		Host:        "127.0.0.1",
		Port:        22,
		User:        "carrier",
		AuthMode:    RemoteAuthModePrivateKey,
		KeyPath:     "/tmp/id_ed25519",
		RuntimeMode: RemoteRuntimeModeOnDemand,
	}, "main"); err == nil {
		t.Fatal("expected remoteListSessions error")
	}
}

func TestRemoteArchiveSessionAndDeleteSessionActions(t *testing.T) {
	configureSSHRunner(t, func(command string) remoteExecResult {
		if strings.Contains(command, "mv") {
			return remoteExecResult{ExitCode: 0}
		}
		if strings.Contains(command, "rm -f") {
			return remoteExecResult{ExitCode: 0}
		}
		return remoteExecResult{ExitCode: 0}
	})

	host := RemoteHost{Host: "127.0.0.1", Port: 22, User: "carrier", AuthMode: RemoteAuthModePrivateKey, KeyPath: "/tmp/id_ed25519", RuntimeMode: RemoteRuntimeModeOnDemand}
	if _, err := remoteArchiveSession(context.Background(), host, "main", "sess-1"); err != nil {
		t.Fatalf("remoteArchiveSession success returned error: %v", err)
	}

	if _, err := remoteDeleteSession(context.Background(), host, "main", "sess-1"); err != nil {
		t.Fatalf("remoteDeleteSession success returned error: %v", err)
	}

	configureSSHRunner(t, func(command string) remoteExecResult {
		return remoteExecResult{ExitCode: 44}
	})
	if _, err := remoteArchiveSession(context.Background(), host, "main", "sess-missing"); err == nil {
		t.Fatal("expected remoteArchiveSession not-found error")
	}

	configureSSHRunner(t, func(command string) remoteExecResult {
		return remoteExecResult{ExitCode: 1}
	})
	if _, err := remoteDeleteSession(context.Background(), host, "main", "sess-fail"); err == nil {
		t.Fatal("expected remoteDeleteSession failure error")
	}
}
