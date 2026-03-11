package baseagent

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type SkillsRegistry struct {
	store   SkillsStore
	catalog map[string]SkillDefinition
}

func NewSkillsRegistry(store SkillsStore, catalog []SkillDefinition) *SkillsRegistry {
	if store == nil {
		store = NewMemorySkillsStore()
	}
	registry := &SkillsRegistry{
		store:   store,
		catalog: map[string]SkillDefinition{},
	}
	for _, skill := range catalog {
		normalized := normalizeSkillDefinition(skill)
		if normalized.Name == "" {
			continue
		}
		registry.catalog[normalized.Name] = normalized
	}
	return registry
}

func (r *SkillsRegistry) ListInstalledSkills(ctx context.Context) []SkillDefinition {
	if r == nil {
		return nil
	}
	names, err := r.store.ListInstalledSkillNames(ctx)
	if err != nil {
		return nil
	}
	out := make([]SkillDefinition, 0, len(names))
	for _, name := range names {
		skill, ok := r.catalog[name]
		if !ok {
			continue
		}
		out = append(out, cloneSkillDefinition(skill))
	}
	return out
}

func (r *SkillsRegistry) SearchSkills(ctx context.Context, query string) []SkillDefinition {
	if r == nil {
		return nil
	}
	query = strings.TrimSpace(query)
	if query == "" {
		out := make([]SkillDefinition, 0, len(r.catalog))
		for _, skill := range r.catalog {
			out = append(out, cloneSkillDefinition(skill))
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		return out
	}

	type scoredSkill struct {
		skill SkillDefinition
		score int
	}
	scored := make([]scoredSkill, 0, len(r.catalog))
	for _, skill := range r.catalog {
		score := scoreSkillMatch(skill, query)
		if score <= 0 {
			continue
		}
		scored = append(scored, scoredSkill{skill: skill, score: score})
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].skill.Name < scored[j].skill.Name
		}
		return scored[i].score > scored[j].score
	})
	out := make([]SkillDefinition, 0, len(scored))
	for _, item := range scored {
		out = append(out, cloneSkillDefinition(item.skill))
	}
	return out
}

func (r *SkillsRegistry) InstallSkill(ctx context.Context, name string) (SkillDefinition, error) {
	if r == nil {
		return SkillDefinition{}, fmt.Errorf("skills registry is unavailable")
	}
	name = strings.TrimSpace(strings.ToLower(name))
	skill, ok := r.catalog[name]
	if !ok {
		return SkillDefinition{}, fmt.Errorf("skill %q is not available", name)
	}
	names, err := r.store.ListInstalledSkillNames(ctx)
	if err != nil {
		return SkillDefinition{}, err
	}
	names = append(names, name)
	if err := r.store.SetInstalledSkillNames(ctx, names); err != nil {
		return SkillDefinition{}, err
	}
	return cloneSkillDefinition(skill), nil
}

func (r *SkillsRegistry) RelevantSkillsSummary(ctx context.Context, message string) string {
	message = strings.TrimSpace(message)
	if r == nil || message == "" {
		return ""
	}
	installed := r.ListInstalledSkills(ctx)
	if len(installed) == 0 {
		return ""
	}

	type scoredSkill struct {
		skill SkillDefinition
		score int
	}
	scored := make([]scoredSkill, 0, len(installed))
	for _, skill := range installed {
		score := scoreSkillMatch(skill, message)
		if score <= 0 {
			continue
		}
		scored = append(scored, scoredSkill{skill: skill, score: score})
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].skill.Name < scored[j].skill.Name
		}
		return scored[i].score > scored[j].score
	})
	if len(scored) == 0 {
		return ""
	}
	if len(scored) > 3 {
		scored = scored[:3]
	}
	lines := make([]string, 0, len(scored))
	for _, item := range scored {
		lines = append(lines, fmt.Sprintf("- %s: %s", item.skill.Name, item.skill.Summary))
	}
	return strings.Join(lines, "\n")
}

func normalizeSkillDefinition(skill SkillDefinition) SkillDefinition {
	skill.Name = strings.TrimSpace(strings.ToLower(skill.Name))
	skill.Summary = strings.TrimSpace(skill.Summary)
	skill.Keywords = normalizeSkillValues(skill.Keywords)
	skill.Tags = normalizeSkillValues(skill.Tags)
	return skill
}

func normalizeSkillValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(strings.ToLower(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func cloneSkillDefinition(skill SkillDefinition) SkillDefinition {
	return SkillDefinition{
		Name:     skill.Name,
		Summary:  skill.Summary,
		Keywords: append([]string(nil), skill.Keywords...),
		Tags:     append([]string(nil), skill.Tags...),
	}
}

func scoreSkillMatch(skill SkillDefinition, query string) int {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return 0
	}
	score := 0
	if strings.Contains(query, skill.Name) {
		score += 5
	}
	for _, keyword := range skill.Keywords {
		if strings.Contains(query, keyword) {
			score += 4
		}
	}
	for _, tag := range skill.Tags {
		if strings.Contains(query, tag) {
			score += 2
		}
	}
	if summary := strings.ToLower(strings.TrimSpace(skill.Summary)); summary != "" && strings.Contains(query, summary) {
		score++
	}
	return score
}
