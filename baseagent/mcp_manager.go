package baseagent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
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
	mu             sync.RWMutex
	serverOrder    []string
	serverHooks    map[string]MCPServerHooks
	serverState    map[string]bool
	serverAttached map[string]bool
	serverConfig   map[string]string
	toolOrder      []string
	tools          map[string]managedMCPTool
	aliases        map[string]string
}

func NewManagedMCPManager(cfg MCPConfig, opts ManagedMCPManagerOptions) (*ManagedMCPManager, error) {
	if err := ValidateMCPConfig(cfg); err != nil {
		return nil, err
	}
	cfg = normalizeMCPConfig(cfg)
	manager := &ManagedMCPManager{
		serverOrder:    make([]string, 0, len(cfg.Servers)),
		serverHooks:    map[string]MCPServerHooks{},
		serverState:    map[string]bool{},
		serverAttached: map[string]bool{},
		serverConfig:   map[string]string{},
		toolOrder:      []string{},
		tools:          map[string]managedMCPTool{},
		aliases:        map[string]string{},
	}
	for name, hooks := range opts.ServerHooks {
		manager.serverHooks[strings.TrimSpace(strings.ToLower(name))] = hooks
	}
	for _, server := range cfg.Servers {
		manager.serverOrder = append(manager.serverOrder, server.Name)
		manager.serverState[server.Name] = true
		manager.serverAttached[server.Name] = true
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
		if !m.serverState[tool.serverName] || !m.serverAttached[tool.serverName] {
			continue
		}
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
	if !m.serverState[tool.serverName] {
		return executionError(fmt.Sprintf("mcp server %s is disabled", tool.serverName))
	}
	if !m.serverAttached[tool.serverName] {
		return executionError(fmt.Sprintf("mcp server %s is detached", tool.serverName))
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
			Name:       serverName,
			Health:     "stopped",
			Enabled:    m.serverState[serverName],
			Attached:   m.serverAttached[serverName],
			Manageable: true,
		}
		if !m.serverAttached[serverName] {
			serverSummary.Health = "detached"
		} else if m.serverState[serverName] {
			serverSummary.Health = "healthy"
		}
		serverSummary.ConfigDigest = mcpConfigDigest(m.serverConfig[serverName])
		serverSummary.ConfigSummary = summarizeMCPConfig(m.serverConfig[serverName])
		for _, toolName := range m.toolOrder {
			tool := m.tools[toolName]
			if tool.serverName != serverName {
				continue
			}
			capability := MCPToolCapability{
				Name:        tool.descriptor.Name,
				Description: tool.descriptor.Description,
			}
			if tool.config.Hidden {
				serverSummary.HiddenToolCount++
				serverSummary.HiddenTools = append(serverSummary.HiddenTools, capability)
				continue
			}
			serverSummary.VisibleToolCount++
			serverSummary.VisibleTools = append(serverSummary.VisibleTools, capability)
			if !m.serverState[serverName] || !m.serverAttached[serverName] {
				continue
			}
			summary.VisibleTools = append(summary.VisibleTools, capability)
		}
		summary.Servers = append(summary.Servers, serverSummary)
	}
	sortMCPCapabilitySummary(&summary)
	return summary
}

