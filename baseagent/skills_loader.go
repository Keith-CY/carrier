package baseagent

import (
	"context"
	"strings"
)

type SkillDefinition struct {
	Name     string
	Summary  string
	Keywords []string
	Tags     []string
}

type SkillsLoader interface {
	RelevantSkillsSummary(ctx context.Context, message string) string
	ListInstalledSkills(ctx context.Context) []SkillDefinition
	SearchSkills(ctx context.Context, query string) []SkillDefinition
	InstallSkill(ctx context.Context, name string) (SkillDefinition, error)
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
