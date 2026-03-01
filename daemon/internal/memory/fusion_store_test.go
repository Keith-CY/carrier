package memory

import (
	"archive/zip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFusionSearchHonorsScopePermissions(t *testing.T) {
	store := NewStore(WithRootDir(t.TempDir()))

	if _, err := store.UpsertRecord(UpsertRecordInput{
		Subject:        "agent-a",
		Scope:          Scope("agent:agent-a"),
		ContentSummary: "project alpha decision log",
	}); err != nil {
		t.Fatalf("upsert record: %v", err)
	}

	hitsA := store.Search(SearchOptions{Subject: "agent-a", Query: "alpha"})
	if len(hitsA) == 0 {
		t.Fatalf("expected agent-a to search own scope")
	}
	hitsB := store.Search(SearchOptions{Subject: "agent-b", Query: "alpha"})
	if len(hitsB) != 0 {
		t.Fatalf("expected agent-b to be denied from agent-a scope")
	}
}

func TestFusionGrantAndRevokeSharedScope(t *testing.T) {
	store := NewStore(WithRootDir(t.TempDir()))
	grant, err := store.GrantScope("agent-a", Scope("shared:profile"), "user-1", "share profile")
	if err != nil {
		t.Fatalf("grant scope: %v", err)
	}

	rec, err := store.UpsertRecord(UpsertRecordInput{
		Subject:        "agent-a",
		Scope:          Scope("shared:profile"),
		ContentSummary: "team timezone: JST",
	})
	if err != nil {
		t.Fatalf("upsert shared record: %v", err)
	}
	if rec.Scope != Scope("shared:profile") {
		t.Fatalf("unexpected scope: %s", rec.Scope)
	}

	if got := store.Search(SearchOptions{Subject: "agent-a", Query: "timezone"}); len(got) == 0 {
		t.Fatalf("expected granted subject to search shared scope")
	}
	if got := store.Search(SearchOptions{Subject: "agent-b", Query: "timezone"}); len(got) != 0 {
		t.Fatalf("expected non-granted subject to be denied")
	}

	if err := store.RevokeScope(grant.ID, "user-1"); err != nil {
		t.Fatalf("revoke grant: %v", err)
	}
	if got := store.Search(SearchOptions{Subject: "agent-a", Query: "timezone"}); len(got) != 0 {
		t.Fatalf("expected revoked scope to stop new reads")
	}
}

func TestFusionObserveAutoCurateAndTimeline(t *testing.T) {
	store := NewStore(WithRootDir(t.TempDir()))
	ev, err := store.Observe(ObserveInput{
		Subject:       "agent-a",
		AgentID:       "agent-a",
		Scope:         Scope("agent:agent-a"),
		ToolName:      "web_fetch",
		OutputSnippet: "fetched onboarding checklist",
		Status:        "ok",
		AutoCurate:    true,
	})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	timeline := store.Timeline("agent-a", ev.ID, 2)
	if len(timeline) == 0 {
		t.Fatalf("expected timeline around observation")
	}
	hits := store.Search(SearchOptions{Subject: "agent-a", Query: "onboarding"})
	if len(hits) == 0 {
		t.Fatalf("expected auto-curated record to be searchable")
	}
}

