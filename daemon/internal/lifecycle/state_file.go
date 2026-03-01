package lifecycle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// StateFile manages persistent agent state across daemon restarts.
type StateFile struct {
	path string
}

func cloneMemoryState(src *MemoryState) *MemoryState {
	if src == nil {
		return nil
	}
	cloned := *src
	if src.SyncedAt != nil {
		at := *src.SyncedAt
		cloned.SyncedAt = &at
	}
	return &cloned
}

// PersistedAgentState represents the minimal state needed to restore agent lifecycle.
type PersistedAgentState struct {
	ID             string       `json:"id"`
	Installed      bool         `json:"installed"`
	RuntimeState   string       `json:"runtime_state"`
	Memory         *MemoryState `json:"memory,omitempty"`
	LastTransition time.Time    `json:"last_transition"`
	Restarts       []time.Time  `json:"restarts,omitempty"`
	CooldownUntil  time.Time    `json:"cooldown_until,omitempty"`
}

// NewStateFile creates a StateFile with the given path.
// Default path: /var/lib/carrier/state.json
func NewStateFile(path string) *StateFile {
	if path == "" {
		path = "/var/lib/carrier/state.json"
	}
	return &StateFile{path: path}
}

// Save writes the agent states to disk atomically.
// It writes to a temporary file first, then renames to prevent partial writes.
func (sf *StateFile) Save(agents map[string]*AgentState) error {
	persisted := make(map[string]PersistedAgentState, len(agents))
	for id, state := range agents {
		if state == nil {
			continue
		}
		persisted[id] = PersistedAgentState{
			ID:             state.ID,
			Installed:      state.Install == InstallStateInstalled,
			RuntimeState:   string(state.Runtime),
			Memory:         cloneMemoryState(state.Memory),
			LastTransition: state.UpdatedAt,
		}
	}
	return sf.SavePersisted(persisted)
}

// SavePersisted writes pre-built persisted state to disk atomically.
func (sf *StateFile) SavePersisted(persisted map[string]PersistedAgentState) error {
	if sf == nil || sf.path == "" {
		return nil // no-op if not configured
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	// Ensure parent directory exists
	dir := filepath.Dir(sf.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}

	// Write atomically: write to .tmp, then rename
	tmpPath := sf.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("write temporary state file: %w", err)
	}

	if err := os.Rename(tmpPath, sf.path); err != nil {
		os.Remove(tmpPath) // clean up on failure
		return fmt.Errorf("atomic rename state file: %w", err)
	}

	return nil
}

// Load reads and parses the state file.
// Returns an empty map if the file doesn't exist (not an error).
func (sf *StateFile) Load() (map[string]*PersistedAgentState, error) {
	if sf == nil || sf.path == "" {
		return make(map[string]*PersistedAgentState), nil
	}

	data, err := os.ReadFile(sf.path)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet - return empty map
			return make(map[string]*PersistedAgentState), nil
		}
		return nil, fmt.Errorf("read state file: %w", err)
	}

	var states map[string]PersistedAgentState
	if err := json.Unmarshal(data, &states); err != nil {
		return nil, fmt.Errorf("parse state file: %w", err)
	}

	// Convert to pointer map for easier handling
	result := make(map[string]*PersistedAgentState, len(states))
	for id, state := range states {
		stateCopy := state
		result[id] = &stateCopy
	}

	return result, nil
}
