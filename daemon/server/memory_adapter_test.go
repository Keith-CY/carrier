package server

import (
	"strings"
	"testing"

	"carrier/baseagent"
	"carrier/daemon/internal/memory"
)

func TestBaseAgentMemoryStoreAdapterFusionMethods(t *testing.T) {
	store := memory.NewStore(memory.WithRootDir(t.TempDir()))
	adapter := &baseAgentMemoryStoreAdapter{store: store}

	if err := adapter.Create("mem-1", "Adapter Memory", "v1", baseagent.MemoryTypePerAgent, "agent-a"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := adapter.Archive("mem-1"); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	grantID, err := adapter.Grant("agent-a", "shared:team", "tester", "seed shared scope")
	if err != nil {
		t.Fatalf("Grant: %v", err)
	}
	if strings.TrimSpace(grantID) == "" {
		t.Fatal("Grant returned empty id")
	}

	rec, err := store.UpsertRecord(memory.UpsertRecordInput{
		Subject:        "agent-a",
		Scope:          memory.Scope("shared:team"),
		ContentSummary: "team handbook entry",
		Provenance:     "adapter-test",
	})
	if err != nil {
		t.Fatalf("UpsertRecord seed: %v", err)
	}

	hits, err := adapter.Search("agent-a", "handbook", 10, 0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("Search should return at least one hit")
	}

	got, err := adapter.GetRecord("agent-a", rec.ID)
	if err != nil {
		t.Fatalf("GetRecord: %v", err)
	}
	if got.ID != rec.ID {
		t.Fatalf("GetRecord ID = %q, want %q", got.ID, rec.ID)
	}

	obsID, err := adapter.Observe("agent-a", "tool.adapter", "captured output", "agent:agent-a")
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if strings.TrimSpace(obsID) == "" {
		t.Fatal("Observe returned empty id")
	}

	if err := adapter.Revoke(grantID, "tester"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	if _, err := store.CreateMigrationBackup("tester", "req-audit"); err != nil {
		t.Fatalf("CreateMigrationBackup for audit seeding: %v", err)
	}
	if audits := adapter.ListAudits(); len(audits) == 0 {
		t.Fatal("ListAudits should return non-empty list")
	}

	if _, err := adapter.ExportMemory("missing-memory", baseagent.ExportOptions{Actor: "tester", RequestID: "req-export"}); err == nil {
		t.Fatal("ExportMemory should fail for missing memory")
	}
}
