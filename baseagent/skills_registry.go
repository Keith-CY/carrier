package baseagent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

type RuntimeSkillCapability struct {
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
	Enabled         bool     `json:"enabled"`
}

var skillsRegistryNow = time.Now

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
	metadataByName, err := r.store.ListSkillMetadata(ctx)
	if err != nil {
		return nil
	}
	out := make([]SkillDefinition, 0, len(names))
	for _, name := range names {
		skill, ok := r.catalog[name]
		if !ok {
			continue
		}
		out = append(out, hydrateSkillDefinition(skill, versionPins, metadataByName, true))
	}
	return out
}

func (r *SkillsRegistry) SearchSkills(ctx context.Context, query string) []SkillDefinition {
	if r == nil {
		return nil
	}
	query = strings.TrimSpace(query)
	versionPins, _ := r.store.ListSkillVersionPins(ctx)
	metadataByName, _ := r.store.ListSkillMetadata(ctx)
	if query == "" {
		out := make([]SkillDefinition, 0, len(r.catalog))
		for _, skill := range r.catalog {
			out = append(out, hydrateSkillDefinition(skill, versionPins, metadataByName, false))
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
		out = append(out, hydrateSkillDefinition(item.skill, versionPins, metadataByName, false))
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
	metadataByName, err := r.store.ListSkillMetadata(ctx)
	if err != nil {
		return SkillDefinition{}, err
	}
	if metadataByName == nil {
		metadataByName = map[string]SkillLifecycleMetadata{}
	}
	meta := metadataByName[name]
	now := skillsRegistryNow().UTC().Format(time.RFC3339)
	if strings.TrimSpace(meta.InstalledAt) == "" {
		meta.InstalledAt = now
	}
	meta.Provenance = appendSkillProvenanceHistory(meta.Provenance, buildSkillProvenanceSummary("install", skill))
	metadataByName[name] = normalizeSkillLifecycleMetadata(meta)
	if err := r.store.SetSkillMetadata(ctx, metadataByName); err != nil {
		return SkillDefinition{}, err
	}
	return hydrateSkillDefinition(skill, nil, metadataByName, true), nil
}

func (r *SkillsRegistry) ReinstallSkill(ctx context.Context, name string) (SkillDefinition, error) {
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
	metadataByName, err := r.store.ListSkillMetadata(ctx)
	if err != nil {
		return SkillDefinition{}, err
	}
	if metadataByName == nil {
		metadataByName = map[string]SkillLifecycleMetadata{}
	}
	meta := metadataByName[name]
	now := skillsRegistryNow().UTC().Format(time.RFC3339)
	if strings.TrimSpace(meta.InstalledAt) == "" {
		meta.InstalledAt = now
	}
	meta.UpdatedAt = now
	meta.Provenance = appendSkillProvenanceHistory(meta.Provenance, buildSkillProvenanceSummary("reinstall", skill))
	metadataByName[name] = normalizeSkillLifecycleMetadata(meta)
	if err := r.store.SetSkillMetadata(ctx, metadataByName); err != nil {
		return SkillDefinition{}, err
	}
	versionPins, err := r.store.ListSkillVersionPins(ctx)
	if err != nil {
		return SkillDefinition{}, err
	}
	return hydrateSkillDefinition(skill, versionPins, metadataByName, true), nil
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
	metadataByName, err := r.store.ListSkillMetadata(ctx)
	if err != nil {
		return SkillDefinition{}, err
	}
	if metadataByName == nil {
		metadataByName = map[string]SkillLifecycleMetadata{}
	}
	meta := metadataByName[name]
	now := skillsRegistryNow().UTC().Format(time.RFC3339)
	if strings.TrimSpace(meta.InstalledAt) == "" {
		meta.InstalledAt = now
	}
	meta.UpdatedAt = now
	meta.Provenance = appendSkillProvenanceHistory(meta.Provenance, buildSkillProvenanceSummary("update", skill))
	metadataByName[name] = normalizeSkillLifecycleMetadata(meta)
	if err := r.store.SetSkillMetadata(ctx, metadataByName); err != nil {
		return SkillDefinition{}, err
	}
	return hydrateSkillDefinition(skill, versionPins, metadataByName, true), nil
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
	metadataByName, err := r.store.ListSkillMetadata(ctx)
	if err != nil {
		return SkillDefinition{}, err
	}
	delete(metadataByName, name)
	if err := r.store.SetSkillMetadata(ctx, metadataByName); err != nil {
		return SkillDefinition{}, err
	}
	return hydrateSkillDefinition(skill, versionPins, metadataByName, true), nil
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
	metadataByName, err := r.store.ListSkillMetadata(ctx)
	if err != nil {
		return nil
	}
	out := make([]RuntimeSkillCapability, 0, len(names))
	for _, name := range names {
		skill, ok := r.catalog[name]
		if !ok {
			continue
		}
		hydrated := hydrateSkillDefinition(skill, versionPins, metadataByName, true)
		_, enabled := enabledSet[name]
		if !enabled {
			hydrated = applyDisabledSkillHints(hydrated)
		}
		out = append(out, RuntimeSkillCapability{
			Name:            hydrated.Name,
			Summary:         hydrated.Summary,
			Keywords:        append([]string(nil), hydrated.Keywords...),
			Tags:            append([]string(nil), hydrated.Tags...),
			Source:          hydrated.Source,
			Provenance:      hydrated.Provenance,
			Version:         hydrated.Version,
			TargetVersion:   hydrated.TargetVersion,
			InstalledAt:     hydrated.InstalledAt,
			UpdatedAt:       hydrated.UpdatedAt,
			Health:          hydrated.Health,
			HealthDetail:    hydrated.HealthDetail,
			RemediationHint: hydrated.RemediationHint,
			UpdateStatus:    hydrated.UpdateStatus,
			UpdateAvailable: hydrated.UpdateAvailable,
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