func (m *ManagedMCPManager) ServerDetail(name string) (MCPServerCapability, error) {
	if m == nil {
		return MCPServerCapability{}, fmt.Errorf("mcp manager is unavailable")
	}
	serverName := strings.TrimSpace(strings.ToLower(name))
	if serverName == "" {
		return MCPServerCapability{}, fmt.Errorf("mcp server name is required")
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	enabled, knownServer := m.serverState[serverName]
	attached := m.serverAttached[serverName]
	if !knownServer {
		return MCPServerCapability{}, fmt.Errorf("mcp server %q is not available", serverName)
	}
	detail := MCPServerCapability{
		Name:       serverName,
		Health:     "stopped",
		Enabled:    enabled,
		Attached:   attached,
		Manageable: true,
	}
	detail.ConfigDigest = mcpConfigDigest(m.serverConfig[serverName])
	detail.ConfigSummary = summarizeMCPConfig(m.serverConfig[serverName])
	if !attached {
		detail.Health = "detached"
		detail.HealthDetail = "server is detached from the managed runtime"
		if detail.ConfigDigest != "" {
			detail.RemediationHint = "Attach the MCP server to re-apply the saved config and expose its tool surface."
		} else {
			detail.RemediationHint = "Attach the MCP server and enable it to expose its tool surface."
		}
	} else if enabled {
		detail.Health = "healthy"
		detail.HealthDetail = "connected to managed tool runtime"
		detail.RemediationHint = "Disable MCP if the tool surface becomes noisy or conflicts with local tools."
	} else {
		detail.HealthDetail = "server is disabled"
		if detail.ConfigDigest != "" {
			detail.RemediationHint = "Enable MCP to expose its tool surface; the saved config will be reused."
		} else {
			detail.RemediationHint = "Enable MCP to expose its tool surface to the agent runtime."
		}
	}
	for _, toolName := range m.toolOrder {
		tool := m.tools[toolName]
		if tool.serverName != serverName {
			continue
		}
		capability := MCPToolCapability{
			Name:        tool.descriptor.Name,
			Description: tool.descriptor.Description,
		}
		if tool.config.Hidden {
			detail.HiddenToolCount++
			detail.HiddenTools = append(detail.HiddenTools, capability)
			continue
		}
		detail.VisibleToolCount++
		detail.VisibleTools = append(detail.VisibleTools, capability)
	}
	sort.Slice(detail.VisibleTools, func(i, j int) bool {
		return detail.VisibleTools[i].Name < detail.VisibleTools[j].Name
	})
	sort.Slice(detail.HiddenTools, func(i, j int) bool {
		return detail.HiddenTools[i].Name < detail.HiddenTools[j].Name
	})
	return detail, nil
}

func (m *ManagedMCPManager) SetServerEnabled(ctx context.Context, name string, enabled bool) error {
	if m == nil {
		return fmt.Errorf("mcp manager is unavailable")
	}
	serverName := strings.TrimSpace(strings.ToLower(name))
	if serverName == "" {
		return fmt.Errorf("mcp server name is required")
	}

	m.mu.RLock()
	hook := m.serverHooks[serverName]
	_, knownServer := m.serverState[serverName]
	current := m.serverState[serverName]
	attached := m.serverAttached[serverName]
	m.mu.RUnlock()
	if !knownServer {
		return fmt.Errorf("mcp server %q is not available", serverName)
	}
	if enabled && !attached {
		return fmt.Errorf("mcp server %q is detached", serverName)
	}
	if current == enabled {
		return nil
	}

	if enabled {
		if hook.Start != nil {
			if err := hook.Start(ctx); err != nil {
				return err
			}
		}
	} else {
		if hook.Stop != nil {
			if err := hook.Stop(ctx); err != nil {
				return err
			}
		}
	}

	m.mu.Lock()
	m.serverState[serverName] = enabled
	m.mu.Unlock()
	return nil
}

func (m *ManagedMCPManager) SetServerAttached(ctx context.Context, name string, attached bool) error {
	if m == nil {
		return fmt.Errorf("mcp manager is unavailable")
	}
	serverName := strings.TrimSpace(strings.ToLower(name))
	if serverName == "" {
		return fmt.Errorf("mcp server name is required")
	}
	m.mu.RLock()
	currentAttached, knownServer := m.serverAttached[serverName]
	currentEnabled := m.serverState[serverName]
	hook := m.serverHooks[serverName]
	m.mu.RUnlock()
	if !knownServer {
		return fmt.Errorf("mcp server %q is not available", serverName)
	}
	if currentAttached == attached {
		return nil
	}
	if !attached && currentEnabled && hook.Stop != nil {
		if err := hook.Stop(ctx); err != nil {
			return err
		}
	}
	if attached && currentEnabled && hook.Start != nil {
		if err := hook.Start(ctx); err != nil {
			return err
		}
	}
	m.mu.Lock()
	m.serverAttached[serverName] = attached
	if !attached {
		m.serverState[serverName] = false
	}
	m.mu.Unlock()
	return nil
}

func (m *ManagedMCPManager) UpdateServerConfig(_ context.Context, name string, config string) error {
	if m == nil {
		return fmt.Errorf("mcp manager is unavailable")
	}
	serverName := strings.TrimSpace(strings.ToLower(name))
	if serverName == "" {
		return fmt.Errorf("mcp server name is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.serverState[serverName]; !ok {
		return fmt.Errorf("mcp server %q is not available", serverName)
	}
	m.serverConfig[serverName] = strings.TrimSpace(config)
	return nil
}

func mcpConfigDigest(config string) string {
	config = strings.TrimSpace(config)
	if config == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(config))
	return hex.EncodeToString(sum[:8])
}

func summarizeMCPConfig(config string) string {
	config = strings.TrimSpace(config)
	if config == "" {
		return ""
	}
	line := strings.TrimSpace(strings.Split(config, "\n")[0])
	if len(line) > 96 {
		line = line[:96]
	}
	return line
}
