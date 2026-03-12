package baseagent

import (
	"context"
	"testing"
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
