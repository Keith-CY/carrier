// Package lifecycle implements the core agent lifecycle state machine, managing
// install, start, stop, upgrade, and diagnose operations. It coordinates
// runtime checks, command execution, crash-loop detection, memory management,
// and audit logging.
package lifecycle
