package baseagent

import (
	"context"
	"strings"
	"testing"
	"time"
)

func testSkillCatalog() []SkillDefinition {
	return []SkillDefinition{
		{
			Name:     "go-testing",
			Summary:  "Use go test before claiming success.",
			Keywords: []string{"go test", "diagnostics", "verify"},
			Tags:     []string{"go", "testing"},
		},
		{
			Name:     "workspace-inspection",
			Summary:  "Prefer bounded workspace reads before edits.",
			Keywords: []string{"inspect workspace", "list_dir", "read file"},
			Tags:     []string{"workspace", "read"},
		},
	}
}

func TestSkillsRegistryList(t *testing.T) {
	store := NewMemorySkillsStore()
	registry := NewSkillsRegistry(store, testSkillCatalog())

	if _, err := registry.InstallSkill(context.Background(), "go-testing"); err != nil {
		t.Fatalf("install go-testing: %v", err)
	}
	if _, err := registry.InstallSkill(context.Background(), "workspace-inspection"); err != nil {
		t.Fatalf("install workspace-inspection: %v", err)
	}

	installed := registry.ListInstalledSkills(context.Background())
	if len(installed) != 2 {
		t.Fatalf("expected 2 installed skills, got %+v", installed)
	}
	if installed[0].Name != "go-testing" || installed[1].Name != "workspace-inspection" {
		t.Fatalf("unexpected installed skills order: %+v", installed)
	}
}

func TestSkillsRegistrySearch(t *testing.T) {
	registry := NewSkillsRegistry(NewMemorySkillsStore(), testSkillCatalog())

	results := registry.SearchSkills(context.Background(), "workspace read")
	if len(results) == 0 {
		t.Fatal("expected search results")
	}
	if results[0].Name != "workspace-inspection" {
		t.Fatalf("expected workspace-inspection to rank first, got %+v", results)
	}
}

func TestSkillsRegistryRuntimeCapabilitiesAndEnablement(t *testing.T) {
	ctx := context.Background()
	registry := NewSkillsRegistry(NewMemorySkillsStore(), testSkillCatalog())

	if _, err := registry.InstallSkill(ctx, "go-testing"); err != nil {
		t.Fatalf("install go-testing: %v", err)
	}
	if _, err := registry.InstallSkill(ctx, "workspace-inspection"); err != nil {
		t.Fatalf("install workspace-inspection: %v", err)
	}

	caps := registry.ListRuntimeSkillCapabilities(ctx)
	if len(caps) != 2 {
		t.Fatalf("expected 2 runtime capabilities, got %+v", caps)
	}
	if !caps[0].Enabled || !caps[1].Enabled {
		t.Fatalf("expected installed skills to default enabled, got %+v", caps)
	}
	if caps[0].Health != "healthy" || caps[0].UpdateStatus != "current" || caps[0].UpdateAvailable {
		t.Fatalf("expected default lifecycle metadata for first skill, got %+v", caps[0])
	}

	if err := registry.SetSkillEnabled(ctx, "go-testing", false); err != nil {
		t.Fatalf("disable go-testing: %v", err)
	}

	disabled := registry.ListRuntimeSkillCapabilities(ctx)
	if len(disabled) != 2 {
		t.Fatalf("expected 2 runtime capabilities after disable, got %+v", disabled)
	}
	if disabled[0].Name != "go-testing" || disabled[0].Enabled {
		t.Fatalf("expected go-testing disabled, got %+v", disabled)
	}
	if !disabled[1].Enabled {
		t.Fatalf("expected workspace-inspection to stay enabled, got %+v", disabled)
	}

	summary := registry.RelevantSkillsSummary(ctx, "run repository diagnostics and verify with go test")
	if strings.Contains(summary, "go-testing") {
		t.Fatalf("disabled skill should not appear in relevant summary, got %q", summary)
	}
}

func TestSkillsRegistryRuntimeCapabilitiesExposePendingUpdateStatus(t *testing.T) {
	ctx := context.Background()
	registry := NewSkillsRegistry(NewMemorySkillsStore(), testSkillCatalog())

	if _, err := registry.InstallSkill(ctx, "workspace-inspection"); err != nil {
		t.Fatalf("install workspace-inspection: %v", err)
	}
	if _, err := registry.UpdateSkill(ctx, "workspace-inspection", "v2.0.0"); err != nil {
		t.Fatalf("update workspace-inspection: %v", err)
	}

	caps := registry.ListRuntimeSkillCapabilities(ctx)
	if len(caps) != 1 {
		t.Fatalf("expected 1 runtime capability, got %+v", caps)
	}
	if caps[0].Health != "degraded" || caps[0].UpdateStatus != "update_available" || !caps[0].UpdateAvailable {
		t.Fatalf("expected pending update lifecycle metadata, got %+v", caps[0])
	}
}

func TestSkillsRegistryRuntimeCapabilitiesExposeProvenanceAndTimestamps(t *testing.T) {
	ctx := context.Background()
	registry := NewSkillsRegistry(NewMemorySkillsStore(), testSkillCatalog())

	restoreNow := skillsRegistryNow
	t.Cleanup(func() { skillsRegistryNow = restoreNow })

	installAt := time.Date(2026, 3, 13, 3, 4, 5, 0, time.UTC)
	updateAt := installAt.Add(2 * time.Hour)
	skillsRegistryNow = func() time.Time { return installAt }

	if _, err := registry.InstallSkill(ctx, "workspace-inspection"); err != nil {
		t.Fatalf("install workspace-inspection: %v", err)
	}

	skillsRegistryNow = func() time.Time { return updateAt }
	if _, err := registry.UpdateSkill(ctx, "workspace-inspection", "v2.0.0"); err != nil {
		t.Fatalf("update workspace-inspection: %v", err)
	}

	caps := registry.ListRuntimeSkillCapabilities(ctx)
	if len(caps) != 1 {
		t.Fatalf("expected 1 runtime capability, got %+v", caps)
	}
	if caps[0].Provenance != "managed update via catalog" {
		t.Fatalf("expected provenance summary, got %+v", caps[0])
	}
	if caps[0].InstalledAt != installAt.Format(time.RFC3339) {
		t.Fatalf("expected install timestamp %q, got %+v", installAt.Format(time.RFC3339), caps[0])
	}
	if caps[0].UpdatedAt != updateAt.Format(time.RFC3339) {
		t.Fatalf("expected update timestamp %q, got %+v", updateAt.Format(time.RFC3339), caps[0])
	}
	if !strings.Contains(caps[0].HealthDetail, "target version") {
		t.Fatalf("expected health detail to explain target version drift, got %+v", caps[0])
	}
	if !strings.Contains(strings.ToLower(caps[0].RemediationHint), "update skill") {
		t.Fatalf("expected remediation hint to mention update action, got %+v", caps[0])
	}
}
