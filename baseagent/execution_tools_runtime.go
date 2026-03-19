package baseagent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func (r *ExecutionToolRegistry) execCommand(ctx context.Context, args map[string]any) ExecutionToolResult {
	command, ok := args["command"].(string)
	if !ok || strings.TrimSpace(command) == "" {
		return executionError("command is required")
	}
	if guardErr := r.guardCommand(command); guardErr != "" {
		return executionError(guardErr)
	}

	cmdCtx := ctx
	cancel := func() {}
	timeoutSeconds := defaultExecTimeoutSeconds
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var err error
		timeoutSeconds, err = readOptionalIntArg(args, "timeout_seconds", defaultExecTimeoutSeconds)
		if err != nil {
			return executionError(err.Error())
		}
		if timeoutSeconds <= 0 {
			timeoutSeconds = defaultExecTimeoutSeconds
		}
		cmdCtx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	}
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(cmdCtx, "powershell", "-NoProfile", "-NonInteractive", "-Command", command)
	} else {
		cmd = exec.CommandContext(cmdCtx, "sh", "-c", command)
	}
	if r.workspaceRoot != "" {
		cmd.Dir = r.workspaceRoot
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := ExecutionToolResult{
		Output: strings.TrimSpace(stdout.String()),
		Stdout: strings.TrimSpace(stdout.String()),
		Stderr: strings.TrimSpace(stderr.String()),
	}
	if result.Output == "" && result.Stderr != "" {
		result.Output = result.Stderr
	}
	result.Output = truncateExecutionOutput(result.Output)
	result.Stdout = truncateExecutionOutput(result.Stdout)
	result.Stderr = truncateExecutionOutput(result.Stderr)

	if err == nil {
		return result
	}
	if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) {
		result.IsError = true
		result.Output = fmt.Sprintf("command timed out after %ds", timeoutSeconds)
		return result
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		result.IsError = true
		if result.Output == "" {
			result.Output = strings.TrimSpace(exitErr.Error())
		}
		return result
	}

	result.IsError = true
	if result.Output == "" {
		result.Output = err.Error()
	}
	return result
}

func (r *ExecutionToolRegistry) webFetch(ctx context.Context, args map[string]any) ExecutionToolResult {
	url, ok := args["url"].(string)
	if !ok || strings.TrimSpace(url) == "" {
		return executionError("url is required")
	}
	if r.webBackend == nil {
		return executionError("web fetch backend is not configured")
	}
	text, err := r.webBackend.Fetch(ctx, strings.TrimSpace(url))
	if err != nil {
		return executionError(fmt.Sprintf("web fetch: %v", err))
	}
	text = truncateExecutionOutput(strings.TrimSpace(text))
	if text == "" {
		text = "web fetch returned no content"
	}
	return ExecutionToolResult{
		Output: text,
		Metadata: map[string]any{
			"source_url": strings.TrimSpace(url),
		},
	}
}

func (r *ExecutionToolRegistry) webSearch(ctx context.Context, args map[string]any) ExecutionToolResult {
	query, ok := args["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return executionError("query is required")
	}
	limit, err := readOptionalIntArg(args, "limit", defaultWebSearchLimit)
	if err != nil {
		return executionError(err.Error())
	}
	if limit <= 0 {
		limit = defaultWebSearchLimit
	}
	if limit > maxWebSearchLimit {
		limit = maxWebSearchLimit
	}
	if r.webBackend == nil {
		return executionError("web search backend is not configured")
	}
	hits, err := r.webBackend.Search(ctx, strings.TrimSpace(query), limit)
	if err != nil {
		return executionError(fmt.Sprintf("web search: %v", err))
	}
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return ExecutionToolResult{
		Output: renderWebSearchHits(hits),
		Metadata: map[string]any{
			"query":          strings.TrimSpace(query),
			"search_results": hits,
		},
	}
}

func (r *ExecutionToolRegistry) sendFile(_ context.Context, args map[string]any) ExecutionToolResult {
	path, ok := args["path"].(string)
	if !ok || strings.TrimSpace(path) == "" {
		return executionError("path is required")
	}
	resolved, err := r.resolveWorkspacePath(path)
	if err != nil {
		return executionError(err.Error())
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return executionError(fmt.Sprintf("stat file: %v", err))
	}
	if info.IsDir() {
		return executionError("path must point to a file")
	}
	attachment := ExecutionAttachment{
		ID:         resolved,
		Kind:       "file",
		OutputRole: "generated",
		Path:       resolved,
		Name:       info.Name(),
		MIMEType:   mime.TypeByExtension(filepath.Ext(info.Name())),
		MediaType:  mime.TypeByExtension(filepath.Ext(info.Name())),
		SizeBytes:  info.Size(),
		Source:     "workspace",
		ArtifactID: resolved,
	}
	return ExecutionToolResult{
		Output:      fmt.Sprintf("prepared attachment %s (%d bytes)", attachment.Name, attachment.SizeBytes),
		Attachments: []AttachmentRef{attachment},
		ContentBlocks: []ContentBlock{{
			Type:         "file",
			OutputRole:   "generated",
			Name:         attachment.Name,
			Path:         attachment.Path,
			MIMEType:     attachment.MIMEType,
			MediaType:    attachment.MediaType,
			AttachmentID: attachment.ID,
			SizeBytes:    attachment.SizeBytes,
		}},
		Metadata: map[string]any{"attachment": attachment},
	}
}

