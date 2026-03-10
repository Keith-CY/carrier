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
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var managedAgentHTTPClient = &http.Client{Timeout: 60 * time.Second}

type zeroclawGatewayConfig struct {
	Host           string
	Port           int
	RequirePairing bool
}

func maybeProxyManagedAgentChat(ctx context.Context, agentID string, message string) (string, bool, error) {
	switch strings.ToLower(strings.TrimSpace(agentID)) {
	case "zeroclaw":
		cfg, err := loadLocalZeroClawGatewayConfig()
		if err != nil {
			if os.IsNotExist(err) {
				return "", false, nil
			}
			return "", true, err
		}
		if cfg.RequirePairing {
			return "", true, fmt.Errorf("zeroclaw local gateway still requires pairing")
		}
		reply, err := proxyZeroClawWebhook(ctx, cfg, message)
		return reply, true, err
	default:
		return "", false, nil
	}
}

func loadLocalZeroClawGatewayConfig() (zeroclawGatewayConfig, error) {
	home, err := userHomeDirFunc()
	if err != nil {
		return zeroclawGatewayConfig{}, err
	}
	raw, err := os.ReadFile(filepath.Join(home, ".zeroclaw", "config.toml"))
	if err != nil {
		return zeroclawGatewayConfig{}, err
	}
	return parseZeroClawGatewayConfig(raw), nil
}

func parseZeroClawGatewayConfig(raw []byte) zeroclawGatewayConfig {
	cfg := zeroclawGatewayConfig{
		Host:           "127.0.0.1",
		Port:           9091,
		RequirePairing: true,
	}
	section := ""
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
			section = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "["), "]"))
			continue
		}
		if section != "gateway" {
			continue
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "host":
			if unquoted, err := strconv.Unquote(value); err == nil && strings.TrimSpace(unquoted) != "" {
				cfg.Host = strings.TrimSpace(unquoted)
			}
		case "port":
			if port, err := strconv.Atoi(value); err == nil && port > 0 {
				cfg.Port = port
			}
		case "require_pairing":
			cfg.RequirePairing = strings.EqualFold(value, "true")
		}
	}
	return cfg
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
