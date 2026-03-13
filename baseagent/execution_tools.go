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
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultExecTimeoutSeconds = 30
	maxExecutionOutputBytes   = 8192
	defaultWebSearchLimit     = 5
	maxWebSearchLimit         = 10
)

type ExecutionToolResultStatus string

const (
	ExecutionToolResultStatusOK    ExecutionToolResultStatus = "ok"
	ExecutionToolResultStatusError ExecutionToolResultStatus = "error"
	ExecutionToolResultStatusAsk   ExecutionToolResultStatus = "ask"
	ExecutionToolResultStatusDeny  ExecutionToolResultStatus = "deny"
)

type ExecutionToolResult struct {
	Output        string
	Attachments   []AttachmentRef
	ContentBlocks []ContentBlock
	FilesTouched  []string
	Stdout        string
	Stderr        string
	ExitCode      int
	IsError       bool
	Status        ExecutionToolResultStatus
	PolicyReason  string
	PolicyRuleID  string
	Metadata      map[string]any
}

type WebSearchHit struct {
	Title   string
	URL     string
	Snippet string
}

type ExecutionAttachment = AttachmentRef

type SubagentRequest struct {
	Task string
}

type SubagentJobHandle struct {
	JobID   string
	Status  string
	Summary string
}

type WebToolBackend interface {
	Fetch(ctx context.Context, url string) (string, error)
	Search(ctx context.Context, query string, limit int) ([]WebSearchHit, error)
}

type SubagentSpawner interface {
	Spawn(ctx context.Context, req SubagentRequest) (SubagentJobHandle, error)
}

type ExecutionToolRegistryOption func(*ExecutionToolRegistry)

type StructuredToolDescriptor struct {
	Name        string
	Description string
	Parameters  map[string]any
}

type executionToolFunc func(ctx context.Context, args map[string]any) ExecutionToolResult

type executionToolSpec struct {
	Descriptor StructuredToolDescriptor
	Run        executionToolFunc
}

type ExecutionToolRegistry struct {
	workspaceRoot string
	tools         map[string]executionToolSpec
	denyPatterns  []*regexp.Regexp
	webBackend    WebToolBackend
	subagents     SubagentSpawner
}

type noopWebToolBackend struct{}

func (noopWebToolBackend) Fetch(_ context.Context, _ string) (string, error) {
	return "", fmt.Errorf("web fetch backend is not configured")
}

func (noopWebToolBackend) Search(_ context.Context, _ string, _ int) ([]WebSearchHit, error) {
	return nil, fmt.Errorf("web search backend is not configured")
}

type localSubagentSpawner struct {
	mu   sync.Mutex
	next int
}

func (s *localSubagentSpawner) Spawn(_ context.Context, req SubagentRequest) (SubagentJobHandle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	task := strings.TrimSpace(req.Task)
	if task == "" {
		task = "delegated task"
	}
	return SubagentJobHandle{
		JobID:   fmt.Sprintf("subagent-%d", s.next),
		Status:  "queued",
		Summary: task,
	}, nil
}

func WithExecutionToolWebBackend(backend WebToolBackend) ExecutionToolRegistryOption {
	return func(r *ExecutionToolRegistry) {
		if backend != nil {
			r.webBackend = backend
		}
	}
}

func WithExecutionToolSubagentSpawner(spawner SubagentSpawner) ExecutionToolRegistryOption {
	return func(r *ExecutionToolRegistry) {
		if spawner != nil {
			r.subagents = spawner
		}
	}
}

