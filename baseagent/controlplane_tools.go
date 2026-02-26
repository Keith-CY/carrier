package baseagent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// ToolInvocation is the normalized tool call data.
type ToolInvocation struct {
	Name     string
	Args     map[string]string
	RawInput string
}

// ToolMatcher decides whether a tool should handle a user message.
type ToolMatcher func(input string) (ToolInvocation, bool)

// ToolRunner executes a matched tool.
type ToolRunner func(ctx context.Context, call ToolInvocation) (ChatResponse, error)

// ToolSpec registers one tool in the registry.
type ToolSpec struct {
	Name        string
	Description string
	Match       ToolMatcher
	Run         ToolRunner
}

// ToolRegistry routes user input to registered tools.
type ToolRegistry struct {
	mu     sync.RWMutex
	order  []string
	byName map[string]ToolSpec
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		order:  []string{},
		byName: map[string]ToolSpec{},
	}
}

func (r *ToolRegistry) RegisterTool(spec ToolSpec) error {
	if r == nil {
		return fmt.Errorf("tool registry is nil")
	}
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		return fmt.Errorf("tool name is required")
	}
	if spec.Match == nil {
		return fmt.Errorf("tool matcher is required (%s)", name)
	}
	if spec.Run == nil {
		return fmt.Errorf("tool runner is required (%s)", name)
	}

	spec.Name = name
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byName[name]; !exists {
		r.order = append(r.order, name)
	}
	r.byName[name] = spec
	return nil
}

func (r *ToolRegistry) RouteMessage(ctx context.Context, input string) (ChatResponse, bool, error) {
	if r == nil {
		return ChatResponse{}, false, nil
	}
	r.mu.RLock()
	order := append([]string(nil), r.order...)
	tools := make(map[string]ToolSpec, len(r.byName))
	for k, v := range r.byName {
		tools[k] = v
	}
	r.mu.RUnlock()

	for _, name := range order {
		spec := tools[name]
		call, matched := spec.Match(input)
		if !matched {
			continue
		}
		if call.Name == "" {
			call.Name = spec.Name
		}
		if call.Args == nil {
			call.Args = map[string]string{}
		}
		call.RawInput = input
		resp, err := spec.Run(ctx, call)
		return resp, true, err
	}
	return ChatResponse{}, false, nil
}

func (r *ToolRegistry) ExecuteTool(ctx context.Context, name string, args map[string]string) (ChatResponse, error) {
	if r == nil {
		return ChatResponse{}, fmt.Errorf("tool registry is nil")
	}
	name = strings.TrimSpace(name)
	r.mu.RLock()
	spec, ok := r.byName[name]
	r.mu.RUnlock()
	if !ok {
		return ChatResponse{}, fmt.Errorf("tool not found: %s", name)
	}
	return spec.Run(ctx, ToolInvocation{
		Name: name,
		Args: args,
	})
}

func (r *ToolRegistry) ListToolDescriptors() []ToolDescriptor {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ToolDescriptor, 0, len(r.byName))
	for _, name := range r.order {
		spec := r.byName[name]
		out = append(out, ToolDescriptor{
			Name:        spec.Name,
			Description: spec.Description,
		})
	}
	return out
}

func (r *ToolRegistry) ListToolNames() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := append([]string(nil), r.order...)
	return out
}

func (r *ToolRegistry) RenderToolSummary() string {
	descriptors := r.ListToolDescriptors()
	if len(descriptors) == 0 {
		return "No tools are registered."
	}
	lines := make([]string, 0, len(descriptors)+1)
	lines = append(lines, fmt.Sprintf("Available tools (%d):", len(descriptors)))
	for _, d := range descriptors {
		if strings.TrimSpace(d.Description) == "" {
			lines = append(lines, "- "+d.Name)
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", d.Name, d.Description))
	}
	return strings.Join(lines, "\n")
}

func (r *ToolRegistry) SortedToolNames() []string {
	names := r.ListToolNames()
	sort.Strings(names)
	return names
}
