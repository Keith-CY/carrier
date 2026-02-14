package memory

import (
	"testing"
	"time"
)

func fixedNow() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }

func newTestStore() *Store {
	return NewStore(WithNow(fixedNow))
}

func TestCreateAndGet(t *testing.T) {
	s := newTestStore()
	e, err := s.Create("m1", "My Memory", "1.0.0", TypePerAgent, "agent-a")
	if err != nil {
		t.Fatal(err)
	}
	if e.State != StateCreated {
		t.Fatalf("expected created, got %s", e.State)
	}

	got, err := s.Get("m1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "m1" {
		t.Fatal("id mismatch")
	}

	// Duplicate create should fail.
	_, err = s.Create("m1", "dup", "1.0.0", TypeShared, "")
	if err == nil {
		t.Fatal("expected error on duplicate create")
	}
}

func TestList(t *testing.T) {
	s := newTestStore()
	_, _ = s.Create("m1", "A", "1.0.0", TypePerAgent, "a")
	_, _ = s.Create("m2", "B", "1.0.0", TypeShared, "")
	if len(s.List()) != 2 {
		t.Fatal("expected 2 entries")
	}
}

func TestMountUnmount(t *testing.T) {
	s := newTestStore()
	_, _ = s.Create("m1", "A", "1.0.0", TypePerAgent, "agent-a")

	rec, err := s.Mount("m1", "agent-a", AccessReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	if rec.AccessMode != AccessReadWrite {
		t.Fatalf("expected rw, got %s", rec.AccessMode)
	}

	e, err := s.Get("m1")
	if err != nil {
		t.Fatal(err)
	}
	if e.State != StateMounted {
		t.Fatalf("expected mounted, got %s", e.State)
	}

	// Double mount should fail.
	_, err = s.Mount("m1", "agent-a", AccessReadWrite)
	if err == nil {
		t.Fatal("expected error on double mount")
	}

	// Unmount.
	if err := s.Unmount("m1", "agent-a"); err != nil {
		t.Fatal(err)
	}
	e, err = s.Get("m1")
	if err != nil {
		t.Fatal(err)
	}
	if e.State != StateDetached {
		t.Fatalf("expected detached, got %s", e.State)
	}

	// Re-mount should work.
	_, err = s.Mount("m1", "agent-a", AccessReadWrite)
	if err != nil {
		t.Fatalf("re-mount failed: %v", err)
	}
}

func TestMountNotFound(t *testing.T) {
	s := newTestStore()
	_, err := s.Mount("nope", "a", AccessReadOnly)
	if err != ErrMemoryNotFound {
		t.Fatalf("expected ErrMemoryNotFound, got %v", err)
	}
}

func TestUnmountNotMounted(t *testing.T) {
	s := newTestStore()
	_, _ = s.Create("m1", "A", "1.0.0", TypeShared, "")
	if err := s.Unmount("m1", "agent-a"); err != ErrNotMounted {
		t.Fatalf("expected ErrNotMounted, got %v", err)
	}
}

func TestArchive(t *testing.T) {
	s := newTestStore()
	_, _ = s.Create("m1", "A", "1.0.0", TypeShared, "")
	if err := s.Archive("m1"); err != nil {
		t.Fatal(err)
	}
	e, _ := s.Get("m1")
	if e.State != StateArchived {
		t.Fatalf("expected archived, got %s", e.State)
	}
	// Cannot mount archived memory.
	_, err := s.Mount("m1", "agent-a", AccessReadOnly)
	if err == nil {
		t.Fatal("expected error mounting archived memory")
	}
	// Cannot archive again.
	if err := s.Archive("m1"); err == nil {
		t.Fatal("expected error archiving already archived memory")
	}
}

func TestUnmountAll(t *testing.T) {
	s := newTestStore()
	_, _ = s.Create("m1", "A", "1.0.0", TypeShared, "")
	_, _ = s.Create("m2", "B", "1.0.0", TypePublic, "")
	_, _ = s.Mount("m1", "agent-a", AccessReadOnly)
	_, _ = s.Mount("m2", "agent-a", AccessReadOnly)

	n := s.UnmountAll("agent-a")
	if n != 2 {
		t.Fatalf("expected 2 unmounted, got %d", n)
	}
	if len(s.MountsForAgent("agent-a")) != 0 {
		t.Fatal("expected no mounts remaining")
	}
}

func TestMountsForAgent(t *testing.T) {
	s := newTestStore()
	_, _ = s.Create("m1", "A", "1.0.0", TypeShared, "")
	_, _ = s.Mount("m1", "agent-a", AccessReadOnly)

	mounts := s.MountsForAgent("agent-a")
	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(mounts))
	}
	if mounts[0].MemoryID != "m1" {
		t.Fatal("wrong memory id")
	}
}

func TestPerAgentLimitOnePerAgent(t *testing.T) {
	s := newTestStore()
	_, _ = s.Create("m1", "A", "1.0.0", TypePerAgent, "agent-a")
	_, _ = s.Create("m2", "B", "1.0.0", TypePerAgent, "agent-a")

	_, err := s.Mount("m1", "agent-a", AccessReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	// Second per-agent mount should fail.
	_, err = s.Mount("m2", "agent-a", AccessReadWrite)
	if err == nil {
		t.Fatal("expected per-agent limit error")
	}
}

func TestSharedReadOnlyDefault(t *testing.T) {
	s := newTestStore()
	_, _ = s.Create("m1", "A", "1.0.0", TypeShared, "")
	rec, err := s.Mount("m1", "agent-a", AccessReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	if rec.AccessMode != AccessReadOnly {
		t.Fatalf("expected ro, got %s", rec.AccessMode)
	}
}

func TestPublicAlwaysReadOnly(t *testing.T) {
	s := newTestStore()
	_, _ = s.Create("m1", "A", "1.0.0", TypePublic, "")
	rec, err := s.Mount("m1", "agent-a", AccessReadWrite) // request rw
	if err != nil {
		t.Fatal(err)
	}
	if rec.AccessMode != AccessReadOnly {
		t.Fatalf("public should be ro, got %s", rec.AccessMode)
	}
}

func TestPerAgentOwnerRestriction(t *testing.T) {
	s := newTestStore()
	_, _ = s.Create("m1", "A", "1.0.0", TypePerAgent, "agent-a")
	_, err := s.Mount("m1", "agent-b", AccessReadWrite)
	if err == nil {
		t.Fatal("expected owner mismatch error")
	}
}
