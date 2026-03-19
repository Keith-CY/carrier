package baseagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func executeStructuredBuiltinTool(ctx context.Context, builtin *ToolRegistry, toolName, displayName string, args map[string]any) ExecutionToolResult {
	if builtin == nil {
		return executionError("tool registry is unavailable")
	}
	resp, err := builtin.ExecuteTool(ctx, toolName, stringifyStructuredToolArgs(args))
	if err != nil {
		return executionError(err.Error())
	}
	output := strings.TrimSpace(resp.Message)
	if output == "" {
		output = strings.TrimSpace(resp.Action)
	}
	if output == "" {
		output = fmt.Sprintf("tool %s completed", displayName)
	}
	return ExecutionToolResult{Output: output}
}

func structuredWorkspaceToolTier(name string) structuredToolTier {
	switch strings.TrimSpace(name) {
	case "web_fetch", "web_search":
		return structuredToolTierOperationalRead
	case "read_file", "list_dir":
		return structuredToolTierWorkspaceRead
	case "write_file", "append_file", "edit_file":
		return structuredToolTierWorkspaceMutation
	case "exec", "send_file", "spawn_subagent":
		return structuredToolTierHighRisk
	default:
		return structuredToolTierWorkspaceRead
	}
}

func structuredMCPToolTier(name string) structuredToolTier {
	lower := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.HasPrefix(lower, "get_"),
		strings.HasPrefix(lower, "list_"),
		strings.HasPrefix(lower, "read_"),
		strings.Contains(lower, "search"):
		return structuredToolTierMetadataRead
	default:
		return structuredToolTierHighRisk
	}
}

func renderMemorySearchHits(hits []MemorySearchHit) string {
	if len(hits) == 0 {
		return "no memory results"
	}
	lines := make([]string, 0, len(hits))
	for idx, hit := range hits {
		line := fmt.Sprintf("%d. %s", idx+1, strings.TrimSpace(hit.ID))
		if scope := strings.TrimSpace(hit.Scope); scope != "" {
			line += " [" + scope + "]"
		}
		lines = append(lines, line)
		if snippet := strings.TrimSpace(hit.Snippet); snippet != "" {
			lines = append(lines, snippet)
		}
		if provenance := strings.TrimSpace(hit.Provenance); provenance != "" {
			lines = append(lines, "provenance: "+provenance)
		}
	}
	return truncateExecutionOutput(strings.Join(lines, "\n"))
}

func readOptionalFloatArg(args map[string]any, key string, fallback float64) (float64, error) {
	if args == nil {
		return fallback, nil
	}
	value, ok := args[key]
	if !ok || value == nil {
		return fallback, nil
	}
	switch typed := value.(type) {
	case float64:
		return typed, nil
	case float32:
		return float64(typed), nil
	case int:
		return float64(typed), nil
	case int32:
		return float64(typed), nil
	case int64:
		return float64(typed), nil
	default:
		return 0, fmt.Errorf("%s must be a number", key)
	}
}

func stringifyStructuredToolArgs(args map[string]any) map[string]string {
	if len(args) == 0 {
		return nil
	}
	out := make(map[string]string, len(args))
	for key, value := range args {
		out[key] = stringifyStructuredToolArg(value)
	}
	return out
}

func stringifyStructuredToolArg(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		if raw, err := json.Marshal(typed); err == nil {
			return string(raw)
		}
		return fmt.Sprint(typed)
	}
}
