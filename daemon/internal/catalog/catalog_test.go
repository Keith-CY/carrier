package catalog

import "testing"

func TestDefaultEntriesContainsOpenClawAsActive(t *testing.T) {
	entries := DefaultEntries()
	if len(entries) != 7 {
		t.Fatalf("expected 7 entries, got %d", len(entries))
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
	ids := []string{"pi-mono", "nanoclaw"}
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

func TestPicoClawIsActive(t *testing.T) {
	entry, ok := FindByID("picoclaw")
	if !ok {
		t.Fatal("expected picoclaw in catalog")
	}
	if entry.Status != StatusActive {
		t.Fatalf("expected picoclaw status active, got %s", entry.Status)
	}
	if !entry.HasCapability("chat") || !entry.HasCapability("code") {
		t.Fatal("expected picoclaw to have chat and code capabilities")
	}
	if !entry.IsRunnable() {
		t.Fatal("expected picoclaw to be runnable")
	}
}

func TestZeroClawIsActive(t *testing.T) {
	entry, ok := FindByID("zeroclaw")
	if !ok {
		t.Fatal("expected zeroclaw in catalog")
	}
	if entry.Status != StatusActive {
		t.Fatalf("expected zeroclaw status active, got %s", entry.Status)
	}
	if !entry.HasCapability("chat") || !entry.HasCapability("code") {
		t.Fatal("expected zeroclaw to have chat and code capabilities")
	}
	if !entry.IsRunnable() {
		t.Fatal("expected zeroclaw to be runnable")
	}
}

func TestCodexAndOpenCodeAreActive(t *testing.T) {
	for _, id := range []string{"codex", "opencode"} {
		entry, ok := FindByID(id)
		if !ok {
			t.Fatalf("expected %s in catalog", id)
		}
		if entry.Status != StatusActive {
			t.Fatalf("expected %s status active, got %s", id, entry.Status)
		}
		if !entry.HasCapability("code") {
			t.Fatalf("expected %s to have code capability", id)
		}
	}
}

func TestListReturnsAllEntries(t *testing.T) {
	all := List()
	if len(all) != 7 {
		t.Fatalf("expected 7 entries from List(), got %d", len(all))
	}
}

func TestActiveEntriesOnlyReturnsActive(t *testing.T) {
	active := ActiveEntries()
	if len(active) != 5 {
		t.Fatalf("expected 5 active entries from ActiveEntries(), got %d", len(active))
	}
	for _, e := range active {
		if e.Status != StatusActive {
			t.Fatalf("ActiveEntries() returned non-active entry: %s (%s)", e.ID, e.Status)
		}
	}
}

func TestActiveEntriesExcludesCandidates(t *testing.T) {
	active := ActiveEntries()
	for _, e := range active {
		if e.ID == "nanoclaw" || e.ID == "pi-mono" {
			t.Fatalf("ActiveEntries() should not include candidate %s", e.ID)
		}
	}
}

func TestFindByIDNotFound(t *testing.T) {
	_, ok := FindByID("nonexistent-agent")
	if ok {
		t.Fatal("expected FindByID to return false for nonexistent agent")
	}
}

func TestListByStatusActive(t *testing.T) {
	active := ListByStatus(StatusActive)
	if len(active) != 5 {
		t.Fatalf("expected 5 active entries, got %d", len(active))
	}
}

func TestListByStatusCandidate(t *testing.T) {
	candidates := ListByStatus(StatusCandidate)
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidate entries, got %d", len(candidates))
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
