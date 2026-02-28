package memory

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
