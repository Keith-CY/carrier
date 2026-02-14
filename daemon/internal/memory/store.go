package memory

import (
	"fmt"
	"sync"
	"time"
)

// Store is an in-memory registry of memory packages and their mount records.
// It enforces state transitions and delegates policy checks to Policy.
type Store struct {
	mu      sync.RWMutex
	entries map[string]Entry // keyed by memory ID
	mounts  []MountRecord    // active mounts
	policy  Policy
	now     func() time.Time
}

// StoreOption configures a Store.
type StoreOption func(*Store)

// WithNow overrides the clock (useful for tests).
func WithNow(fn func() time.Time) StoreOption {
	return func(s *Store) { s.now = fn }
}

// NewStore creates an empty memory store.
func NewStore(opts ...StoreOption) *Store {
	s := &Store{
		entries: make(map[string]Entry),
		now:     time.Now,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Create registers a new memory entry in StateCreated.
func (s *Store) Create(id, name, version string, memType Type, owner string) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.entries[id]; exists {
		return Entry{}, fmt.Errorf("memory %q already exists", id)
	}

	now := s.now()
	e := Entry{
		ID:        id,
		Name:      name,
		Version:   version,
		Type:      memType,
		Owner:     owner,
		State:     StateCreated,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.entries[id] = e
	return e, nil
}

// Get returns a memory entry by ID.
func (s *Store) Get(id string) (Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.entries[id]
	if !ok {
		return Entry{}, ErrMemoryNotFound
	}
	return e, nil
}

// List returns all memory entries.
func (s *Store) List() []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Entry, 0, len(s.entries))
	for _, e := range s.entries {
		out = append(out, e)
	}
	return out
}

// Mount attaches a memory to an agent after policy checks.
func (s *Store) Mount(memoryID, agentID string, requestedMode AccessMode) (MountRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.entries[memoryID]
	if !ok {
		return MountRecord{}, ErrMemoryNotFound
	}

	// Collect all active mounts for this agent; Policy.CheckMount filters by type internally.
	agentMounts := s.agentMounts(agentID)

	if err := s.policy.CheckMount(entry, agentID, agentMounts); err != nil {
		return MountRecord{}, err
	}

	// Transition state.
	if err := ValidateTransition(entry.State, StateMounted); err != nil {
		return MountRecord{}, err
	}

	mode := s.policy.ResolveAccessMode(entry.Type, requestedMode)

	now := s.now()
	entry.State = StateMounted
	entry.UpdatedAt = now
	s.entries[memoryID] = entry

	rec := MountRecord{
		MemoryID:   memoryID,
		AgentID:    agentID,
		MemoryType: entry.Type,
		AccessMode: mode,
		MountedAt:  now,
	}
	s.mounts = append(s.mounts, rec)
	return rec, nil
}

// Unmount detaches a memory from an agent, transitioning it to StateDetached.
func (s *Store) Unmount(memoryID, agentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.entries[memoryID]
	if !ok {
		return ErrMemoryNotFound
	}

	idx := -1
	for i, m := range s.mounts {
		if m.MemoryID == memoryID && m.AgentID == agentID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ErrNotMounted
	}

	if err := ValidateTransition(entry.State, StateDetached); err != nil {
		return err
	}

	entry.State = StateDetached
	entry.UpdatedAt = s.now()
	s.entries[memoryID] = entry

	// Remove mount record.
	s.mounts = append(s.mounts[:idx], s.mounts[idx+1:]...)
	return nil
}

// Archive transitions a memory to StateArchived (terminal).
func (s *Store) Archive(memoryID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.entries[memoryID]
	if !ok {
		return ErrMemoryNotFound
	}
	if err := ValidateTransition(entry.State, StateArchived); err != nil {
		return err
	}
	entry.State = StateArchived
	entry.UpdatedAt = s.now()
	s.entries[memoryID] = entry
	return nil
}

// MountsForAgent returns all active mount records for an agent.
func (s *Store) MountsForAgent(agentID string) []MountRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []MountRecord
	for _, m := range s.mounts {
		if m.AgentID == agentID {
			out = append(out, m)
		}
	}
	return out
}

// UnmountAll detaches all memories for a given agent. Returns the count unmounted.
func (s *Store) UnmountAll(agentID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0
	remaining := s.mounts[:0]
	for _, m := range s.mounts {
		if m.AgentID == agentID {
			if entry, ok := s.entries[m.MemoryID]; ok && entry.State == StateMounted {
				entry.State = StateDetached
				entry.UpdatedAt = s.now()
				s.entries[m.MemoryID] = entry
			}
			count++
		} else {
			remaining = append(remaining, m)
		}
	}
	s.mounts = remaining
	return count
}

// agentMounts returns all mount records for an agent, enriching MemoryType from entry data.
func (s *Store) agentMounts(agentID string) []MountRecord {
	var out []MountRecord
	for _, m := range s.mounts {
		if m.AgentID != agentID {
			continue
		}
		// Ensure MemoryType is populated (defensive for older records).
		if m.MemoryType == "" {
			if e, ok := s.entries[m.MemoryID]; ok {
				m.MemoryType = e.Type
			}
		}
		out = append(out, m)
	}
	return out
}
