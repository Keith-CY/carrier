package baseagent

import (
	"context"
	"sort"
)

type MCPToolCapability struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type MCPServerCapability struct {
	Name             string `json:"name"`
	Health           string `json:"health"`
	VisibleToolCount int    `json:"visibleToolCount"`
	HiddenToolCount  int    `json:"hiddenToolCount"`
}

type MCPCapabilitySummary struct {
	Servers      []MCPServerCapability `json:"servers,omitempty"`
	VisibleTools []MCPToolCapability   `json:"visibleTools,omitempty"`
}

type RuntimeSkillSummary struct {
	InstalledCount int `json:"installedCount"`
	EnabledCount   int `json:"enabledCount"`
	DisabledCount  int `json:"disabledCount"`
}

type RuntimeCapabilitySummary struct {
	Skills       []RuntimeSkillCapability `json:"skills,omitempty"`
	SkillSummary RuntimeSkillSummary      `json:"skillSummary"`
	MCP          MCPCapabilitySummary     `json:"mcp"`
}

type runtimeSkillCapabilityLoader interface {
	SkillsLoader
	ListRuntimeSkillCapabilities(ctx context.Context) []RuntimeSkillCapability
	SetSkillEnabled(ctx context.Context, name string, enabled bool) error
}

type mcpCapabilityReporter interface {
	CapabilitySummary() MCPCapabilitySummary
}

func (r *Runtime) CapabilitySummary(ctx context.Context) RuntimeCapabilitySummary {
	if r == nil {
		return RuntimeCapabilitySummary{}
	}
	summary := RuntimeCapabilitySummary{}
	if loader, ok := r.skillsLoader.(runtimeSkillCapabilityLoader); ok {
		summary.Skills = loader.ListRuntimeSkillCapabilities(ctx)
	} else if r.skillsLoader != nil {
		installed := r.skillsLoader.ListInstalledSkills(ctx)
		summary.Skills = make([]RuntimeSkillCapability, 0, len(installed))
		for _, skill := range installed {
			summary.Skills = append(summary.Skills, RuntimeSkillCapability{
				Name:     skill.Name,
				Summary:  skill.Summary,
				Keywords: append([]string(nil), skill.Keywords...),
				Tags:     append([]string(nil), skill.Tags...),
				Enabled:  true,
			})
		}
	}
	if reporter, ok := r.mcpManager.(mcpCapabilityReporter); ok {
		summary.MCP = reporter.CapabilitySummary()
	}
	summary.SkillSummary = buildRuntimeSkillSummary(summary.Skills)
	return summary
}

func (r *Runtime) SetSkillEnabled(ctx context.Context, name string, enabled bool) error {
	if r == nil || r.skillsLoader == nil {
		return nil
	}
	loader, ok := r.skillsLoader.(runtimeSkillCapabilityLoader)
	if !ok {
		return nil
	}
	return loader.SetSkillEnabled(ctx, name, enabled)
}

func sortMCPCapabilitySummary(summary *MCPCapabilitySummary) {
	if summary == nil {
		return
	}
	sort.Slice(summary.Servers, func(i, j int) bool {
		return summary.Servers[i].Name < summary.Servers[j].Name
	})
	sort.Slice(summary.VisibleTools, func(i, j int) bool {
		return summary.VisibleTools[i].Name < summary.VisibleTools[j].Name
	})
}

func buildRuntimeSkillSummary(skills []RuntimeSkillCapability) RuntimeSkillSummary {
	summary := RuntimeSkillSummary{InstalledCount: len(skills)}
	for _, skill := range skills {
		if skill.Enabled {
			summary.EnabledCount++
			continue
		}
		summary.DisabledCount++
	}
	return summary
}
