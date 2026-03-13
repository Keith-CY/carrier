package baseagent

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type RuntimeSkillCapability struct {
	Name            string   `json:"name"`
	Summary         string   `json:"summary,omitempty"`
	Keywords        []string `json:"keywords,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	Source          string   `json:"source,omitempty"`
	Version         string   `json:"version,omitempty"`
	TargetVersion   string   `json:"targetVersion,omitempty"`
	Health          string   `json:"health,omitempty"`
	UpdateStatus    string   `json:"updateStatus,omitempty"`
	UpdateAvailable bool     `json:"updateAvailable,omitempty"`
	Enabled         bool     `json:"enabled"`
}

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
	versionPins, err := r.store.ListSkillVersionPins(ctx)
	if err != nil {
		return nil
	}
	out := make([]SkillDefinition, 0, len(names))
	for _, name := range names {
		skill, ok := r.catalog[name]
		if !ok {
			continue
		}
		out = append(out, hydrateSkillDefinitionTargetVersion(skill, versionPins))
	}
	return out
}

func (r *SkillsRegistry) SearchSkills(ctx context.Context, query string) []SkillDefinition {
	if r == nil {
		return nil
	}
	query = strings.TrimSpace(query)
	versionPins, _ := r.store.ListSkillVersionPins(ctx)
	if query == "" {
		out := make([]SkillDefinition, 0, len(r.catalog))
		for _, skill := range r.catalog {
			out = append(out, hydrateSkillDefinitionTargetVersion(skill, versionPins))
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
		out = append(out, hydrateSkillDefinitionTargetVersion(item.skill, versionPins))
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
	enabledNames, err := r.store.ListEnabledSkillNames(ctx)
	if err != nil {
		return SkillDefinition{}, err
	}
	enabledNames = append(enabledNames, name)
	if err := r.store.SetEnabledSkillNames(ctx, enabledNames); err != nil {
		return SkillDefinition{}, err
	}
	return hydrateSkillDefinitionTargetVersion(skill, nil), nil
}

func (r *SkillsRegistry) UpdateSkill(ctx context.Context, name, version string) (SkillDefinition, error) {
	if r == nil {
		return SkillDefinition{}, fmt.Errorf("skills registry is unavailable")
	}
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return SkillDefinition{}, fmt.Errorf("skill name is required")
	}
	skill, ok := r.catalog[name]
	if !ok {
		return SkillDefinition{}, fmt.Errorf("skill %q is not available", name)
	}
	installedNames, err := r.store.ListInstalledSkillNames(ctx)
	if err != nil {
		return SkillDefinition{}, err
	}
	installedSet := map[string]struct{}{}
	for _, installed := range installedNames {
		installedSet[installed] = struct{}{}
	}
	if _, ok := installedSet[name]; !ok {
		return SkillDefinition{}, fmt.Errorf("skill %q is not installed", name)
	}
	versionPins, err := r.store.ListSkillVersionPins(ctx)
	if err != nil {
		return SkillDefinition{}, err
	}
	if versionPins == nil {
		versionPins = map[string]string{}
	}
	trimmedVersion := strings.TrimSpace(version)
	if trimmedVersion == "" {
		delete(versionPins, name)
	} else {
		versionPins[name] = trimmedVersion
	}
	if err := r.store.SetSkillVersionPins(ctx, versionPins); err != nil {
		return SkillDefinition{}, err
	}
	return hydrateSkillDefinitionTargetVersion(skill, versionPins), nil
}

func (r *SkillsRegistry) UninstallSkill(ctx context.Context, name string) (SkillDefinition, error) {
	if r == nil {
		return SkillDefinition{}, fmt.Errorf("skills registry is unavailable")
	}
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return SkillDefinition{}, fmt.Errorf("skill name is required")
	}
	skill, ok := r.catalog[name]
	if !ok {
		return SkillDefinition{}, fmt.Errorf("skill %q is not available", name)
	}
	installedNames, err := r.store.ListInstalledSkillNames(ctx)
	if err != nil {
		return SkillDefinition{}, err
	}
	found := false
	filteredInstalled := make([]string, 0, len(installedNames))
	for _, installed := range installedNames {
		if installed == name {
			found = true
			continue
		}
		filteredInstalled = append(filteredInstalled, installed)
	}
	if !found {
		return SkillDefinition{}, fmt.Errorf("skill %q is not installed", name)
	}
	if err := r.store.SetInstalledSkillNames(ctx, filteredInstalled); err != nil {
		return SkillDefinition{}, err
	}
	enabledNames, err := r.store.ListEnabledSkillNames(ctx)
	if err != nil {
		return SkillDefinition{}, err
	}
	filteredEnabled := make([]string, 0, len(enabledNames))
	for _, enabled := range enabledNames {
		if enabled == name {
			continue
		}
		filteredEnabled = append(filteredEnabled, enabled)
	}
	if err := r.store.SetEnabledSkillNames(ctx, filteredEnabled); err != nil {
		return SkillDefinition{}, err
	}
	versionPins, err := r.store.ListSkillVersionPins(ctx)
	if err != nil {
		return SkillDefinition{}, err
	}
	delete(versionPins, name)
	if err := r.store.SetSkillVersionPins(ctx, versionPins); err != nil {
		return SkillDefinition{}, err
	}
	return hydrateSkillDefinitionTargetVersion(skill, versionPins), nil
}

func (r *SkillsRegistry) ListRuntimeSkillCapabilities(ctx context.Context) []RuntimeSkillCapability {
	if r == nil {
		return nil
	}
	names, err := r.store.ListInstalledSkillNames(ctx)
	if err != nil {
		return nil
	}
	enabledNames, err := r.store.ListEnabledSkillNames(ctx)
	if err != nil {
		return nil
	}
	enabledSet := map[string]struct{}{}
	for _, name := range enabledNames {
		enabledSet[name] = struct{}{}
	}
	versionPins, err := r.store.ListSkillVersionPins(ctx)
	if err != nil {
		return nil
	}
	out := make([]RuntimeSkillCapability, 0, len(names))
	for _, name := range names {
		skill, ok := r.catalog[name]
		if !ok {
			continue
		}
		hydrated := hydrateSkillDefinitionTargetVersion(skill, versionPins)
		health, updateStatus, updateAvailable := deriveSkillLifecycleStatus(hydrated)
		_, enabled := enabledSet[name]
		out = append(out, RuntimeSkillCapability{
			Name:            hydrated.Name,
			Summary:         hydrated.Summary,
			Keywords:        append([]string(nil), hydrated.Keywords...),
			Tags:            append([]string(nil), hydrated.Tags...),
			Source:          hydrated.Source,
			Version:         hydrated.Version,
			TargetVersion:   hydrated.TargetVersion,
			Health:          health,
			UpdateStatus:    updateStatus,
			UpdateAvailable: updateAvailable,
			Enabled:         enabled,
		})
	}
	return out
}

func (r *SkillsRegistry) SetSkillEnabled(ctx context.Context, name string, enabled bool) error {
	if r == nil {
		return fmt.Errorf("skills registry is unavailable")
	}
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return fmt.Errorf("skill name is required")
	}
	if _, ok := r.catalog[name]; !ok {
		return fmt.Errorf("skill %q is not available", name)
	}
	installedNames, err := r.store.ListInstalledSkillNames(ctx)
	if err != nil {
		return err
	}
	installedSet := map[string]struct{}{}
	for _, installed := range installedNames {
		installedSet[installed] = struct{}{}
	}
	if _, ok := installedSet[name]; !ok {
		return fmt.Errorf("skill %q is not installed", name)
	}

	enabledNames, err := r.store.ListEnabledSkillNames(ctx)
	if err != nil {
		return err
	}
	if enabled {
		enabledNames = append(enabledNames, name)
		return r.store.SetEnabledSkillNames(ctx, enabledNames)
	}
	filtered := make([]string, 0, len(enabledNames))
	for _, enabledName := range enabledNames {
		if enabledName == name {
			continue
		}
		filtered = append(filtered, enabledName)
	}
	return r.store.SetEnabledSkillNames(ctx, filtered)
}

func (r *SkillsRegistry) RelevantSkillsSummary(ctx context.Context, message string) string {
	message = strings.TrimSpace(message)
	if r == nil || message == "" {
		return ""
	}
	installed := r.ListRuntimeSkillCapabilities(ctx)
	if len(installed) == 0 {
		return ""
	}

	type scoredSkill struct {
		skill RuntimeSkillCapability
		score int
	}
	scored := make([]scoredSkill, 0, len(installed))
	for _, skill := range installed {
		if !skill.Enabled {
			continue
		}
		score := scoreSkillMatch(SkillDefinition{
			Name:     skill.Name,
			Summary:  skill.Summary,
			Keywords: skill.Keywords,
			Tags:     skill.Tags,
			Source:   skill.Source,
			Version:  skill.Version,
		}, message)
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
	skill.Source = strings.TrimSpace(skill.Source)
	skill.Version = strings.TrimSpace(skill.Version)
	skill.TargetVersion = strings.TrimSpace(skill.TargetVersion)
	skill.Health = strings.TrimSpace(strings.ToLower(skill.Health))
	skill.UpdateStatus = strings.TrimSpace(strings.ToLower(skill.UpdateStatus))
	if skill.Source == "" {
		skill.Source = "catalog"
	}
	if skill.Version == "" {
		skill.Version = "builtin"
	}
	if skill.Health == "" || skill.UpdateStatus == "" {
		health, updateStatus, updateAvailable := deriveSkillLifecycleStatus(skill)
		if skill.Health == "" {
			skill.Health = health
		}
		if skill.UpdateStatus == "" {
			skill.UpdateStatus = updateStatus
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
		Version:         skill.Version,
		TargetVersion:   skill.TargetVersion,
		Health:          skill.Health,
		UpdateStatus:    skill.UpdateStatus,
		UpdateAvailable: skill.UpdateAvailable,
	}
}

func hydrateSkillDefinitionTargetVersion(skill SkillDefinition, versionPins map[string]string) SkillDefinition {
	cloned := cloneSkillDefinition(skill)
	if len(versionPins) != 0 {
		cloned.TargetVersion = strings.TrimSpace(versionPins[strings.TrimSpace(strings.ToLower(skill.Name))])
	}
	cloned.Health, cloned.UpdateStatus, cloned.UpdateAvailable = deriveSkillLifecycleStatus(cloned)
	return cloned
}

func deriveSkillLifecycleStatus(skill SkillDefinition) (health string, updateStatus string, updateAvailable bool) {
	currentVersion := strings.TrimSpace(skill.Version)
	targetVersion := strings.TrimSpace(skill.TargetVersion)
	switch {
	case targetVersion == "":
		return "healthy", "current", false
	case currentVersion == "":
		return "degraded", "unknown_current_version", true
	case targetVersion == currentVersion:
		return "healthy", "pinned_current", false
	default:
		return "degraded", "update_available", true
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
