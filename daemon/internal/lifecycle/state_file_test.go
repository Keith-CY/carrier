package lifecycle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStateFile_SaveAndLoad_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	sf := NewStateFile(statePath)

	// Create some test agent states
	now := time.Now().UTC().Truncate(time.Second)
	agents := map[string]*AgentState{
		"agent1": {
			ID:        "agent1",
			Install:   InstallStateInstalled,
			Runtime:   RuntimeStateRunning,
			UpdatedAt: now,
		},
		"agent2": {
			ID:        "agent2",
			Install:   InstallStateNotInstalled,
			Runtime:   RuntimeStateStopped,
			UpdatedAt: now.Add(-5 * time.Minute),
		},
		"agent3": {
			ID:        "agent3",
			Install:   InstallStateInstalled,
			Runtime:   RuntimeStateStopped,
			UpdatedAt: now.Add(-10 * time.Minute),
		},
	}

	// Save
	if err := sf.Save(agents); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load
	loaded, err := sf.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify
	if len(loaded) != len(agents) {
		t.Fatalf("expected %d agents, got %d", len(agents), len(loaded))
	}

	for id, original := range agents {
		persisted, ok := loaded[id]
		if !ok {
			t.Errorf("agent %s not found in loaded state", id)
			continue
		}

		if persisted.ID != original.ID {
			t.Errorf("agent %s: ID mismatch: got %s, want %s", id, persisted.ID, original.ID)
		}

		expectedInstalled := original.Install == InstallStateInstalled
		if persisted.Installed != expectedInstalled {
			t.Errorf("agent %s: Installed mismatch: got %v, want %v", id, persisted.Installed, expectedInstalled)
		}

		if persisted.RuntimeState != string(original.Runtime) {
			t.Errorf("agent %s: RuntimeState mismatch: got %s, want %s", id, persisted.RuntimeState, string(original.Runtime))
		}

		if !persisted.LastTransition.Equal(original.UpdatedAt) {
			t.Errorf("agent %s: LastTransition mismatch: got %v, want %v", id, persisted.LastTransition, original.UpdatedAt)
		}
	}
}

func TestStateFile_Load_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "nonexistent.json")

	sf := NewStateFile(statePath)

	// Load should return empty map, not error
	loaded, err := sf.Load()
	if err != nil {
		t.Fatalf("Load of missing file should not error: %v", err)
	}

	if len(loaded) != 0 {
		t.Errorf("expected empty map, got %d entries", len(loaded))
	}
}

func TestStateFile_AtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	sf := NewStateFile(statePath)

	// First write
	now := time.Now().UTC().Truncate(time.Second)
	agents1 := map[string]*AgentState{
		"agent1": {
			ID:        "agent1",
			Install:   InstallStateInstalled,
			Runtime:   RuntimeStateRunning,
			UpdatedAt: now,
		},
	}

	if err := sf.Save(agents1); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists and is valid JSON
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	var firstWrite map[string]PersistedAgentState
	if err := json.Unmarshal(data, &firstWrite); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if len(firstWrite) != 1 {
		t.Errorf("expected 1 agent in first write, got %d", len(firstWrite))
	}

	// Second write (simulate update)
	agents2 := map[string]*AgentState{
		"agent1": {
			ID:        "agent1",
			Install:   InstallStateInstalled,
			Runtime:   RuntimeStateStopped,
			UpdatedAt: now.Add(5 * time.Minute),
		},
		"agent2": {
			ID:        "agent2",
			Install:   InstallStateInstalled,
			Runtime:   RuntimeStateRunning,
			UpdatedAt: now.Add(5 * time.Minute),
		},
	}

	if err := sf.Save(agents2); err != nil {
		t.Fatalf("Second Save failed: %v", err)
	}

	// Verify file was atomically updated
	data, err = os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("ReadFile after second write failed: %v", err)
	}

	var secondWrite map[string]PersistedAgentState
	if err := json.Unmarshal(data, &secondWrite); err != nil {
		t.Fatalf("JSON unmarshal after second write failed: %v", err)
	}

	if len(secondWrite) != 2 {
		t.Errorf("expected 2 agents in second write, got %d", len(secondWrite))
	}

	// Verify no .tmp file left behind
	tmpPath := statePath + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf("temporary file should not exist after successful write")
	}
}

func TestStateFile_DefaultPath(t *testing.T) {
	sf := NewStateFile("")
	if sf.path != "/var/lib/carrier/state.json" {
		t.Errorf("expected default path /var/lib/carrier/state.json, got %s", sf.path)
	}
}

func TestStateFile_NilSafe(t *testing.T) {
	var sf *StateFile

	// Save should not panic
	if err := sf.Save(nil); err != nil {
		t.Errorf("Save on nil StateFile should not error: %v", err)
	}

	// Load should not panic and return empty map
	loaded, err := sf.Load()
	if err != nil {
		t.Errorf("Load on nil StateFile should not error: %v", err)
	}
	if len(loaded) != 0 {
		t.Errorf("Load on nil StateFile should return empty map, got %d entries", len(loaded))
	}
}

func TestStateFile_SaveSkipsNilStates(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	sf := NewStateFile(statePath)

	now := time.Now().UTC().Truncate(time.Second)
	agents := map[string]*AgentState{
		"agent1": {
			ID:        "agent1",
			Install:   InstallStateInstalled,
			Runtime:   RuntimeStateRunning,
			UpdatedAt: now,
		},
		"agent2": nil, // nil state should be skipped
	}

	if err := sf.Save(agents); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := sf.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Should only have agent1, not agent2
	if len(loaded) != 1 {
		t.Errorf("expected 1 agent (skipping nil), got %d", len(loaded))
	}

	if _, ok := loaded["agent1"]; !ok {
		t.Errorf("agent1 should be present")
	}

	if _, ok := loaded["agent2"]; ok {
		t.Errorf("agent2 (nil) should not be present")
	}
}