func NewExecutionToolRegistry(workspaceRoot string, opts ...ExecutionToolRegistryOption) *ExecutionToolRegistry {
	root := strings.TrimSpace(workspaceRoot)
	if root != "" {
		if abs, err := filepath.Abs(root); err == nil {
			root = abs
		}
	}

	registry := &ExecutionToolRegistry{
		workspaceRoot: root,
		tools:         map[string]executionToolSpec{},
		webBackend:    noopWebToolBackend{},
		subagents:     &localSubagentSpawner{},
		denyPatterns: []*regexp.Regexp{
			regexp.MustCompile(`\brm\s+-[rf]{1,2}\b`),
			regexp.MustCompile(`\bmkfs\b`),
			regexp.MustCompile(`\bdd\s+if=`),
			regexp.MustCompile(`\bshutdown\b`),
			regexp.MustCompile(`\breboot\b`),
			regexp.MustCompile(`\bsudo\b`),
		},
	}

	registry.tools["write_file"] = executionToolSpec{
		Descriptor: StructuredToolDescriptor{
			Name:        "write_file",
			Description: "Write content to a file inside the execution workspace.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string", "description": "Path to the target file."},
					"content": map[string]any{"type": "string", "description": "New file content."},
				},
				"required": []string{"path", "content"},
			},
		},
		Run: registry.writeFile,
	}
	registry.tools["append_file"] = executionToolSpec{
		Descriptor: StructuredToolDescriptor{
			Name:        "append_file",
			Description: "Append content to a file inside the execution workspace, creating it when needed.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string", "description": "Path to the target file."},
					"content": map[string]any{"type": "string", "description": "Content to append to the file."},
				},
				"required": []string{"path", "content"},
			},
		},
		Run: registry.appendFile,
	}
	registry.tools["read_file"] = executionToolSpec{
		Descriptor: StructuredToolDescriptor{
			Name:        "read_file",
			Description: "Read a file from the execution workspace.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":   map[string]any{"type": "string", "description": "Path to the file to read."},
					"offset": map[string]any{"type": "integer", "description": "Optional zero-based line offset for paginated reads."},
					"limit":  map[string]any{"type": "integer", "description": "Optional maximum number of lines to return."},
				},
				"required": []string{"path"},
			},
		},
		Run: registry.readFile,
	}
	registry.tools["edit_file"] = executionToolSpec{
		Descriptor: StructuredToolDescriptor{
			Name:        "edit_file",
			Description: "Replace one exact text span inside a file in the execution workspace.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":     map[string]any{"type": "string", "description": "Path to the target file."},
					"old_text": map[string]any{"type": "string", "description": "Exact text to replace."},
					"new_text": map[string]any{"type": "string", "description": "Replacement text."},
				},
				"required": []string{"path", "old_text", "new_text"},
			},
		},
		Run: registry.editFile,
	}
	registry.tools["list_dir"] = executionToolSpec{
		Descriptor: StructuredToolDescriptor{
			Name:        "list_dir",
			Description: "List files and subdirectories inside a workspace directory.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":   map[string]any{"type": "string", "description": "Directory path. Defaults to the workspace root."},
					"offset": map[string]any{"type": "integer", "description": "Optional zero-based entry offset for paginated directory listings."},
					"limit":  map[string]any{"type": "integer", "description": "Optional maximum number of entries to return."},
				},
			},
		},
		Run: registry.listDir,
	}
	registry.tools["exec"] = executionToolSpec{
		Descriptor: StructuredToolDescriptor{
			Name:        "exec",
			Description: "Execute a shell command inside the execution workspace, subject to safety guards.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command":         map[string]any{"type": "string", "description": "Shell command to execute."},
					"timeout_seconds": map[string]any{"type": "integer", "description": "Optional timeout in seconds. Defaults to 30 seconds."},
				},
				"required": []string{"command"},
			},
		},
		Run: registry.execCommand,
	}
	registry.tools["web_fetch"] = executionToolSpec{
		Descriptor: StructuredToolDescriptor{
			Name:        "web_fetch",
			Description: "Fetch page text from a URL and return a bounded text envelope.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]any{"type": "string", "description": "URL to fetch."},
				},
				"required": []string{"url"},
			},
		},
		Run: registry.webFetch,
	}
	registry.tools["web_search"] = executionToolSpec{
		Descriptor: StructuredToolDescriptor{
			Name:        "web_search",
			Description: "Run a compact web search and return the top hits.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string", "description": "Search query text."},
					"limit": map[string]any{"type": "integer", "description": "Optional maximum number of search hits to return."},
				},
				"required": []string{"query"},
			},
		},
		Run: registry.webSearch,
	}
	registry.tools["send_file"] = executionToolSpec{
		Descriptor: StructuredToolDescriptor{
			Name:        "send_file",
			Description: "Prepare a workspace file as a structured attachment result.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "Path to the file inside the workspace."},
				},
				"required": []string{"path"},
			},
		},
		Run: registry.sendFile,
	}
	registry.tools["spawn_subagent"] = executionToolSpec{
		Descriptor: StructuredToolDescriptor{
			Name:        "spawn_subagent",
			Description: "Create a delegated subagent job handle for a bounded task request.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"task": map[string]any{"type": "string", "description": "Delegated task description."},
				},
				"required": []string{"task"},
			},
		},
		Run: registry.spawnSubagent,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(registry)
		}
	}
	return registry
}

