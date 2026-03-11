package baseagent

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type MCPConfig struct {
	Servers []MCPServerConfig `json:"servers"`
}

type MCPServerConfig struct {
	Name  string          `json:"name"`
	Tools []MCPToolConfig `json:"tools"`
}

type MCPToolConfig struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
	Hidden      bool           `json:"hidden,omitempty"`
	Aliases     []string       `json:"aliases,omitempty"`
}

func LoadMCPConfigFile(path string) (MCPConfig, error) {
	raw, err := os.ReadFile(strings.TrimSpace(path))
	if err != nil {
		return MCPConfig{}, fmt.Errorf("read mcp config: %w", err)
	}
	var cfg MCPConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return MCPConfig{}, fmt.Errorf("parse mcp config: %w", err)
	}
	if err := ValidateMCPConfig(cfg); err != nil {
		return MCPConfig{}, err
	}
	return normalizeMCPConfig(cfg), nil
}

func ValidateMCPConfig(cfg MCPConfig) error {
	serverSeen := map[string]struct{}{}
	toolSeen := map[string]struct{}{}
	aliasSeen := map[string]struct{}{}
	for _, server := range cfg.Servers {
		serverName := strings.TrimSpace(strings.ToLower(server.Name))
		if serverName == "" {
			return fmt.Errorf("mcp server name is required")
		}
		if _, exists := serverSeen[serverName]; exists {
			return fmt.Errorf("duplicate mcp server %q", serverName)
		}
		serverSeen[serverName] = struct{}{}
		for _, tool := range server.Tools {
			toolName := strings.TrimSpace(strings.ToLower(tool.Name))
			if toolName == "" {
				return fmt.Errorf("mcp tool name is required (%s)", serverName)
			}
			if _, exists := toolSeen[toolName]; exists {
				return fmt.Errorf("duplicate mcp tool %q", toolName)
			}
			toolSeen[toolName] = struct{}{}
			for _, alias := range tool.Aliases {
				alias = strings.TrimSpace(strings.ToLower(alias))
				if alias == "" {
					return fmt.Errorf("empty alias for mcp tool %q", toolName)
				}
				if alias == toolName {
					return fmt.Errorf("alias %q duplicates canonical tool name", alias)
				}
				if _, exists := toolSeen[alias]; exists {
					return fmt.Errorf("alias %q conflicts with tool name", alias)
				}
				if _, exists := aliasSeen[alias]; exists {
					return fmt.Errorf("duplicate alias %q", alias)
				}
				aliasSeen[alias] = struct{}{}
			}
		}
	}
	return nil
}

func normalizeMCPConfig(cfg MCPConfig) MCPConfig {
	out := MCPConfig{Servers: make([]MCPServerConfig, 0, len(cfg.Servers))}
	for _, server := range cfg.Servers {
		normalizedServer := MCPServerConfig{
			Name:  strings.TrimSpace(strings.ToLower(server.Name)),
			Tools: make([]MCPToolConfig, 0, len(server.Tools)),
		}
		for _, tool := range server.Tools {
			normalizedServer.Tools = append(normalizedServer.Tools, MCPToolConfig{
				Name:        strings.TrimSpace(strings.ToLower(tool.Name)),
				Description: strings.TrimSpace(tool.Description),
				Parameters:  cloneToolSchema(tool.Parameters),
				Hidden:      tool.Hidden,
				Aliases:     normalizeSkillValues(tool.Aliases),
			})
		}
		out.Servers = append(out.Servers, normalizedServer)
	}
	return out
}
