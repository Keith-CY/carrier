package memory

import "time"

// PackageManifest describes the memory.yaml metadata inside a mempack artifact.
type PackageManifest struct {
	SchemaVersion string           `json:"schema_version" yaml:"schema_version"`
	ID            string           `json:"id" yaml:"id"`
	Name          string           `json:"name" yaml:"name"`
	Version       string           `json:"version" yaml:"version"`
	Region        Type             `json:"region" yaml:"region"`
	Kind          string           `json:"type" yaml:"type"`
	Publisher     string           `json:"publisher,omitempty" yaml:"publisher,omitempty"`
	Provenance    Provenance       `json:"provenance" yaml:"provenance"`
	Collections   []CollectionSpec `json:"collections" yaml:"collections"`
	Mount         MountDefaults    `json:"mount" yaml:"mount"`
}

type Provenance struct {
	Source string `json:"source" yaml:"source"`
	URI    string `json:"uri" yaml:"uri"`
	Digest string `json:"digest" yaml:"digest"`
}

type CollectionSpec struct {
	ID           string `json:"id" yaml:"id"`
	Path         string `json:"path" yaml:"path"`
	Sensitivity  string `json:"sensitivity" yaml:"sensitivity"`
	DefaultMount string `json:"default_mount" yaml:"default_mount"`
}

type MountDefaults struct {
	DefaultMode AccessMode `json:"default_mode" yaml:"default_mode"`
	DefaultSlot string     `json:"default_slot" yaml:"default_slot"`
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
	Format      string `json:"format,omitempty"`
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
	PreviousDigest   string `json:"previous_digest,omitempty"`
	CurrentMemoryID  string `json:"current_memory_id"`
	CurrentDigest    string `json:"current_digest,omitempty"`
	WinnerMemoryID   string `json:"winner_memory_id,omitempty"`
	Resolution       string `json:"resolution,omitempty"`
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
