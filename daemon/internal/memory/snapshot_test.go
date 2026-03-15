package memory

import (
	"context"
	"testing"
	"time"
)

func TestCreateSnapshotForInstanceScopesProducesFrozenReadonlyMount(t *testing.T) {
	fixed := time.Date(2026, 3, 16, 9, 30, 0, 0, time.UTC)
	store := NewStore(
		WithRootDir(t.TempDir()),
		WithNow(func() time.Time { return fixed }),
	)

	if _, err := store.GrantScope("parent", Scope("shared:team"), "owner", "delegate shared team memory"); err != nil {
		t.Fatalf("GrantScope(shared:team): %v", err)
	}
	if _, err := store.UpsertRecord(UpsertRecordInput{
		ID:             "shared-team-1",
		Subject:        "parent",
		Scope:          Scope("shared:team"),
		Type:           RecordTypeFact,
		ContentSummary: "team timezone is tokyo",
		Provenance:     "seed:shared-team-1",
	}); err != nil {
		t.Fatalf("UpsertRecord(shared:team): %v", err)
	}
	if _, err := store.UpsertRecord(UpsertRecordInput{
		ID:             "parent-private-1",
		Subject:        "parent",
		Scope:          Scope("agent:parent"),
		Type:           RecordTypeNote,
		ContentSummary: "private parent tokyo note",
		Provenance:     "seed:parent-private-1",
	}); err != nil {
		t.Fatalf("UpsertRecord(agent:parent): %v", err)
	}

	snapshot, err := store.CreateSnapshotForInstance(context.Background(), SnapshotOptions{
		Actor:            "tester",
		RequestID:        "req-snapshot-1",
		SourceSubject:    "parent",
		SourceScopes:     []Scope{Scope("shared:team")},
		TargetInstanceID: "child",
		Reason:           "delegate task",
	})
	if err != nil {
		t.Fatalf("CreateSnapshotForInstance: %v", err)
	}
	if snapshot.ID == "" {
		t.Fatal("expected snapshot id")
	}
	if snapshot.Digest == "" {
		t.Fatal("expected snapshot digest")
	}
	if snapshot.SourceSubject != "parent" {
		t.Fatalf("SourceSubject = %q, want parent", snapshot.SourceSubject)
	}
	if snapshot.TargetInstanceID != "child" {
		t.Fatalf("TargetInstanceID = %q, want child", snapshot.TargetInstanceID)
	}
	if len(snapshot.SourceScopes) != 1 || snapshot.SourceScopes[0] != Scope("shared:team") {
		t.Fatalf("unexpected SourceScopes: %+v", snapshot.SourceScopes)
	}
	if len(snapshot.SourceRecordIDs) != 1 || snapshot.SourceRecordIDs[0] != "shared-team-1" {
		t.Fatalf("unexpected SourceRecordIDs: %+v", snapshot.SourceRecordIDs)
	}
	if !snapshot.CreatedAt.Equal(fixed) {
		t.Fatalf("CreatedAt = %s, want %s", snapshot.CreatedAt, fixed)
	}

	if _, err := store.UpsertRecord(UpsertRecordInput{
		ID:             "shared-team-1",
		Subject:        "parent",
		Scope:          Scope("shared:team"),
		Type:           RecordTypeFact,
		ContentSummary: "team timezone is osaka",
		Provenance:     "seed:shared-team-1",
	}); err != nil {
		t.Fatalf("mutate live shared record: %v", err)
	}

	if err := store.MountSnapshot("child", snapshot.ID); err != nil {
		t.Fatalf("MountSnapshot: %v", err)
	}

	snapshotScope := sharedSnapshotScope(snapshot.ID)
	scopes := store.InstanceScopes("child")
	if len(scopes) != 1 || scopes[0] != snapshotScope {
		t.Fatalf("InstanceScopes(child) = %+v, want [%s]", scopes, snapshotScope)
	}

	frozenHits := store.Search(SearchOptions{Subject: "child", Query: "tokyo"})
	if len(frozenHits) == 0 {
		t.Fatal("expected mounted child snapshot search hits for frozen content")
	}
	if frozenHits[0].Scope != snapshotScope {
		t.Fatalf("snapshot hit scope = %s, want %s", frozenHits[0].Scope, snapshotScope)
	}

	if liveHits := store.Search(SearchOptions{Subject: "child", Query: "osaka"}); len(liveHits) != 0 {
		t.Fatalf("expected child snapshot search to stay frozen, got %+v", liveHits)
	}
	if parentHits := store.Search(SearchOptions{Subject: "parent", Query: "osaka"}); len(parentHits) == 0 {
		t.Fatal("expected source subject to keep seeing live shared updates")
	}
	if _, err := store.GrantScope("observer", Scope("shared:*"), "owner", "broad shared access"); err != nil {
		t.Fatalf("GrantScope(shared:*): %v", err)
	}
	if observerHits := store.Search(SearchOptions{Subject: "observer", Query: "tokyo"}); len(observerHits) != 0 {
		t.Fatalf("expected wildcard shared grant to exclude snapshot reads, got %+v", observerHits)
	}
}

