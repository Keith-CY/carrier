package baseagent

import (
	"context"
	"strings"
)

type SkillDefinition struct {
	Name            string   `json:"name"`
	Summary         string   `json:"summary,omitempty"`
	Keywords        []string `json:"keywords,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	Source          string   `json:"source,omitempty"`
	Provenance      string   `json:"provenance,omitempty"`
	Version         string   `json:"version,omitempty"`
	TargetVersion   string   `json:"targetVersion,omitempty"`
	InstalledAt     string   `json:"installedAt,omitempty"`
	UpdatedAt       string   `json:"updatedAt,omitempty"`
	Health          string   `json:"health,omitempty"`
	HealthDetail    string   `json:"healthDetail,omitempty"`
	RemediationHint string   `json:"remediationHint,omitempty"`
	UpdateStatus    string   `json:"updateStatus,omitempty"`
	UpdateAvailable bool     `json:"updateAvailable,omitempty"`
}

type SkillsLoader interface {
	RelevantSkillsSummary(ctx context.Context, message string) string
	ListInstalledSkills(ctx context.Context) []SkillDefinition
	SearchSkills(ctx context.Context, query string) []SkillDefinition
	InstallSkill(ctx context.Context, name string) (SkillDefinition, error)
	UpdateSkill(ctx context.Context, name, version string) (SkillDefinition, error)
	UninstallSkill(ctx context.Context, name string) (SkillDefinition, error)
}

func composeSkillAwareSystemPrompt(basePrompt, skillSummary string) string {
	basePrompt = strings.TrimSpace(basePrompt)
	skillSummary = strings.TrimSpace(skillSummary)
	if skillSummary == "" {
		return basePrompt
	}
	if basePrompt == "" {
		return "Relevant skills summary:\n" + skillSummary
	}
	return basePrompt + "\n\nRelevant skills summary:\n" + skillSummary
}
