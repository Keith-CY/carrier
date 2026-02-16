package catalog

import "sort"

// CandidateStatus represents the lifecycle stage of a catalog entry.
type CandidateStatus string

const (
	StatusActive    CandidateStatus = "active"
	StatusCandidate CandidateStatus = "candidate"
)

// Entry represents an agent in the catalog with version and capability metadata.
type Entry struct {
	ID           string
	Name         string
	Version      string
	Status       CandidateStatus
	Capabilities []string
	Description  string
}

// List returns all catalog entries sorted by ID.
func List() []Entry {
	return DefaultEntries()
}

// ListByStatus returns entries filtered by candidate status.
func ListByStatus(status CandidateStatus) []Entry {
	var result []Entry
	for _, e := range DefaultEntries() {
		if e.Status == status {
			result = append(result, e)
		}
	}
	return result
}

// DefaultEntries returns the built-in catalog sorted by ID.
func DefaultEntries() []Entry {
	entries := []Entry{
		{
			ID:           "openclaw",
			Name:         "OpenClaw",
			Version:      "1.0.0",
			Status:       StatusActive,
			Capabilities: []string{"chat", "code", "memory"},
			Description:  "Full-featured AI assistant with memory support",
		},
		{
			ID:           "pi-mono",
			Name:         "Pi Mono",
			Version:      "0.1.0",
			Status:       StatusCandidate,
			Capabilities: []string{"chat"},
			Description:  "Lightweight conversational agent",
		},
		{
			ID:           "nanoclaw",
			Name:         "NanoClaw",
			Version:      "0.1.0",
			Status:       StatusCandidate,
			Capabilities: []string{"chat", "code"},
			Description:  "Compact coding assistant",
		},
		{
			ID:           "zeroclaw",
			Name:         "ZeroClaw",
			Version:      "0.1.0",
			Status:       StatusActive,
			Capabilities: []string{"chat", "code"},
			Description:  "Rust-based AI assistant with chat and code capabilities",
		},
		{
			ID:           "picoclaw",
			Name:         "Pico Claw",
			Version:      "0.1.0",
			Status:       StatusCandidate,
			Capabilities: []string{"chat"},
			Description:  "Minimal chat agent",
		},
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ID < entries[j].ID
	})
	return entries
}

// FindByID looks up a catalog entry by its ID.
func FindByID(id string) (Entry, bool) {
	for _, e := range DefaultEntries() {
		if e.ID == id {
			return e, true
		}
	}
	return Entry{}, false
}

// IsRunnable returns true if the entry has active status and can be installed/started.
func (e Entry) IsRunnable() bool {
	return e.Status == StatusActive
}

// HasCapability checks if the entry advertises the given capability.
func (e Entry) HasCapability(cap string) bool {
	for _, c := range e.Capabilities {
		if c == cap {
			return true
		}
	}
	return false
}