func TestSnapshotMountRejectsWrites(t *testing.T) {
	store := NewStore(WithRootDir(t.TempDir()))

	if _, err := store.GrantScope("parent", Scope("shared:team"), "owner", "delegate shared team memory"); err != nil {
		t.Fatalf("GrantScope(shared:team): %v", err)
	}
	if _, err := store.UpsertRecord(UpsertRecordInput{
		ID:             "shared-team-1",
		Subject:        "parent",
		Scope:          Scope("shared:team"),
		Type:           RecordTypeFact,
		ContentSummary: "team timezone is tokyo",
	}); err != nil {
		t.Fatalf("UpsertRecord(shared:team): %v", err)
	}

	snapshot, err := store.CreateSnapshotForInstance(context.Background(), SnapshotOptions{
		Actor:            "tester",
		RequestID:        "req-snapshot-2",
		SourceSubject:    "parent",
		SourceScopes:     []Scope{Scope("shared:team")},
		TargetInstanceID: "child",
		Reason:           "delegate task",
	})
	if err != nil {
		t.Fatalf("CreateSnapshotForInstance: %v", err)
	}
	if err := store.MountSnapshot("child", snapshot.ID); err != nil {
		t.Fatalf("MountSnapshot: %v", err)
	}
	if _, err := store.GrantScope("child", Scope("shared:*"), "owner", "broad shared access"); err != nil {
		t.Fatalf("GrantScope(shared:*): %v", err)
	}

	_, err = store.UpsertRecord(UpsertRecordInput{
		Subject:        "child",
		Scope:          sharedSnapshotScope(snapshot.ID),
		Type:           RecordTypeNote,
		ContentSummary: "attempted child snapshot mutation",
	})
	if err != ErrMountDenied {
		t.Fatalf("expected ErrMountDenied for snapshot write, got %v", err)
	}
}

func TestSnapshotRejectsGrantAttachAndStaleMountBypass(t *testing.T) {
	store := NewStore(WithRootDir(t.TempDir()))

	if _, err := store.GrantScope("parent", Scope("shared:team"), "owner", "delegate shared team memory"); err != nil {
		t.Fatalf("GrantScope(shared:team): %v", err)
	}
	if _, err := store.UpsertRecord(UpsertRecordInput{
		ID:             "shared-team-1",
		Subject:        "parent",
		Scope:          Scope("shared:team"),
		Type:           RecordTypeFact,
		ContentSummary: "team timezone is tokyo",
	}); err != nil {
		t.Fatalf("UpsertRecord(shared:team): %v", err)
	}

	snapshot, err := store.CreateSnapshotForInstance(context.Background(), SnapshotOptions{
		Actor:            "tester",
		RequestID:        "req-snapshot-3",
		SourceSubject:    "parent",
		SourceScopes:     []Scope{Scope("shared:team")},
		TargetInstanceID: "child",
		Reason:           "delegate task",
	})
	if err != nil {
		t.Fatalf("CreateSnapshotForInstance: %v", err)
	}
	snapshotScope := sharedSnapshotScope(snapshot.ID)

	if _, err := store.GrantScope("observer", snapshotScope, "owner", "unauthorized snapshot grant"); err == nil {
		t.Fatal("expected exact snapshot grant to be rejected")
	}
	if err := store.AttachScope("observer", snapshotScope); err == nil {
		t.Fatal("expected generic AttachScope to reject snapshot scopes")
	}

	store.mu.Lock()
	_ = store.addManualScopeLocked("observer", snapshotScope)
	store.rebuildInstanceScopesLocked("observer")
	store.mu.Unlock()

	if hits := store.Search(SearchOptions{Subject: "observer", Query: "tokyo"}); len(hits) != 0 {
		t.Fatalf("expected stale unauthorized mount to be ignored, got %+v", hits)
	}
}

func TestSnapshotClonedRecordIDsCannotBeReusedForMutation(t *testing.T) {
	store := NewStore(WithRootDir(t.TempDir()))

	if _, err := store.GrantScope("parent", Scope("shared:team"), "owner", "delegate shared team memory"); err != nil {
		t.Fatalf("GrantScope(shared:team): %v", err)
	}
	if _, err := store.UpsertRecord(UpsertRecordInput{
		ID:             "shared-team-1",
		Subject:        "parent",
		Scope:          Scope("shared:team"),
		Type:           RecordTypeFact,
		ContentSummary: "team timezone is tokyo",
	}); err != nil {
		t.Fatalf("UpsertRecord(shared:team): %v", err)
	}

	snapshot, err := store.CreateSnapshotForInstance(context.Background(), SnapshotOptions{
		Actor:            "tester",
		RequestID:        "req-snapshot-4",
		SourceSubject:    "parent",
		SourceScopes:     []Scope{Scope("shared:team")},
		TargetInstanceID: "child",
		Reason:           "delegate task",
	})
	if err != nil {
		t.Fatalf("CreateSnapshotForInstance: %v", err)
	}
	if err := store.MountSnapshot("child", snapshot.ID); err != nil {
		t.Fatalf("MountSnapshot: %v", err)
	}

	_, err = store.UpsertRecord(UpsertRecordInput{
		Subject:        "child",
		ID:             snapshot.ClonedRecordIDs[0],
		Scope:          Scope("agent:child"),
		Type:           RecordTypeNote,
		ContentSummary: "stolen snapshot content",
	})
	if err != ErrMountDenied {
		t.Fatalf("expected ErrMountDenied for snapshot clone ID reuse, got %v", err)
	}

	rec, err := store.GetRecord("child", snapshot.ClonedRecordIDs[0])
	if err != nil {
		t.Fatalf("GetRecord(child, snapshot clone): %v", err)
	}
	if rec.Scope != sharedSnapshotScope(snapshot.ID) {
		t.Fatalf("snapshot clone scope = %s, want %s", rec.Scope, sharedSnapshotScope(snapshot.ID))
	}
}
