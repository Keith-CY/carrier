package baseagent

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

type MCPManager interface {
	ListStructuredTools() []StructuredToolDescriptor
	ExecuteTool(ctx context.Context, name string, args map[string]any) ExecutionToolResult
	CapabilitySummary() MCPCapabilitySummary
	ServerDetail(name string) (MCPServerCapability, error)
	SetServerEnabled(ctx context.Context, name string, enabled bool) error
	SetServerAttached(ctx context.Context, name string, attached bool) error
	UpdateServerConfig(ctx context.Context, name string, config string) error
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

func (m *StaticMCPManager) CapabilitySummary() MCPCapabilitySummary {
	if m == nil {
		return MCPCapabilitySummary{}
	}
	visible := m.ListStructuredTools()
	summary := MCPCapabilitySummary{
		Servers: []MCPServerCapability{
			{
				Name:             "static",
				Health:           "healthy",
				Enabled:          true,
				Manageable:       false,
				VisibleToolCount: len(visible),
				HiddenToolCount:  0,
			},
		},
		VisibleTools: make([]MCPToolCapability, 0, len(visible)),
	}
	for _, tool := range visible {
		summary.VisibleTools = append(summary.VisibleTools, MCPToolCapability{
			Name:        tool.Name,
			Description: tool.Description,
		})
	}
	sortMCPCapabilitySummary(&summary)
	return summary
}

func (m *StaticMCPManager) ServerDetail(name string) (MCPServerCapability, error) {
	if strings.TrimSpace(strings.ToLower(name)) != "static" {
		return MCPServerCapability{}, fmt.Errorf("mcp server %q is not available", strings.TrimSpace(name))
	}
	visible := m.ListStructuredTools()
	detail := MCPServerCapability{
		Name:             "static",
		Health:           "healthy",
		Enabled:          true,
		Manageable:       false,
		VisibleToolCount: len(visible),
		HiddenToolCount:  0,
		HealthDetail:     "Static MCP tools are wired into the runtime.",
		VisibleTools:     make([]MCPToolCapability, 0, len(visible)),
	}
	for _, tool := range visible {
		detail.VisibleTools = append(detail.VisibleTools, MCPToolCapability{
			Name:        tool.Name,
			Description: tool.Description,
		})
	}
	return detail, nil
}

func (m *StaticMCPManager) SetServerEnabled(_ context.Context, name string, enabled bool) error {
	if strings.TrimSpace(strings.ToLower(name)) != "static" {
		return fmt.Errorf("mcp server %q is not available", strings.TrimSpace(name))
	}
	if !enabled {
		return fmt.Errorf("mcp server %q cannot be disabled", strings.TrimSpace(name))
	}
	return nil
}

func (m *StaticMCPManager) SetServerAttached(_ context.Context, name string, attached bool) error {
	if strings.TrimSpace(strings.ToLower(name)) != "static" {
		return fmt.Errorf("mcp server %q is not available", strings.TrimSpace(name))
	}
	if !attached {
		return fmt.Errorf("mcp server %q cannot be detached", strings.TrimSpace(name))
	}
	return nil
}

func (m *StaticMCPManager) UpdateServerConfig(_ context.Context, name string, _ string) error {
	if strings.TrimSpace(strings.ToLower(name)) != "static" {
		return fmt.Errorf("mcp server %q is not available", strings.TrimSpace(name))
	}
	return nil
}
