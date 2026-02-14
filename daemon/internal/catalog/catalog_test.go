package catalog

import "testing"

func TestDefaultEntriesContainsOpenClawAsActive(t *testing.T) {
	entries := DefaultEntries()
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}

	openclaw, ok := FindByID("openclaw")
	if !ok {
		t.Fatal("expected openclaw in catalog")
	}
	if openclaw.Status != StatusActive {
		t.Fatalf("expected openclaw status active, got %s", openclaw.Status)
	}
}

func TestCandidateAgentsPresent(t *testing.T) {
	ids := []string{"pi-mono", "nanoclaw", "picoclaw"}
	for _, id := range ids {
		entry, ok := FindByID(id)
		if !ok {
			t.Fatalf("expected %s in catalog", id)
		}
		if entry.Status != StatusCandidate {
			t.Fatalf("expected %s to be candidate, got %s", id, entry.Status)
		}
	}
}
