package memory

import (
	"math"
	"os"
	"testing"
)

func TestFusionMigrationBackupValidateRollback(t *testing.T) {
	root := t.TempDir()
	store := NewStore(WithRootDir(root))

	rec, err := store.UpsertRecord(UpsertRecordInput{
		Subject:        "agent-a",
		Scope:          Scope("agent:agent-a"),
		ContentSummary: "critical migration note",
	})
	if err != nil {
		t.Fatalf("upsert record: %v", err)
	}
	if _, err := store.Observe(ObserveInput{
		Subject:       "agent-a",
		AgentID:       "agent-a",
		Scope:         Scope("agent:agent-a"),
		ToolName:      "tool.call",
		OutputSnippet: "observed useful output",
		Status:        "ok",
	}); err != nil {
		t.Fatalf("observe: %v", err)
	}
	if _, err := store.GrantScope("agent-a", Scope("shared:team"), "tester", "seed grant"); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := store.AttachScope("agent-a", Scope("shared:team")); err != nil {
		t.Fatalf("attach scope: %v", err)
	}

	backupPath, err := store.CreateMigrationBackup("tester", "req-backup")
	if err != nil {
		t.Fatalf("CreateMigrationBackup: %v", err)
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup artifact missing: %v", err)
	}

	validation := store.ValidateMigration()
	if !validation.Consistent {
		t.Fatalf("validation should be consistent before rollback: %+v", validation)
	}

	store.mu.Lock()
	store.records = map[string]MemoryRecord{}
	store.observations = nil
	store.grants = map[string]Grant{}
	store.instanceScopes = map[string][]Scope{}
	if err := store.persistStateLocked(); err != nil {
		store.mu.Unlock()
		t.Fatalf("persist modified state: %v", err)
	}
	store.mu.Unlock()

	if err := store.RollbackFromBackup(backupPath, "tester", "req-rollback"); err != nil {
		t.Fatalf("RollbackFromBackup: %v", err)
	}

	if _, err := store.GetRecord("agent-a", rec.ID); err != nil {
		t.Fatalf("GetRecord after rollback: %v", err)
	}
	if grants := store.ListGrants("agent-a"); len(grants) == 0 {
		t.Fatal("expected grants restored after rollback")
	}
	if scopes := store.InstanceScopes("agent-a"); len(scopes) == 0 {
		t.Fatal("expected instance scopes restored after rollback")
	}

	validation = store.ValidateMigration()
	if !validation.Consistent {
		t.Fatalf("validation should be consistent after rollback: %+v", validation)
	}
}

func TestFusionSQLiteRebuildPersistsObservationAndGrant(t *testing.T) {
	store := NewStore(WithRootDir(t.TempDir()))
	if _, err := store.UpsertRecord(UpsertRecordInput{
		Subject:        "agent-a",
		Scope:          Scope("agent:agent-a"),
		ContentSummary: "sqlite indexed record",
	}); err != nil {
		t.Fatalf("upsert record: %v", err)
	}
	if _, err := store.Observe(ObserveInput{
		Subject:       "agent-a",
		AgentID:       "agent-a",
		Scope:         Scope("agent:agent-a"),
		ToolName:      "tool.sqlite",
		OutputSnippet: "sqlite observation",
		Status:        "ok",
	}); err != nil {
		t.Fatalf("observe: %v", err)
	}
	if _, err := store.GrantScope("agent-a", Scope("shared:profile"), "tester", "profile sharing"); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := store.AttachScope("agent-a", Scope("shared:profile")); err != nil {
		t.Fatalf("attach scope: %v", err)
	}

	store.mu.Lock()
	store.rebuildSQLiteIndexLocked()
	store.mu.Unlock()

	store.mu.RLock()
	db, err := store.openSQLiteLocked()
	store.mu.RUnlock()
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	var recordsCount int
	if err := db.QueryRow(`SELECT COUNT(1) FROM memory_records WHERE archived_at IS NULL`).Scan(&recordsCount); err != nil {
		t.Fatalf("query records count: %v", err)
	}
	if recordsCount == 0 {
		t.Fatal("expected memory_records rows in sqlite index")
	}

	var observationCount int
	if err := db.QueryRow(`SELECT COUNT(1) FROM observation_events`).Scan(&observationCount); err != nil {
		t.Fatalf("query observation count: %v", err)
	}
	if observationCount == 0 {
		t.Fatal("expected observation_events rows in sqlite index")
	}

	var grantCount int
	if err := db.QueryRow(`SELECT COUNT(1) FROM grants`).Scan(&grantCount); err != nil {
		t.Fatalf("query grant count: %v", err)
	}
	if grantCount == 0 {
		t.Fatal("expected grant rows in sqlite index")
	}
}

func TestFusionFallbackSearchUsesInMemoryScore(t *testing.T) {
	store := NewStore()
	if _, err := store.UpsertRecord(UpsertRecordInput{
		Subject:        "agent-a",
		Scope:          Scope("agent:agent-a"),
		ContentSummary: "project alpha launch checklist",
	}); err != nil {
		t.Fatalf("upsert record: %v", err)
	}

	hits := store.Search(SearchOptions{Subject: "agent-a", Query: "alpha checklist", MinScore: 0.5})
	if len(hits) == 0 {
		t.Fatal("expected in-memory search hits")
	}
	if misses := store.Search(SearchOptions{Subject: "agent-a", Query: "nonexistent"}); len(misses) != 0 {
		t.Fatalf("expected no hits for unmatched query, got %d", len(misses))
	}
}

func TestBuildFTSQueryAndRankToScore(t *testing.T) {
	if got := buildFTSQuery("alpha beta"); got != "alpha AND beta" {
		t.Fatalf("buildFTSQuery simple = %q, want %q", got, "alpha AND beta")
	}
	if got := buildFTSQuery(`alpha "beta`); got == "" {
		t.Fatal("buildFTSQuery should keep non-empty sanitized query")
	}
	if got := buildFTSQuery("   "); got != "" {
		t.Fatalf("buildFTSQuery blank = %q, want empty", got)
	}

	if score := rankToScore(math.NaN()); score != 0 {
		t.Fatalf("rankToScore(NaN) = %v, want 0", score)
	}
	if score := rankToScore(-2); score != 1 {
		t.Fatalf("rankToScore(-2) = %v, want 1", score)
	}
	if score := rankToScore(3); score <= 0 || score >= 1 {
		t.Fatalf("rankToScore(3) out of expected range: %v", score)
	}
}
