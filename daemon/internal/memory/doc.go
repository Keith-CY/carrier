// Package memory implements an in-memory store for memory packages — named,
// versioned data units that can be mounted to and unmounted from agents.
// It enforces state transitions (created → mounted → detached → archived)
// and access-mode policies per memory type.
package memory
