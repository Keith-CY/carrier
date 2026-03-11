package baseagent

import (
	"context"
	"strings"
)

type SkillsLoader interface {
	RelevantSkillsSummary(ctx context.Context, message string) string
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