func TestFusionInstanceImportExport(t *testing.T) {
	root := t.TempDir()
	store := NewStore(WithRootDir(root))

	src := filepath.Join(t.TempDir(), "notes.md")
	if err := os.WriteFile(src, []byte("# Notes\nLine one"), 0o644); err != nil {
		t.Fatalf("write import source: %v", err)
	}
	id, err := store.ImportForInstance("agent-a", src, InstanceImportOptions{
		Actor:       "tester",
		RequestID:   "req-1",
		TargetScope: Scope("agent:agent-a"),
	})
	if err != nil {
		t.Fatalf("import for instance: %v", err)
	}
	if strings.TrimSpace(id) == "" {
		t.Fatalf("expected imported id")
	}

	ref, err := store.ExportForInstance("agent-a", InstanceExportOptions{
		Actor:     "tester",
		RequestID: "req-2",
		Format:    "truth-only",
	})
	if err != nil {
		t.Fatalf("export for instance: %v", err)
	}
	if _, err := os.Stat(ref); err != nil {
		t.Fatalf("expected export artifact at %s: %v", ref, err)
	}

	zr, err := zip.OpenReader(ref)
	if err != nil {
		t.Fatalf("open export zip: %v", err)
	}
	defer zr.Close()
	found := false
	for _, f := range zr.File {
		if strings.Contains(f.Name, "truth/agent/agent-a") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected exported zip to contain agent truth files")
	}
}

