package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"carrier/baseagent"
	"carrier/daemon/internal/lifecycle"
)

var managedAgentHTTPClient = &http.Client{Timeout: 60 * time.Second}
var (
	managedExecCommandContext = exec.CommandContext
	managedANSISequence       = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
	managedStructuredLogLine  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\S+\s+(TRACE|DEBUG|INFO|WARN|ERROR)\b`)
)

type zeroclawGatewayConfig struct {
	Host           string
	Port           int
	RequirePairing bool
}

type zeroclawProviderProfile struct {
	SectionName string
	ModelAlias  string
	Model       string
	Provider    string
	ProviderID  string
}

type zeroclawLocalConfig struct {
	Raw             []byte
	DefaultProvider string
	DefaultModel    string
	Gateway         zeroclawGatewayConfig
	Profiles        []zeroclawProviderProfile
}

type managedZeroClawModelSelection struct {
	RequestedAlias    string
	RequestedModel    string
	ResolvedModel     string
	ResolvedProfile   string
	FallbackGroup     string
	SelectionStrategy string
	SelectionOrdinal  int
	OverrideHit       bool
	FallbackHit       bool
	cursorGroup       string
	nextCursor        int
}

func maybeProxyManagedAgentChat(ctx context.Context, svc *lifecycle.Service, agentID string, provider string, modelAlias string, model string, message string) (baseagent.ChatResponse, bool, error) {
	switch strings.ToLower(strings.TrimSpace(agentID)) {
	case "zeroclaw":
		cfg, err := loadLocalZeroClawConfig()
		if err != nil {
			if os.IsNotExist(err) {
				return baseagent.ChatResponse{}, false, nil
			}
			return baseagent.ChatResponse{}, true, err
		}
		if cfg.Gateway.RequirePairing {
			return baseagent.ChatResponse{}, true, fmt.Errorf("zeroclaw local gateway still requires pairing")
		}
		var cliErr error
		if svc != nil {
			if state, stateErr := svc.Status(agentID); stateErr == nil {
				if reply, handled, err := maybeProxyManagedZeroClawAgentCLI(ctx, state, cfg, provider, modelAlias, model, message); handled {
					if err == nil {
						return reply, true, nil
					}
					cliErr = err
				}
			}
		}
		reply, err := proxyZeroClawWebhook(ctx, cfg.Gateway, message)
		if err != nil && cliErr != nil {
			return baseagent.ChatResponse{}, true, fmt.Errorf("%v; webhook fallback failed: %w", cliErr, err)
		}
		return reply, true, err
	default:
		return baseagent.ChatResponse{}, false, nil
	}
}

func maybeProxyManagedZeroClawAgentCLI(ctx context.Context, state lifecycle.AgentState, cfg zeroclawLocalConfig, provider string, modelAlias string, model string, message string) (baseagent.ChatResponse, bool, error) {
	if !state.Isolated {
		return baseagent.ChatResponse{}, false, nil
	}
	instanceName := strings.TrimSpace(state.LimaInstanceName)
	if instanceName == "" {
		return baseagent.ChatResponse{}, false, nil
	}
	selection, err := resolveManagedZeroClawModelSelection(agentIDFromStateOrDefault(state, "zeroclaw"), cfg, provider, modelAlias, model)
	if err != nil {
		return baseagent.ChatResponse{}, true, err
	}
	overrideConfig, err := buildManagedZeroClawModelOverride(cfg, selection.ResolvedModel)
	if err != nil {
		return baseagent.ChatResponse{}, true, err
	}
	reply, err := runManagedZeroClawAgentCLI(ctx, instanceName, provider, message, overrideConfig)
	if err != nil {
		return baseagent.ChatResponse{}, true, err
	}
	_ = persistManagedAgentModelRuntime(agentIDFromStateOrDefault(state, "zeroclaw"), selection)
	return reply, true, nil
}

func agentIDFromStateOrDefault(state lifecycle.AgentState, fallback string) string {
	if id := strings.TrimSpace(state.ID); id != "" {
		return id
	}
	return strings.TrimSpace(fallback)
}

func runManagedZeroClawAgentCLI(ctx context.Context, instanceName string, provider string, message string, overrideConfigB64 string) (baseagent.ChatResponse, error) {
	limactlPath, err := resolveManagedLimaCtlPath()
	if err != nil {
		return baseagent.ChatResponse{}, err
	}
	safeInstance := strings.TrimSpace(instanceName)
	if safeInstance == "" {
		return baseagent.ChatResponse{}, fmt.Errorf("zeroclaw managed instance name is empty")
	}
	guestScript := `set -e
if [ -x "$HOME/.local/bin/zeroclaw" ]; then
  ZC="$HOME/.local/bin/zeroclaw"
else
  ZC="zeroclaw"
fi
CONFIG_DIR="$HOME/.zeroclaw"
if [ -n "$3" ]; then
  TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/carrier-zc-XXXXXX")"
  mkdir -p "$TMP_DIR"
  if [ -d "$HOME/.zeroclaw" ]; then
    cp -R "$HOME/.zeroclaw/." "$TMP_DIR/" 2>/dev/null || true
  fi
  if ! printf '%s' "$3" | base64 -d >"$TMP_DIR/config.toml" 2>/dev/null; then
    printf '%s' "$3" | base64 -D >"$TMP_DIR/config.toml"
  fi
  CONFIG_DIR="$TMP_DIR"
fi
if [ -n "$2" ]; then
  exec "$ZC" agent --config-dir "$CONFIG_DIR" --json --no-color -p "$2" -m "$1"
fi
exec "$ZC" agent --config-dir "$CONFIG_DIR" --json --no-color -m "$1"`
	cmd := managedExecCommandContext(
		ctx,
		limactlPath,
		"shell",
		safeInstance,
		"--",
		"sh",
		"-lc",
		guestScript,
		"carrier-managed-proxy",
		strings.TrimSpace(message),
		strings.TrimSpace(provider),
		strings.TrimSpace(overrideConfigB64),
	)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errText := strings.TrimSpace(stderr.String())
		if errText != "" {
			return baseagent.ChatResponse{}, fmt.Errorf("zeroclaw agent command failed: %w: %s", err, errText)
		}
		return baseagent.ChatResponse{}, fmt.Errorf("zeroclaw agent command failed: %w", err)
	}
	if resp := parseManagedAgentCLIResponse(stdout.String()); strings.TrimSpace(resp.Message) != "" || resp.RichContent != nil {
		return resp, nil
	}
	if errText := strings.TrimSpace(stderr.String()); errText != "" {
		return baseagent.ChatResponse{}, fmt.Errorf("zeroclaw agent command returned empty output: %s", errText)
	}
	return baseagent.ChatResponse{}, fmt.Errorf("zeroclaw agent command returned empty output")
}

func normalizeManagedAgentCLIOutput(raw string) string {
	lines := strings.Split(raw, "\n")
	all := make([]string, 0, len(lines))
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		clean := strings.TrimSpace(managedANSISequence.ReplaceAllString(line, ""))
		if clean == "" {
			continue
		}
		all = append(all, clean)
		if managedStructuredLogLine.MatchString(clean) {
			continue
		}
		filtered = append(filtered, clean)
	}
	if len(filtered) > 0 {
		return strings.TrimSpace(strings.Join(filtered, "\n"))
	}
	return strings.TrimSpace(strings.Join(all, "\n"))
}

func parseManagedAgentCLIResponse(raw string) baseagent.ChatResponse {
	payload := strings.TrimSpace(extractManagedJSONObject(raw))
	if payload != "" {
		var resp baseagent.ChatResponse
		if err := json.Unmarshal([]byte(payload), &resp); err == nil {
			if strings.TrimSpace(resp.Message) != "" || resp.RichContent != nil {
				return resp
			}
		}
	}
	if out := normalizeManagedAgentCLIOutput(raw); out != "" {
		return baseagent.ChatResponse{Message: out}
	}
	return baseagent.ChatResponse{}
}

func extractManagedJSONObject(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if json.Valid([]byte(trimmed)) {
		return trimmed
	}
	start := strings.Index(trimmed, "{")
	if start < 0 {
		return ""
	}
	candidate := strings.TrimSpace(trimmed[start:])
	if json.Valid([]byte(candidate)) {
		return candidate
	}
	end := strings.LastIndex(candidate, "}")
	if end >= 0 {
		maybe := strings.TrimSpace(candidate[:end+1])
		if json.Valid([]byte(maybe)) {
			return maybe
		}
	}
	return ""
}

func proxyZeroClawWebhook(ctx context.Context, cfg zeroclawGatewayConfig, message string) (baseagent.ChatResponse, error) {
	host := strings.TrimSpace(cfg.Host)
	if host == "" {
		host = "127.0.0.1"
	}
	if cfg.Port <= 0 {
		return baseagent.ChatResponse{}, fmt.Errorf("zeroclaw gateway port is not configured")
	}
	endpoint := "http://" + net.JoinHostPort(host, strconv.Itoa(cfg.Port)) + "/webhook"
	rawBody, err := json.Marshal(map[string]string{"message": strings.TrimSpace(message)})
	if err != nil {
		return baseagent.ChatResponse{}, fmt.Errorf("marshal zeroclaw webhook payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(rawBody))
	if err != nil {
		return baseagent.ChatResponse{}, fmt.Errorf("build zeroclaw webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := managedAgentHTTPClient.Do(req)
	if err != nil {
		return baseagent.ChatResponse{}, fmt.Errorf("zeroclaw webhook request failed: %w", err)
	}
	defer resp.Body.Close()
	respRaw, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return baseagent.ChatResponse{}, fmt.Errorf("read zeroclaw webhook response: %w", readErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return baseagent.ChatResponse{}, fmt.Errorf("zeroclaw webhook status=%d: %s", resp.StatusCode, strings.TrimSpace(string(respRaw)))
	}
	if structured := extractManagedAgentWebhookResponse(respRaw); strings.TrimSpace(structured.Message) != "" || structured.RichContent != nil {
		return structured, nil
	}
	if trimmed := strings.TrimSpace(string(respRaw)); trimmed != "" {
		return baseagent.ChatResponse{Message: trimmed}, nil
	}
	return baseagent.ChatResponse{}, fmt.Errorf("zeroclaw webhook returned empty response")
}

func extractManagedAgentWebhookResponse(raw []byte) baseagent.ChatResponse {
	var resp baseagent.ChatResponse
	if err := json.Unmarshal(raw, &resp); err == nil {
		if strings.TrimSpace(resp.Message) != "" || resp.RichContent != nil {
			return resp
		}
	}
	if msg := extractManagedAgentMessage(raw); msg != "" {
		return baseagent.ChatResponse{Message: msg}
	}
	return baseagent.ChatResponse{}
}

func extractManagedAgentMessage(raw []byte) string {
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	for _, key := range []string{"message", "response", "output"} {
		if value := strings.TrimSpace(managedAnyToString(payload[key])); value != "" {
			return value
		}
	}
	if nested, ok := payload["result"].(map[string]interface{}); ok {
		for _, key := range []string{"message", "response", "output"} {
			if value := strings.TrimSpace(managedAnyToString(nested[key])); value != "" {
				return value
			}
		}
	}
	return ""
}

func managedAnyToString(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return ""
	}
}