func (r *ExecutionToolRegistry) spawnSubagent(ctx context.Context, args map[string]any) ExecutionToolResult {
	task, ok := args["task"].(string)
	if !ok || strings.TrimSpace(task) == "" {
		return executionError("task is required")
	}
	if r.subagents == nil {
		return executionError("subagent spawner is not configured")
	}
	handle, err := r.subagents.Spawn(ctx, SubagentRequest{Task: strings.TrimSpace(task)})
	if err != nil {
		return executionError(fmt.Sprintf("spawn subagent: %v", err))
	}
	status := strings.TrimSpace(handle.Status)
	if status == "" {
		status = "queued"
	}
	output := fmt.Sprintf("delegated job %s (%s)", strings.TrimSpace(handle.JobID), status)
	if summary := strings.TrimSpace(handle.Summary); summary != "" {
		output += ": " + summary
	}
	return ExecutionToolResult{
		Output: output,
		Metadata: map[string]any{
			"delegation":  handle,
			"job_id":      strings.TrimSpace(handle.JobID),
			"job_status":  status,
			"job_summary": strings.TrimSpace(handle.Summary),
		},
	}
}

func (r *ExecutionToolRegistry) guardCommand(command string) string {
	lower := strings.ToLower(strings.TrimSpace(command))
	for _, pattern := range r.denyPatterns {
		if pattern.MatchString(lower) {
			return "command denied by execution safety policy"
		}
	}
	return ""
}

func executionError(message string) ExecutionToolResult {
	return ExecutionToolResult{Output: strings.TrimSpace(message), IsError: true, Status: ExecutionToolResultStatusError}
}

func executionAsk(message string) ExecutionToolResult {
	return ExecutionToolResult{Output: strings.TrimSpace(message), IsError: true, Status: ExecutionToolResultStatusAsk}
}

func executionDeny(message string) ExecutionToolResult {
	return ExecutionToolResult{Output: strings.TrimSpace(message), IsError: true, Status: ExecutionToolResultStatusDeny}
}

func applyStructuredPolicyMetadata(result ExecutionToolResult, decision StructuredPolicyDecision) ExecutionToolResult {
	if strings.TrimSpace(result.PolicyReason) == "" {
		result.PolicyReason = strings.TrimSpace(decision.Reason)
	}
	if strings.TrimSpace(result.PolicyRuleID) == "" {
		result.PolicyRuleID = strings.TrimSpace(decision.RuleID)
	}
	return result
}

func executionAskWithPolicy(message string, decision StructuredPolicyDecision) ExecutionToolResult {
	return applyStructuredPolicyMetadata(executionAsk(message), decision)
}

func executionDenyWithPolicy(message string, decision StructuredPolicyDecision) ExecutionToolResult {
	return applyStructuredPolicyMetadata(executionDeny(message), decision)
}

func renderWebSearchHits(hits []WebSearchHit) string {
	if len(hits) == 0 {
		return "no search results"
	}
	lines := make([]string, 0, len(hits)*2)
	for idx, hit := range hits {
		title := strings.TrimSpace(hit.Title)
		if title == "" {
			title = fmt.Sprintf("Result %d", idx+1)
		}
		url := strings.TrimSpace(hit.URL)
		line := fmt.Sprintf("%d. %s", idx+1, title)
		if url != "" {
			line += " - " + url
		}
		lines = append(lines, line)
		if snippet := strings.TrimSpace(hit.Snippet); snippet != "" {
			lines = append(lines, snippet)
		}
	}
	return truncateExecutionOutput(strings.Join(lines, "\n"))
}

func readOptionalIntArg(args map[string]any, key string, fallback int) (int, error) {
	if args == nil {
		return fallback, nil
	}
	value, ok := args[key]
	if !ok || value == nil {
		return fallback, nil
	}
	switch typed := value.(type) {
	case int:
		return typed, nil
	case int32:
		return int(typed), nil
	case int64:
		return int(typed), nil
	case float64:
		return int(typed), nil
	default:
		return 0, fmt.Errorf("%s must be an integer", key)
	}
}

func truncateExecutionOutput(content string) string {
	content = strings.TrimSpace(content)
	if len(content) <= maxExecutionOutputBytes {
		return content
	}
	suffix := "\n[truncated output]"
	if maxExecutionOutputBytes <= len(suffix) {
		return suffix
	}
	return content[:maxExecutionOutputBytes-len(suffix)] + suffix
}