func TestFusionSearchUsesSQLiteIndexCache(t *testing.T) {
	root := t.TempDir()
	store := NewStore(WithRootDir(root))
	if _, err := store.UpsertRecord(UpsertRecordInput{
		Subject:        "agent-a",
		Scope:          Scope("agent:agent-a"),
		ContentSummary: "sqlite indexed content",
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	indexPath := filepath.Join(root, "index", "mem_index.sqlite")
	if _, err := os.Stat(indexPath); err != nil {
		t.Fatalf("expected sqlite index at %s: %v", indexPath, err)
	}

	// Force fallback memory map empty to ensure search can still use SQLite cache.
	store.mu.Lock()
	store.records = map[string]MemoryRecord{}
	store.mu.Unlock()

	hits := store.Search(SearchOptions{Subject: "agent-a", Query: "sqlite"})
	if len(hits) == 0 {
		t.Fatalf("expected sqlite-backed search hits")
	}
}

func TestFusionSearchReranksWithHybridWeights(t *testing.T) {
	store := NewStore()
	phrase, err := store.UpsertRecord(UpsertRecordInput{
		Subject:        "agent-a",
		Scope:          Scope("agent:agent-a"),
		ContentSummary: "alpha rollout checklist includes integration smoke tests",
	})
	if err != nil {
		t.Fatalf("upsert phrase record: %v", err)
	}
	_, err = store.UpsertRecord(UpsertRecordInput{
		Subject:        "agent-a",
		Scope:          Scope("agent:agent-a"),
		ContentSummary: "alpha rollout",
	})
	if err != nil {
		t.Fatalf("upsert topic record: %v", err)
	}

	rerank := true
	adaptive := true
	lex := 0.2
	sem := 0.8
	hits := store.Search(SearchOptions{
		Subject:        "agent-a",
		Query:          "alpha rollout checklist",
		AdaptiveRecall: &adaptive,
		Rerank:         &rerank,
		LexicalWeight:  &lex,
		SemanticWeight: &sem,
	})
	if len(hits) == 0 {
		t.Fatal("expected hybrid rerank hits")
	}
	if hits[0].ID != phrase.ID {
		t.Fatalf("expected best hit %s, got %s", phrase.ID, hits[0].ID)
	}
}

func TestFusionSearchEmptyResultsWithRerank(t *testing.T) {
	store := NewStore()
	_, err := store.UpsertRecord(UpsertRecordInput{
		Subject:        "agent-a",
		Scope:          Scope("agent:agent-a"),
		ContentSummary: "unrelated content about databases",
	})
	if err != nil {
		t.Fatalf("upsert record: %v", err)
	}

	rerank := true
	minScore := 0.9 // High threshold should filter everything
	hits := store.Search(SearchOptions{
		Subject:  "agent-a",
		Query:    "quantum physics research",
		Rerank:   &rerank,
		MinScore: minScore,
	})
	if len(hits) != 0 {
		t.Fatalf("expected empty results with high minScore, got %d hits", len(hits))
	}
}

func TestFusionSearchZeroWeightsFallbackToDefaults(t *testing.T) {
	store := NewStore()
	_, err := store.UpsertRecord(UpsertRecordInput{
		Subject:        "agent-a",
		Scope:          Scope("agent:agent-a"),
		ContentSummary: "deployment checklist for production",
	})
	if err != nil {
		t.Fatalf("upsert record: %v", err)
	}

	rerank := true
	zeroLex := 0.0
	zeroSem := 0.0
	hits := store.Search(SearchOptions{
		Subject:        "agent-a",
		Query:          "deployment",
		Rerank:         &rerank,
		LexicalWeight:  &zeroLex,
		SemanticWeight: &zeroSem,
	})
	// Should fallback to defaults and still return results
	if len(hits) == 0 {
		t.Fatal("expected results with zero weights (should fallback to defaults)")
	}
}

func TestFusionSearchLargeCandidateMultiplier(t *testing.T) {
	store := NewStore()
	// Create multiple records
	for i := 0; i < 20; i++ {
		_, err := store.UpsertRecord(UpsertRecordInput{
			Subject:        "agent-a",
			Scope:          Scope("agent:agent-a"),
			ContentSummary: fmt.Sprintf("test document number %d about search functionality", i),
		})
		if err != nil {
			t.Fatalf("upsert record %d: %v", i, err)
		}
	}

	rerank := true
	hits := store.Search(SearchOptions{
		Subject:             "agent-a",
		Query:               "search functionality",
		Rerank:              &rerank,
		CandidateMultiplier: 20, // Max multiplier
		MaxResults:          5,
	})
	if len(hits) == 0 {
		t.Fatal("expected results with large candidate multiplier")
	}
	if len(hits) > 5 {
		t.Fatalf("expected max 5 results, got %d", len(hits))
	}
}
func TestFusionRecordReadArchiveAndAccessControl(t *testing.T) {
	store := NewStore(WithRootDir(t.TempDir()))

	rec, err := store.UpsertRecord(UpsertRecordInput{
		Subject:        "agent-a",
		Scope:          Scope("agent:agent-a"),
		Type:           RecordTypeDecision,
		ContentRaw:     "raw details",
		ContentSummary: "decision summary",
	})
	if err != nil {
		t.Fatalf("upsert record: %v", err)
	}

	got, err := store.GetRecord("agent-a", rec.ID)
	if err != nil {
		t.Fatalf("GetRecord(agent-a): %v", err)
	}
	if got.ID != rec.ID || got.Type != RecordTypeDecision {
		t.Fatalf("unexpected record read: %+v", got)
	}

	if _, err := store.GetRecord("agent-b", rec.ID); err != ErrMountDenied {
		t.Fatalf("expected ErrMountDenied for unauthorized read, got %v", err)
	}

	if err := store.ArchiveRecord("agent-b", rec.ID); err != ErrMountDenied {
		t.Fatalf("expected ErrMountDenied for unauthorized archive, got %v", err)
	}
	if err := store.ArchiveRecord("agent-a", rec.ID); err != nil {
		t.Fatalf("ArchiveRecord(agent-a): %v", err)
	}
	if err := store.ArchiveRecord("agent-a", "missing-id"); err == nil {
		t.Fatal("expected missing record archive error")
	}

	if hits := store.Search(SearchOptions{Subject: "agent-a", Query: "decision"}); len(hits) != 0 {
		t.Fatalf("expected archived record to be filtered from search, got %d hits", len(hits))
	}
}

func TestFusionGrantAndInstanceScopeOperations(t *testing.T) {
	store := NewStore(WithRootDir(t.TempDir()))

	if _, err := store.GrantScope("", Scope("shared:team"), "owner", "missing subject"); err == nil {
		t.Fatal("expected grant validation error for empty subject")
	}

	grant, err := store.GrantScope("agent-a", Scope("shared:team"), "owner", "share team memory")
	if err != nil {
		t.Fatalf("GrantScope: %v", err)
	}

	grants := store.ListGrants("agent-a")
	if len(grants) != 1 || grants[0].ID != grant.ID {
		t.Fatalf("unexpected grants list: %+v", grants)
	}
	if all := store.ListGrants(""); len(all) == 0 {
		t.Fatal("expected non-empty global grants list")
	}

	if err := store.RevokeScope("missing-grant", "owner"); err != ErrMemoryNotFound {
		t.Fatalf("expected ErrMemoryNotFound for missing grant revoke, got %v", err)
	}
	if err := store.RevokeScope(grant.ID, "owner"); err != nil {
		t.Fatalf("RevokeScope: %v", err)
	}

	if err := store.AttachScope("", Scope("shared:team")); err == nil {
		t.Fatal("expected attach validation error")
	}
	if err := store.AttachScope("inst-1", Scope("shared:zeta")); err != nil {
		t.Fatalf("AttachScope zeta: %v", err)
	}
	if err := store.AttachScope("inst-1", Scope("shared:alpha")); err != nil {
		t.Fatalf("AttachScope alpha: %v", err)
	}
	if err := store.AttachScope("inst-1", Scope("shared:alpha")); err != nil {
		t.Fatalf("AttachScope duplicate should be no-op: %v", err)
	}

	scopes := store.InstanceScopes("inst-1")
	if len(scopes) != 2 {
		t.Fatalf("InstanceScopes length = %d, want 2", len(scopes))
	}
	if scopes[0] != Scope("shared:alpha") || scopes[1] != Scope("shared:zeta") {
		t.Fatalf("InstanceScopes should be sorted, got %+v", scopes)
	}

	if err := store.DetachScope("inst-1", Scope("shared:missing")); err != ErrAttachmentMissing {
		t.Fatalf("expected ErrAttachmentMissing for missing scope, got %v", err)
	}
	if err := store.DetachScope("inst-1", Scope("shared:alpha")); err != nil {
		t.Fatalf("DetachScope alpha: %v", err)
	}
	remaining := store.InstanceScopes("inst-1")
	if len(remaining) != 1 || remaining[0] != Scope("shared:zeta") {
		t.Fatalf("unexpected remaining scopes: %+v", remaining)
	}
}

func TestFusionInstanceDistillDryRunAndApply(t *testing.T) {
	root := t.TempDir()
	fixed := time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC)
	store := NewStore(
		WithRootDir(root),
		WithNow(func() time.Time { return fixed }),
	)

	if _, err := store.UpsertRecord(UpsertRecordInput{
		ID:             "dup-1",
		Subject:        "agent-a",
		Scope:          Scope("agent:agent-a"),
		ContentSummary: "deployment region is tokyo",
	}); err != nil {
		t.Fatalf("upsert first record: %v", err)
	}
	if _, err := store.UpsertRecord(UpsertRecordInput{
		ID:             "dup-2",
		Subject:        "agent-a",
		Scope:          Scope("agent:agent-a"),
		ContentSummary: "deployment region is tokyo",
	}); err != nil {
		t.Fatalf("upsert duplicate record: %v", err)
	}

	dryRun, err := store.DistillForInstance(context.Background(), InstanceDistillOptions{
		InstanceID: "agent-a",
		DryRun:     true,
		Force:      true,
	})
	if err != nil {
		t.Fatalf("dry-run distill: %v", err)
	}
	if strings.TrimSpace(dryRun.RunID) == "" {
		t.Fatal("expected dry-run distill run id")
	}
	if dryRun.Status != "planned" {
		t.Fatalf("expected planned status, got %s", dryRun.Status)
	}

	run, err := store.DistillForInstance(context.Background(), InstanceDistillOptions{
		InstanceID: "agent-a",
		Force:      true,
	})
	if err != nil {
		t.Fatalf("apply distill: %v", err)
	}
	if run.Status != "completed" {
		t.Fatalf("expected completed status, got %s", run.Status)
	}
	if run.Removed < 1 {
		t.Fatalf("expected at least one removed source record, got %d", run.Removed)
	}
}
