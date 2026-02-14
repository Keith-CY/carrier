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

func TestListReturnsAllEntries(t *testing.T) {
	all := List()
	if len(all) != 4 {
		t.Fatalf("expected 4 entries from List(), got %d", len(all))
	}
}

func TestListByStatusActive(t *testing.T) {
	active := ListByStatus(StatusActive)
	if len(active) != 1 {
		t.Fatalf("expected 1 active entry, got %d", len(active))
	}
	if active[0].ID != "openclaw" {
		t.Fatalf("expected openclaw, got %s", active[0].ID)
	}
}

func TestListByStatusCandidate(t *testing.T) {
	candidates := ListByStatus(StatusCandidate)
	if len(candidates) != 3 {
		t.Fatalf("expected 3 candidate entries, got %d", len(candidates))
	}
}

func TestEntryVersions(t *testing.T) {
	openclaw, _ := FindByID("openclaw")
	if openclaw.Version != "1.0.0" {
		t.Fatalf("expected openclaw version 1.0.0, got %s", openclaw.Version)
	}
	pi, _ := FindByID("pi-mono")
	if pi.Version != "0.1.0" {
		t.Fatalf("expected pi-mono version 0.1.0, got %s", pi.Version)
	}
}

func TestEntryCapabilities(t *testing.T) {
	openclaw, _ := FindByID("openclaw")
	if !openclaw.HasCapability("chat") {
		t.Fatal("expected openclaw to have chat capability")
	}
	if !openclaw.HasCapability("code") {
		t.Fatal("expected openclaw to have code capability")
	}
	if !openclaw.HasCapability("memory") {
		t.Fatal("expected openclaw to have memory capability")
	}
	if openclaw.HasCapability("nonexistent") {
		t.Fatal("expected openclaw not to have nonexistent capability")
	}
}

func TestEntryIsRunnable(t *testing.T) {
	openclaw, _ := FindByID("openclaw")
	if !openclaw.IsRunnable() {
		t.Fatal("expected openclaw to be runnable")
	}
	pi, _ := FindByID("pi-mono")
	if pi.IsRunnable() {
		t.Fatal("expected pi-mono not to be runnable")
	}
}

func TestEntryDescriptions(t *testing.T) {
	openclaw, _ := FindByID("openclaw")
	if openclaw.Description == "" {
		t.Fatal("expected openclaw to have a description")
	}
}

func TestEntriesSortedByID(t *testing.T) {
	entries := DefaultEntries()
	for i := 1; i < len(entries); i++ {
		if entries[i].ID < entries[i-1].ID {
			t.Fatalf("entries not sorted: %s before %s", entries[i-1].ID, entries[i].ID)
		}
	}
}
