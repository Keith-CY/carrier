package baseagent

import (
	"fmt"
	"strings"
)

func normalizeSkillDefinition(skill SkillDefinition) SkillDefinition {
	skill.Name = strings.TrimSpace(strings.ToLower(skill.Name))
	skill.Summary = strings.TrimSpace(skill.Summary)
	skill.Keywords = normalizeSkillValues(skill.Keywords)
	skill.Tags = normalizeSkillValues(skill.Tags)
	skill.Source = strings.TrimSpace(skill.Source)
	skill.Provenance = strings.TrimSpace(skill.Provenance)
	skill.Version = strings.TrimSpace(skill.Version)
	skill.TargetVersion = strings.TrimSpace(skill.TargetVersion)
	skill.InstalledAt = strings.TrimSpace(skill.InstalledAt)
	skill.UpdatedAt = strings.TrimSpace(skill.UpdatedAt)
	skill.Health = strings.TrimSpace(strings.ToLower(skill.Health))
	skill.HealthDetail = strings.TrimSpace(skill.HealthDetail)
	skill.RemediationHint = strings.TrimSpace(skill.RemediationHint)
	skill.UpdateStatus = strings.TrimSpace(strings.ToLower(skill.UpdateStatus))
	if skill.Source == "" {
		skill.Source = "catalog"
	}
	if skill.Version == "" {
		skill.Version = "builtin"
	}
	if skill.Health == "" || skill.UpdateStatus == "" || skill.HealthDetail == "" || skill.RemediationHint == "" {
		health, updateStatus, updateAvailable, healthDetail, remediationHint := deriveSkillLifecycleStatus(skill)
		if skill.Health == "" {
			skill.Health = health
		}
		if skill.UpdateStatus == "" {
			skill.UpdateStatus = updateStatus
		}
		if skill.HealthDetail == "" {
			skill.HealthDetail = healthDetail
		}
		if skill.RemediationHint == "" {
			skill.RemediationHint = remediationHint
		}
		skill.UpdateAvailable = updateAvailable
	}
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
		Name:            skill.Name,
		Summary:         skill.Summary,
		Keywords:        append([]string(nil), skill.Keywords...),
		Tags:            append([]string(nil), skill.Tags...),
		Source:          skill.Source,
		Provenance:      skill.Provenance,
		Version:         skill.Version,
		TargetVersion:   skill.TargetVersion,
		InstalledAt:     skill.InstalledAt,
		UpdatedAt:       skill.UpdatedAt,
		Health:          skill.Health,
		HealthDetail:    skill.HealthDetail,
		RemediationHint: skill.RemediationHint,
		UpdateStatus:    skill.UpdateStatus,
		UpdateAvailable: skill.UpdateAvailable,
	}
}

func hydrateSkillDefinition(skill SkillDefinition, versionPins map[string]string, metadataByName map[string]SkillLifecycleMetadata, installed bool) SkillDefinition {
	cloned := cloneSkillDefinition(skill)
	if len(versionPins) != 0 {
		cloned.TargetVersion = strings.TrimSpace(versionPins[strings.TrimSpace(strings.ToLower(skill.Name))])
	}
	if meta, ok := metadataByName[strings.TrimSpace(strings.ToLower(cloned.Name))]; ok {
		cloned.Provenance = strings.TrimSpace(meta.Provenance)
		cloned.InstalledAt = strings.TrimSpace(meta.InstalledAt)
		cloned.UpdatedAt = strings.TrimSpace(meta.UpdatedAt)
	}
	if installed && strings.TrimSpace(cloned.Provenance) == "" {
		cloned.Provenance = buildSkillProvenanceSummary("catalog", cloned)
	}
	cloned.Health, cloned.UpdateStatus, cloned.UpdateAvailable, cloned.HealthDetail, cloned.RemediationHint = deriveSkillLifecycleStatus(cloned)
	return cloned
}

func deriveSkillLifecycleStatus(skill SkillDefinition) (health string, updateStatus string, updateAvailable bool, healthDetail string, remediationHint string) {
	currentVersion := strings.TrimSpace(skill.Version)
	targetVersion := strings.TrimSpace(skill.TargetVersion)
	switch {
	case targetVersion == "":
		return "healthy", "current", false, "Skill is aligned with its current catalog version.", ""
	case currentVersion == "":
		return "degraded", "unknown_current_version", true, fmt.Sprintf("Current installed version is unknown while target version %s is pinned.", targetVersion), fmt.Sprintf("Reinstall or update the skill to %s to restore version tracking.", targetVersion)
	case targetVersion == currentVersion:
		return "healthy", "pinned_current", false, fmt.Sprintf("Installed version %s matches the pinned target version.", currentVersion), "Clear the target pin if this skill should follow the catalog default."
	default:
		return "degraded", "update_available", true, fmt.Sprintf("Installed version %s differs from target version %s.", currentVersion, targetVersion), fmt.Sprintf("Update skill to %s or clear the target pin.", targetVersion)
	}
}

func applyDisabledSkillHints(skill SkillDefinition) SkillDefinition {
	detail := "Skill is installed but disabled in runtime."
	if strings.TrimSpace(skill.HealthDetail) != "" {
		skill.HealthDetail = detail + " " + strings.TrimSpace(skill.HealthDetail)
	} else {
		skill.HealthDetail = detail
	}
	hint := "Enable the skill to expose its guidance and tools in runtime."
	if strings.TrimSpace(skill.RemediationHint) != "" {
		skill.RemediationHint = hint + " " + strings.TrimSpace(skill.RemediationHint)
	} else {
		skill.RemediationHint = hint
	}
	return skill
}

func buildSkillProvenanceSummary(action string, skill SkillDefinition) string {
	source := strings.TrimSpace(skill.Source)
	if source == "" {
		source = "catalog"
	}
	switch strings.TrimSpace(strings.ToLower(action)) {
	case "install":
		return "managed install via " + source
	case "reinstall":
		return "managed reinstall via " + source
	case "update":
		return "managed update via " + source
	default:
		return source
	}
}

func appendSkillProvenanceHistory(existing, next string) string {
	existing = strings.TrimSpace(existing)
	next = strings.TrimSpace(next)
	switch {
	case existing == "":
		return next
	case next == "":
		return existing
	case strings.Contains(existing, next):
		return existing
	default:
		return existing + " -> " + next
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