func (r *ExecutionToolRegistry) HasWorkspaceRoot() bool {
	return r != nil && strings.TrimSpace(r.workspaceRoot) != ""
}

func (r *ExecutionToolRegistry) Execute(ctx context.Context, name string, args map[string]any) ExecutionToolResult {
	tool, ok := r.tools[strings.TrimSpace(name)]
	if !ok {
		return executionError(fmt.Sprintf("unknown tool %q", name))
	}
	return tool.Run(ctx, args)
}

func (r *ExecutionToolRegistry) Descriptors() []StructuredToolDescriptor {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]StructuredToolDescriptor, 0, len(names))
	for _, name := range names {
		out = append(out, r.tools[name].Descriptor)
	}
	return out
}

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
	return ExecutionToolResult{
		Output:       fmt.Sprintf("wrote %s", resolved),
		FilesTouched: []string{resolved},
	}
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
	return ExecutionToolResult{
		Output:       fmt.Sprintf("appended %s", resolved),
		FilesTouched: []string{resolved},
	}
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
	content := paginateTextByLines(string(raw), offset, limit)
	return ExecutionToolResult{
		Output: content,
	}
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
	return ExecutionToolResult{
		Output:       fmt.Sprintf("edited %s", resolved),
		FilesTouched: []string{resolved},
	}
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
	names = paginateStrings(names, offset, limit)
	return ExecutionToolResult{Output: strings.Join(names, "\n")}
}

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
		timeoutSeconds, err := readOptionalIntArg(args, "timeout_seconds", defaultExecTimeoutSeconds)
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

	var stdout bytes.Buffer
	var stderr bytes.Buffer
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
		ContentBlocks: []ContentBlock{
			{
				Type:         "file",
				OutputRole:   "generated",
				Name:         attachment.Name,
				Path:         attachment.Path,
				MIMEType:     attachment.MIMEType,
				MediaType:    attachment.MediaType,
				AttachmentID: attachment.ID,
				SizeBytes:    attachment.SizeBytes,
			},
		},
		Metadata: map[string]any{
			"attachment": attachment,
		},
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
	return ExecutionToolResult{
		Output:  strings.TrimSpace(message),
		IsError: true,
		Status:  ExecutionToolResultStatusError,
	}
}

func executionAsk(message string) ExecutionToolResult {
	return ExecutionToolResult{
		Output:  strings.TrimSpace(message),
		IsError: true,
		Status:  ExecutionToolResultStatusAsk,
	}
}

func executionDeny(message string) ExecutionToolResult {
	return ExecutionToolResult{
		Output:  strings.TrimSpace(message),
		IsError: true,
		Status:  ExecutionToolResultStatusDeny,
	}
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

func paginateTextByLines(content string, offset, limit int) string {
	lines := strings.Split(content, "\n")
	if offset >= len(lines) {
		return "[END OF FILE - no content at this offset]"
	}
	sliced := paginateStrings(lines, offset, limit)
	return strings.Join(sliced, "\n")
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
