package baseagent

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
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
	Output          string
	Attachments     []AttachmentRef
	ContentBlocks   []ContentBlock
	FilesTouched    []string
	Stdout          string
	Stderr          string
	ExitCode        int
	IsError         bool
	Status          ExecutionToolResultStatus
	PolicyReason    string
	PolicyRuleID    string
	GuardrailEvents []GuardrailEvent
	Metadata        map[string]any
}

type WebSearchHit struct {
	Title   string
	URL     string
	Snippet string
}

type ExecutionAttachment = AttachmentRef

type SubagentRequest struct {
	Task     string
	Contract *DelegationContract
}

type SubagentJobHandle struct {
	JobID      string `json:"jobId"`
	Status     string `json:"status"`
	Summary    string `json:"summary"`
	ContractID string `json:"contractId,omitempty"`
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
