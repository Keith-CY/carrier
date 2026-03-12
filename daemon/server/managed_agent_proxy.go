package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"carrier/daemon/internal/lifecycle"
)

var managedAgentHTTPClient = &http.Client{Timeout: 60 * time.Second}
var (
	managedExecCommandContext = exec.CommandContext
	managedLookPath           = exec.LookPath
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

func maybeProxyManagedAgentChat(ctx context.Context, svc *lifecycle.Service, agentID string, provider string, modelAlias string, model string, message string) (string, bool, error) {
	switch strings.ToLower(strings.TrimSpace(agentID)) {
	case "zeroclaw":
		cfg, err := loadLocalZeroClawConfig()
		if err != nil {
			if os.IsNotExist(err) {
				return "", false, nil
			}
			return "", true, err
		}
		if cfg.Gateway.RequirePairing {
			return "", true, fmt.Errorf("zeroclaw local gateway still requires pairing")
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
			return "", true, fmt.Errorf("%v; webhook fallback failed: %w", cliErr, err)
		}
		return reply, true, err
	default:
		return "", false, nil
	}
}

func maybeProxyManagedZeroClawAgentCLI(ctx context.Context, state lifecycle.AgentState, cfg zeroclawLocalConfig, provider string, modelAlias string, model string, message string) (string, bool, error) {
	if !state.Isolated {
		return "", false, nil
	}
	instanceName := strings.TrimSpace(state.LimaInstanceName)
	if instanceName == "" {
		return "", false, nil
	}
	overrideConfig, err := buildManagedZeroClawModelOverride(cfg, provider, modelAlias, model)
	if err != nil {
		return "", true, err
	}
	reply, err := runManagedZeroClawAgentCLI(ctx, instanceName, provider, message, overrideConfig)
	if err != nil {
		return "", true, err
	}
	return reply, true, nil
}

func runManagedZeroClawAgentCLI(ctx context.Context, instanceName string, provider string, message string, overrideConfigB64 string) (string, error) {
	limactlPath, err := resolveManagedLimaCtlPath()
	if err != nil {
		return "", err
	}
	safeInstance := strings.TrimSpace(instanceName)
	if safeInstance == "" {
		return "", fmt.Errorf("zeroclaw managed instance name is empty")
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
  exec "$ZC" agent --config-dir "$CONFIG_DIR" -p "$2" -m "$1"
fi
exec "$ZC" agent --config-dir "$CONFIG_DIR" -m "$1"`
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
			return "", fmt.Errorf("zeroclaw agent command failed: %w: %s", err, errText)
		}
		return "", fmt.Errorf("zeroclaw agent command failed: %w", err)
	}
	if out := normalizeManagedAgentCLIOutput(stdout.String()); out != "" {
		return out, nil
	}
	if errText := strings.TrimSpace(stderr.String()); errText != "" {
		return "", fmt.Errorf("zeroclaw agent command returned empty output: %s", errText)
	}
	return "", fmt.Errorf("zeroclaw agent command returned empty output")
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

func resolveManagedLimaCtlPath() (string, error) {
	if managedLookPath != nil {
		if path, err := managedLookPath("limactl"); err == nil && strings.TrimSpace(path) != "" {
			return strings.TrimSpace(path), nil
		}
	}
	for _, candidate := range []string{"/opt/homebrew/bin/limactl", "/usr/local/bin/limactl"} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("limactl executable not found for managed zeroclaw proxy")
}

func loadLocalZeroClawGatewayConfig() (zeroclawGatewayConfig, error) {
	cfg, err := loadLocalZeroClawConfig()
	if err != nil {
		return zeroclawGatewayConfig{}, err
	}
	return cfg.Gateway, nil
}

func loadLocalZeroClawConfig() (zeroclawLocalConfig, error) {
	home, err := userHomeDirFunc()
	if err != nil {
		return zeroclawLocalConfig{}, err
	}
	raw, err := os.ReadFile(filepath.Join(home, ".zeroclaw", "config.toml"))
	if err != nil {
		return zeroclawLocalConfig{}, err
	}
	return parseZeroClawLocalConfig(raw), nil
}

func parseZeroClawLocalConfig(raw []byte) zeroclawLocalConfig {
	cfg := zeroclawLocalConfig{
		Raw: raw,
		Gateway: zeroclawGatewayConfig{
			Host:           "127.0.0.1",
			Port:           9091,
			RequirePairing: true,
		},
	}
	section := ""
	currentProfile := zeroclawProviderProfile{}
	flushProfile := func() {
		if strings.TrimSpace(currentProfile.SectionName) == "" {
			return
		}
		cfg.Profiles = append(cfg.Profiles, currentProfile)
		currentProfile = zeroclawProviderProfile{}
	}
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if idx := strings.Index(trimmed, "#"); idx >= 0 {
			trimmed = strings.TrimSpace(trimmed[:idx])
		}
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			flushProfile()
			section = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "["), "]"))
			if strings.HasPrefix(section, "provider_profiles.") {
				currentProfile.SectionName = strings.TrimSpace(strings.TrimPrefix(section, "provider_profiles."))
			}
			continue
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch {
		case section == "":
			switch key {
			case "default_provider":
				if unquoted, err := strconv.Unquote(value); err == nil {
					cfg.DefaultProvider = strings.TrimSpace(unquoted)
				}
			case "default_model":
				if unquoted, err := strconv.Unquote(value); err == nil {
					cfg.DefaultModel = strings.TrimSpace(unquoted)
				}
			}
		case section == "gateway":
			switch key {
			case "host":
				if unquoted, err := strconv.Unquote(value); err == nil && strings.TrimSpace(unquoted) != "" {
					cfg.Gateway.Host = strings.TrimSpace(unquoted)
				}
			case "port":
				if port, err := strconv.Atoi(value); err == nil && port > 0 {
					cfg.Gateway.Port = port
				}
			case "require_pairing":
				cfg.Gateway.RequirePairing = strings.EqualFold(value, "true")
			}
		case strings.HasPrefix(section, "provider_profiles."):
			switch key {
			case "model_alias":
				if unquoted, err := strconv.Unquote(value); err == nil {
					currentProfile.ModelAlias = strings.TrimSpace(unquoted)
				}
			case "model":
				if unquoted, err := strconv.Unquote(value); err == nil {
					currentProfile.Model = strings.TrimSpace(unquoted)
				}
			case "provider":
				if unquoted, err := strconv.Unquote(value); err == nil {
					currentProfile.Provider = strings.TrimSpace(unquoted)
				}
			case "provider_id":
				if unquoted, err := strconv.Unquote(value); err == nil {
					currentProfile.ProviderID = strings.TrimSpace(unquoted)
				}
			}
		}
	}
	flushProfile()
	return cfg
}

func parseZeroClawGatewayConfig(raw []byte) zeroclawGatewayConfig {
	return parseZeroClawLocalConfig(raw).Gateway
}

func buildManagedZeroClawModelOverride(cfg zeroclawLocalConfig, provider, modelAlias, model string) (string, error) {
	selectedModel, err := resolveManagedZeroClawSelectedModel(cfg, provider, modelAlias, model)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(selectedModel) == "" {
		return "", nil
	}
	rewritten := rewriteZeroClawDefaultModel(cfg.Raw, selectedModel)
	return base64.StdEncoding.EncodeToString(rewritten), nil
}

func resolveManagedZeroClawSelectedModel(cfg zeroclawLocalConfig, provider, modelAlias, model string) (string, error) {
	if trimmedModel := strings.TrimSpace(model); trimmedModel != "" {
		return trimmedModel, nil
	}
	alias := strings.TrimSpace(modelAlias)
	if alias == "" {
		return "", nil
	}
	requestedProvider := strings.ToLower(strings.TrimSpace(provider))
	defaultProvider := strings.ToLower(strings.TrimSpace(cfg.DefaultProvider))
	for _, profile := range cfg.Profiles {
		if !strings.EqualFold(strings.TrimSpace(profile.ModelAlias), alias) {
			continue
		}
		candidateProviders := []string{
			strings.ToLower(strings.TrimSpace(profile.Provider)),
			strings.ToLower(strings.TrimSpace(profile.ProviderID)),
		}
		if requestedProvider != "" {
			if requestedProvider != candidateProviders[0] && requestedProvider != candidateProviders[1] {
				continue
			}
		} else if defaultProvider != "" {
			if defaultProvider != candidateProviders[0] && defaultProvider != candidateProviders[1] {
				continue
			}
		}
		if strings.TrimSpace(profile.Model) != "" {
			return strings.TrimSpace(profile.Model), nil
		}
	}
	for _, profile := range cfg.Profiles {
		if strings.EqualFold(strings.TrimSpace(profile.ModelAlias), alias) && strings.TrimSpace(profile.Model) != "" {
			return strings.TrimSpace(profile.Model), nil
		}
	}
	return "", fmt.Errorf("zeroclaw config does not define model alias %q", alias)
}

func rewriteZeroClawDefaultModel(raw []byte, model string) []byte {
	lines := strings.Split(string(raw), "\n")
	replaced := false
	insertAt := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			if insertAt < 0 {
				insertAt = i
			}
			continue
		}
		key, _, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(key), "default_model") {
			lines[i] = fmt.Sprintf("default_model = %s", strconv.Quote(strings.TrimSpace(model)))
			replaced = true
			break
		}
	}
	if !replaced {
		if insertAt < 0 {
			insertAt = len(lines)
		}
		newLine := fmt.Sprintf("default_model = %s", strconv.Quote(strings.TrimSpace(model)))
		lines = append(lines[:insertAt], append([]string{newLine}, lines[insertAt:]...)...)
	}
	text := strings.Join(lines, "\n")
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return []byte(text)
}

func proxyZeroClawWebhook(ctx context.Context, cfg zeroclawGatewayConfig, message string) (string, error) {
	host := strings.TrimSpace(cfg.Host)
	if host == "" {
		host = "127.0.0.1"
	}
	if cfg.Port <= 0 {
		return "", fmt.Errorf("zeroclaw gateway port is not configured")
	}
	endpoint := "http://" + net.JoinHostPort(host, strconv.Itoa(cfg.Port)) + "/webhook"
	rawBody, err := json.Marshal(map[string]string{"message": strings.TrimSpace(message)})
	if err != nil {
		return "", fmt.Errorf("marshal zeroclaw webhook payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(rawBody))
	if err != nil {
		return "", fmt.Errorf("build zeroclaw webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := managedAgentHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("zeroclaw webhook request failed: %w", err)
	}
	defer resp.Body.Close()
	respRaw, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return "", fmt.Errorf("read zeroclaw webhook response: %w", readErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("zeroclaw webhook status=%d: %s", resp.StatusCode, strings.TrimSpace(string(respRaw)))
	}
	if msg := extractManagedAgentMessage(respRaw); msg != "" {
		return msg, nil
	}
	if trimmed := strings.TrimSpace(string(respRaw)); trimmed != "" {
		return trimmed, nil
	}
	return "", fmt.Errorf("zeroclaw webhook returned empty response")
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
