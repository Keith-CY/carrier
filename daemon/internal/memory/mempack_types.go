package memory

import "time"

// PackageManifest describes the memory.yaml metadata inside a mempack artifact.
type PackageManifest struct {
	SchemaVersion string           `json:"schema_version"`
	ID            string           `json:"id"`
	Name          string           `json:"name"`
	Version       string           `json:"version"`
	Region        Type             `json:"region"`
	Kind          string           `json:"type"`
	Publisher     string           `json:"publisher,omitempty"`
	Provenance    Provenance       `json:"provenance"`
	Collections   []CollectionSpec `json:"collections"`
	Mount         MountDefaults    `json:"mount"`
}

type Provenance struct {
	Source string `json:"source"`
	URI    string `json:"uri"`
	Digest string `json:"digest"`
}

type CollectionSpec struct {
	ID           string `json:"id"`
	Path         string `json:"path"`
	Sensitivity  string `json:"sensitivity"`
	DefaultMount string `json:"default_mount"`
}

type MountDefaults struct {
	DefaultMode AccessMode `json:"default_mode"`
	DefaultSlot string     `json:"default_slot"`
}

type ImportOptions struct {
	TargetRegion Type
	Owner        string
	Publisher    string
	Actor        string
	RequestID    string
}

type ExportOptions struct {
	Collections []string
	Actor       string
	RequestID   string
}

type AttachOptions struct {
	Mode        AccessMode
	Priority    int
	Collections []string
	Actor       string
	RequestID   string
}

type Attachment struct {
	AgentID     string     `json:"agent_id"`
	MemoryID    string     `json:"memory_id"`
	Mode        AccessMode `json:"mode"`
	Priority    int        `json:"priority"`
	Collections []string   `json:"collections,omitempty"`
	AttachedAt  time.Time  `json:"attached_at"`
}

type ViewSource struct {
	MemoryID    string   `json:"memory_id"`
	Region      Type     `json:"region"`
	Priority    int      `json:"priority"`
	Collections []string `json:"collections,omitempty"`
	SourcePath  string   `json:"source_path"`
}

type ViewConflict struct {
	Path             string `json:"path"`
	PreviousMemoryID string `json:"previous_memory_id"`
	CurrentMemoryID  string `json:"current_memory_id"`
}

type ViewExplanation struct {
	AgentID      string         `json:"agent_id"`
	ViewPath     string         `json:"view_path"`
	MountMapPath string         `json:"mountmap_path"`
	Digest       string         `json:"digest"`
	GeneratedAt  time.Time      `json:"generated_at"`
	Sources      []ViewSource   `json:"sources"`
	Conflicts    []ViewConflict `json:"conflicts"`
}

type RuntimeMount struct {
	Source string     `json:"source"`
	Target string     `json:"target"`
	Mode   AccessMode `json:"mode"`
}

type RuntimeMemoryContract struct {
	AgentID          string            `json:"agent_id"`
	ViewPath         string            `json:"view_path"`
	PrivateWritePath string            `json:"private_write_path"`
	MountMapPath     string            `json:"mountmap_path"`
	ViewDigest       string            `json:"view_digest"`
	Mounts           []RuntimeMount    `json:"mounts"`
	Env              map[string]string `json:"env"`
	Explanation      ViewExplanation   `json:"explanation"`
}

type AuditEvent struct {
	RequestID string    `json:"request_id"`
	Actor     string    `json:"actor"`
	Action    string    `json:"action"`
	Target    string    `json:"target"`
	Result    string    `json:"result"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

const (
	auditResultSuccess = "success"
	auditResultFailure = "failure"
)
