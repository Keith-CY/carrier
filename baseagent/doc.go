// Package baseagent provides Carrier's built-in agent control plane.
//
// It includes:
// - Runtime chat orchestration for agent lifecycle operations
// - Failure triage interfaces and default repair policies
// - LLM provider plumbing with safe fallback behavior
// - Session, tool, channel, and message-bus control-plane primitives
//
// The package is designed so daemon and gateway can depend on a stable runtime
// API while internal orchestration can evolve independently.
package baseagent
