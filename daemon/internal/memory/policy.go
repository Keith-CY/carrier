package memory

import (
	"errors"
	"fmt"
)

var (
	ErrMountDenied    = errors.New("mount denied by policy")
	ErrAlreadyMounted = errors.New("memory already mounted by this agent")
	ErrPerAgentLimit  = errors.New("agent already has a per-agent memory mounted")
	ErrNotMounted     = errors.New("memory is not mounted by this agent")
	ErrMemoryNotFound = errors.New("memory not found")
	ErrInvalidState   = errors.New("memory is not in a mountable state")
	ErrOwnerMismatch  = errors.New("per-agent memory can only be mounted by its owner")
)

// Policy enforces mount rules per the PRD:
//   - Per-Agent: read-write, only by owner, at most 1 per agent
//   - Shared:    read-only by default (writable requires explicit flag)
//   - Public:    read-only always
type Policy struct{}

// DefaultAccessMode returns the default access mode for a memory type.
func (Policy) DefaultAccessMode(t Type) AccessMode {
	if t == TypePerAgent {
		return AccessReadWrite
	}
	return AccessReadOnly
}

// CheckMount validates whether agentID may mount the given memory entry,
// considering existing mounts.
func (p Policy) CheckMount(entry Entry, agentID string, mounts []MountRecord) error {
	// Only mountable in created or detached state.
	if entry.State != StateCreated && entry.State != StateDetached {
		return fmt.Errorf("%w: current state is %s", ErrInvalidState, entry.State)
	}

	switch entry.Type {
	case TypePerAgent:
		// Per-agent memory can only be mounted by its owner.
		if entry.Owner != agentID {
			return fmt.Errorf("%w: memory owner is %q, requester is %q", ErrOwnerMismatch, entry.Owner, agentID)
		}
		// An agent may have at most one per-agent memory mounted.
		for _, m := range mounts {
			if m.AgentID == agentID && m.MemoryID != entry.ID && m.MemoryType == TypePerAgent {
				return ErrPerAgentLimit
			}
		}
	case TypeShared, TypePublic:
		// Any agent may mount shared/public memory.
	default:
		return fmt.Errorf("%w: unknown memory type %q", ErrMountDenied, entry.Type)
	}

	// Check duplicate mount.
	for _, m := range mounts {
		if m.MemoryID == entry.ID && m.AgentID == agentID {
			return ErrAlreadyMounted
		}
	}

	return nil
}

// ResolveAccessMode returns the effective access mode for a mount.
// writable requests are only honoured for per-agent memory.
func (p Policy) ResolveAccessMode(t Type, requested AccessMode) AccessMode {
	if t == TypePublic {
		return AccessReadOnly
	}
	if t == TypeShared {
		// Shared memory can be writable only with explicit authorization.
		if requested == AccessReadWrite {
			return AccessReadWrite
		}
		return AccessReadOnly
	}
	// per_agent — always rw
	return AccessReadWrite
}
