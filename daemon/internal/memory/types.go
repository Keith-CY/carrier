// Package memory implements Memory Platform lifecycle and mount policy.
package memory

import (
	"fmt"
	"time"
)

// Type classifies a memory package.
type Type string

const (
	TypePerAgent Type = "per_agent"
	TypeShared   Type = "shared"
	TypePublic   Type = "public"
)

// ValidTypes returns all valid memory types.
func ValidTypes() []Type { return []Type{TypePerAgent, TypeShared, TypePublic} }

// State represents the lifecycle state of a memory package.
type State string

const (
	StateCreated  State = "created"
	StateMounted  State = "mounted"
	StateDetached State = "detached"
	StateArchived State = "archived"
)

// AccessMode describes how a memory is mounted.
type AccessMode string

const (
	AccessReadOnly  AccessMode = "ro"
	AccessReadWrite AccessMode = "rw"
)

// Entry is a memory package registered in the store.
type Entry struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Version   string    `json:"version"`
	Type      Type      `json:"type"`
	Owner     string    `json:"owner"`      // agent ID that created it (empty for public)
	State     State     `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MountRecord tracks one memory→agent attachment.
type MountRecord struct {
	MemoryID   string     `json:"memory_id"`
	AgentID    string     `json:"agent_id"`
	AccessMode AccessMode `json:"access_mode"`
	MountedAt  time.Time  `json:"mounted_at"`
}

// validTransitions defines allowed state transitions.
var validTransitions = map[State][]State{
	StateCreated:  {StateMounted, StateArchived},
	StateMounted:  {StateDetached},
	StateDetached: {StateMounted, StateArchived},
	StateArchived: {}, // terminal
}

// ValidateTransition checks whether from→to is a legal state transition.
func ValidateTransition(from, to State) error {
	targets, ok := validTransitions[from]
	if !ok {
		return fmt.Errorf("unknown state %q", from)
	}
	for _, t := range targets {
		if t == to {
			return nil
		}
	}
	return fmt.Errorf("invalid state transition %s → %s", from, to)
}
