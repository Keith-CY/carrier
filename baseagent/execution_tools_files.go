package baseagent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func (r *ExecutionToolRegistry) writeFile(_ context.Context, args map[string]any) ExecutionToolResult {
	path, ok := args["path"].(string)
	if !ok || strings.TrimSpace(path) == "" {
		return executionError("path is required")
	}
	content, ok := args["content"].(string)
	if !ok {
		return executionError("content is required")
	}

	resolved, err := r.resolveWorkspacePath(path)
	if err != nil {
		return executionError(err.Error())
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return executionError(fmt.Sprintf("create parent directory: %v", err))
	}
	if err := os.WriteFile(resolved, []byte(content), 0o600); err != nil {
		return executionError(fmt.Sprintf("write file: %v", err))
	}
	return ExecutionToolResult{Output: fmt.Sprintf("wrote %s", resolved), FilesTouched: []string{resolved}}
}

func (r *ExecutionToolRegistry) appendFile(_ context.Context, args map[string]any) ExecutionToolResult {
	path, ok := args["path"].(string)
	if !ok || strings.TrimSpace(path) == "" {
		return executionError("path is required")
	}
	content, ok := args["content"].(string)
	if !ok {
		return executionError("content is required")
	}

	resolved, err := r.resolveWorkspacePath(path)
	if err != nil {
		return executionError(err.Error())
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return executionError(fmt.Sprintf("create parent directory: %v", err))
	}
	file, err := os.OpenFile(resolved, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return executionError(fmt.Sprintf("open append target: %v", err))
	}
	defer file.Close()
	if _, err := file.WriteString(content); err != nil {
		return executionError(fmt.Sprintf("append file: %v", err))
	}
	return ExecutionToolResult{Output: fmt.Sprintf("appended %s", resolved), FilesTouched: []string{resolved}}
}

func (r *ExecutionToolRegistry) readFile(_ context.Context, args map[string]any) ExecutionToolResult {
	path, ok := args["path"].(string)
	if !ok || strings.TrimSpace(path) == "" {
		return executionError("path is required")
	}
	resolved, err := r.resolveWorkspacePath(path)
	if err != nil {
		return executionError(err.Error())
	}
	raw, err := os.ReadFile(resolved)
	if err != nil {
		return executionError(fmt.Sprintf("read file: %v", err))
	}
	offset, err := readOptionalIntArg(args, "offset", 0)
	if err != nil {
		return executionError(err.Error())
	}
	limit, err := readOptionalIntArg(args, "limit", 0)
	if err != nil {
		return executionError(err.Error())
	}
	return ExecutionToolResult{Output: paginateTextByLines(string(raw), offset, limit)}
}

func (r *ExecutionToolRegistry) editFile(_ context.Context, args map[string]any) ExecutionToolResult {
	path, ok := args["path"].(string)
	if !ok || strings.TrimSpace(path) == "" {
		return executionError("path is required")
	}
	oldText, ok := args["old_text"].(string)
	if !ok || oldText == "" {
		return executionError("old_text is required")
	}
	newText, ok := args["new_text"].(string)
	if !ok {
		return executionError("new_text is required")
	}

	resolved, err := r.resolveWorkspacePath(path)
	if err != nil {
		return executionError(err.Error())
	}
	raw, err := os.ReadFile(resolved)
	if err != nil {
		return executionError(fmt.Sprintf("read file: %v", err))
	}
	content := string(raw)
	if !strings.Contains(content, oldText) {
		return executionError("old_text not found in file")
	}
	if strings.Count(content, oldText) > 1 {
		return executionError("old_text appears multiple times")
	}
	updated := strings.Replace(content, oldText, newText, 1)
	if err := os.WriteFile(resolved, []byte(updated), 0o600); err != nil {
		return executionError(fmt.Sprintf("write edited file: %v", err))
	}
	return ExecutionToolResult{Output: fmt.Sprintf("edited %s", resolved), FilesTouched: []string{resolved}}
}

func (r *ExecutionToolRegistry) listDir(_ context.Context, args map[string]any) ExecutionToolResult {
	path, _ := args["path"].(string)
	if strings.TrimSpace(path) == "" {
		path = "."
	}
	resolved, err := r.resolveWorkspacePath(path)
	if err != nil {
		return executionError(err.Error())
	}
	entries, err := os.ReadDir(resolved)
	if err != nil {
		return executionError(fmt.Sprintf("list directory: %v", err))
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		names = append(names, name)
	}
	sort.Strings(names)
	offset, err := readOptionalIntArg(args, "offset", 0)
	if err != nil {
		return executionError(err.Error())
	}
	limit, err := readOptionalIntArg(args, "limit", 0)
	if err != nil {
		return executionError(err.Error())
	}
	return ExecutionToolResult{Output: strings.Join(paginateStrings(names, offset, limit), "\n")}
}

func (r *ExecutionToolRegistry) resolveWorkspacePath(rawPath string) (string, error) {
	trimmed := strings.TrimSpace(rawPath)
	if trimmed == "" {
		return "", fmt.Errorf("path is required")
	}
	if r.workspaceRoot == "" {
		return "", fmt.Errorf("workspace root is not configured")
	}

	root := r.workspaceRoot
	resolved := trimmed
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(root, resolved)
	}
	resolved = filepath.Clean(resolved)

	rel, err := filepath.Rel(root, resolved)
	if err != nil {
		return "", fmt.Errorf("path resolution failed: %w", err)
	}
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path escapes workspace root")
	}
	if err := ensurePathInsideWorkspace(root, resolved); err != nil {
		return "", err
	}
	return resolved, nil
}

func paginateTextByLines(content string, offset, limit int) string {
	lines := strings.Split(content, "\n")
	if offset >= len(lines) {
		return "[END OF FILE - no content at this offset]"
	}
	return strings.Join(paginateStrings(lines, offset, limit), "\n")
}

func paginateStrings(items []string, offset, limit int) []string {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(items) {
		return []string{}
	}
	items = items[offset:]
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items
}

func ensurePathInsideWorkspace(root, resolved string) error {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		realRoot = root
	}

	probe := resolved
	for {
		info, err := os.Lstat(probe)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 || info.IsDir() || !info.IsDir() {
				break
			}
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			break
		}
		probe = parent
	}

	realProbe, err := filepath.EvalSymlinks(probe)
	if err != nil {
		return fmt.Errorf("path escapes workspace root")
	}
	rel, err := filepath.Rel(realRoot, realProbe)
	if err != nil {
		return fmt.Errorf("path resolution failed: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escapes workspace root")
	}
	return nil
}
