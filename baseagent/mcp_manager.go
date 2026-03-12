package baseagent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

type MCPManager interface {
	ListStructuredTools() []StructuredToolDescriptor
	ExecuteTool(ctx context.Context, name string, args map[string]any) ExecutionToolResult
	CapabilitySummary() MCPCapabilitySummary
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

type MCPServerHooks struct {
	Start func(context.Context) error
	Stop  func(context.Context) error
}

type MCPToolRunner func(context.Context, map[string]any) ExecutionToolResult

type ManagedMCPManagerOptions struct {
	ServerHooks map[string]MCPServerHooks
	ToolRunners map[string]MCPToolRunner
}

type managedMCPTool struct {
	serverName string
	config     MCPToolConfig
	descriptor StructuredToolDescriptor
	run        MCPToolRunner
}

type ManagedMCPManager struct {
	mu          sync.RWMutex
	serverOrder []string
	serverHooks map[string]MCPServerHooks
	serverState map[string]bool
	toolOrder   []string
	tools       map[string]managedMCPTool
	aliases     map[string]string
}

func NewManagedMCPManager(cfg MCPConfig, opts ManagedMCPManagerOptions) (*ManagedMCPManager, error) {
	if err := ValidateMCPConfig(cfg); err != nil {
		return nil, err
	}
	cfg = normalizeMCPConfig(cfg)
	manager := &ManagedMCPManager{
		serverOrder: make([]string, 0, len(cfg.Servers)),
		serverHooks: map[string]MCPServerHooks{},
		serverState: map[string]bool{},
		toolOrder:   []string{},
		tools:       map[string]managedMCPTool{},
		aliases:     map[string]string{},
	}
	for name, hooks := range opts.ServerHooks {
		manager.serverHooks[strings.TrimSpace(strings.ToLower(name))] = hooks
	}
	for _, server := range cfg.Servers {
		manager.serverOrder = append(manager.serverOrder, server.Name)
		manager.serverState[server.Name] = false
		for _, tool := range server.Tools {
			manager.toolOrder = append(manager.toolOrder, tool.Name)
			manager.tools[tool.Name] = managedMCPTool{
				serverName: server.Name,
				config:     tool,
				descriptor: StructuredToolDescriptor{
					Name:        tool.Name,
					Description: tool.Description,
					Parameters:  cloneToolSchema(tool.Parameters),
				},
				run: opts.ToolRunners[tool.Name],
			}
			for _, alias := range tool.Aliases {
				manager.aliases[alias] = tool.Name
			}
		}
	}
	return manager, nil
}

func (m *ManagedMCPManager) Start(ctx context.Context) error {
	if m == nil {
		return fmt.Errorf("mcp manager is unavailable")
	}
	m.mu.Lock()
	serverOrder := append([]string(nil), m.serverOrder...)
	hooks := make(map[string]MCPServerHooks, len(m.serverHooks))
	for name, hook := range m.serverHooks {
		hooks[name] = hook
	}
	m.mu.Unlock()

	for _, server := range serverOrder {
		hook := hooks[server]
		if hook.Start != nil {
			if err := hook.Start(ctx); err != nil {
				return err
			}
		}
		m.mu.Lock()
		m.serverState[server] = true
		m.mu.Unlock()
	}
	return nil
}

func (m *ManagedMCPManager) Stop(ctx context.Context) error {
	if m == nil {
		return fmt.Errorf("mcp manager is unavailable")
	}
	m.mu.Lock()
	serverOrder := append([]string(nil), m.serverOrder...)
	hooks := make(map[string]MCPServerHooks, len(m.serverHooks))
	for name, hook := range m.serverHooks {
		hooks[name] = hook
	}
	m.mu.Unlock()

	for i := len(serverOrder) - 1; i >= 0; i-- {
		server := serverOrder[i]
		hook := hooks[server]
		if hook.Stop != nil {
			if err := hook.Stop(ctx); err != nil {
				return err
			}
		}
		m.mu.Lock()
		m.serverState[server] = false
		m.mu.Unlock()
	}
	return nil
}

func (m *ManagedMCPManager) ListStructuredTools() []StructuredToolDescriptor {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]StructuredToolDescriptor, 0, len(m.toolOrder))
	for _, name := range m.toolOrder {
		tool := m.tools[name]
		if tool.config.Hidden {
			continue
		}
		out = append(out, StructuredToolDescriptor{
			Name:        tool.descriptor.Name,
			Description: tool.descriptor.Description,
			Parameters:  cloneToolSchema(tool.descriptor.Parameters),
		})
	}
	return out
}

func (m *ManagedMCPManager) ExecuteTool(ctx context.Context, name string, args map[string]any) ExecutionToolResult {
	if m == nil {
		return executionError("mcp manager is unavailable")
	}
	canonical := strings.TrimSpace(strings.ToLower(name))
	m.mu.RLock()
	if resolved, ok := m.aliases[canonical]; ok {
		canonical = resolved
	}
	tool, ok := m.tools[canonical]
	m.mu.RUnlock()
	if !ok {
		return executionError("unknown mcp tool")
	}
	if tool.run == nil {
		return executionError(fmt.Sprintf("mcp tool %s is not configured", canonical))
	}
	return tool.run(ctx, args)
}

func (m *ManagedMCPManager) SortedVisibleToolNames() []string {
	tools := m.ListStructuredTools()
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	return names
}

func (m *ManagedMCPManager) CapabilitySummary() MCPCapabilitySummary {
	if m == nil {
		return MCPCapabilitySummary{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	summary := MCPCapabilitySummary{
		Servers:      make([]MCPServerCapability, 0, len(m.serverOrder)),
		VisibleTools: []MCPToolCapability{},
	}
	for _, serverName := range m.serverOrder {
		serverSummary := MCPServerCapability{
			Name:   serverName,
			Health: "stopped",
		}
		if m.serverState[serverName] {
			serverSummary.Health = "healthy"
		}
		for _, toolName := range m.toolOrder {
			tool := m.tools[toolName]
			if tool.serverName != serverName {
				continue
			}
			if tool.config.Hidden {
				serverSummary.HiddenToolCount++
				continue
			}
			serverSummary.VisibleToolCount++
			summary.VisibleTools = append(summary.VisibleTools, MCPToolCapability{
				Name:        tool.descriptor.Name,
				Description: tool.descriptor.Description,
			})
		}
		summary.Servers = append(summary.Servers, serverSummary)
	}
	sortMCPCapabilitySummary(&summary)
	return summary
}
