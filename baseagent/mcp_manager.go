package baseagent

import (
	"context"
	"strings"
	"sync"
)

type MCPManager interface {
	ListStructuredTools() []StructuredToolDescriptor
	ExecuteTool(ctx context.Context, name string, args map[string]any) ExecutionToolResult
}

type staticMCPTool struct {
	descriptor StructuredToolDescriptor
	run        func(context.Context, map[string]any) ExecutionToolResult
}

type StaticMCPManager struct {
	mu    sync.RWMutex
	tools map[string]staticMCPTool
}

func NewStaticMCPManager() *StaticMCPManager {
	return &StaticMCPManager{
		tools: map[string]staticMCPTool{},
	}
}

func (m *StaticMCPManager) RegisterTool(name string, descriptor StructuredToolDescriptor, run func(context.Context, map[string]any) ExecutionToolResult) {
	if m == nil || run == nil {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	descriptor.Name = name
	descriptor.Parameters = cloneToolSchema(descriptor.Parameters)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.tools[name] = staticMCPTool{
		descriptor: descriptor,
		run:        run,
	}
}

func (m *StaticMCPManager) ListStructuredTools() []StructuredToolDescriptor {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]StructuredToolDescriptor, 0, len(m.tools))
	for _, tool := range m.tools {
		out = append(out, StructuredToolDescriptor{
			Name:        tool.descriptor.Name,
			Description: tool.descriptor.Description,
			Parameters:  cloneToolSchema(tool.descriptor.Parameters),
		})
	}
	return out
}

func (m *StaticMCPManager) ExecuteTool(ctx context.Context, name string, args map[string]any) ExecutionToolResult {
	if m == nil {
		return executionError("mcp manager is unavailable")
	}
	m.mu.RLock()
	tool, ok := m.tools[strings.TrimSpace(name)]
	m.mu.RUnlock()
	if !ok {
		return executionError("unknown mcp tool")
	}
	return tool.run(ctx, args)
}
