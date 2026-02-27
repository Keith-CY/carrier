package contract

import "context"

type Capability string

const (
	CapabilityReadFile         Capability = "read_file"
	CapabilityWriteFile        Capability = "write_file"
	CapabilityApplyPatch       Capability = "apply_patch"
	CapabilityRunShell         Capability = "run_shell"
	CapabilityRunShellRedirect Capability = "run_shell_redirect"
)

type PolicyDecision string

const (
	PolicyDecisionAllow PolicyDecision = "allow"
	PolicyDecisionAsk   PolicyDecision = "ask"
	PolicyDecisionDeny  PolicyDecision = "deny"
)

type PolicyDecisionEnvelope struct {
	Action PolicyDecision `json:"action"`
	Reason string         `json:"reason,omitempty"`
}

type WriteMode string

const (
	WriteModeOverwrite WriteMode = "overwrite"
	WriteModeAppend    WriteMode = "append"
)

type RunRequest struct {
	Capability      Capability `json:"capability"`
	Path            string     `json:"path,omitempty"`
	Content         string     `json:"content,omitempty"`
	WriteMode       WriteMode  `json:"writeMode,omitempty"`
	Command         string     `json:"command,omitempty"`
	CWD             string     `json:"cwd,omitempty"`
	TimeoutSec      int        `json:"timeoutSec,omitempty"`
	StdoutPath      string     `json:"stdoutPath,omitempty"`
	StderrPath      string     `json:"stderrPath,omitempty"`
	AppendOutput    bool       `json:"appendOutput,omitempty"`
	ResumeSessionID string     `json:"resumeSessionId,omitempty"`
}

type ResultEnvelope struct {
	Ok              bool           `json:"ok"`
	ExitCode        int            `json:"exit_code,omitempty"`
	Stdout          string         `json:"stdout,omitempty"`
	Stderr          string         `json:"stderr,omitempty"`
	FilesTouched    []string       `json:"files_touched,omitempty"`
	DurationMS      int64          `json:"duration_ms"`
	CostEstimateUSD float64        `json:"cost_estimate_usd,omitempty"`
	PolicyDecision  PolicyDecision `json:"policy_decision"`
	PolicyReason    string         `json:"policy_reason,omitempty"`
}

type Target struct {
	HostID        string `json:"hostId,omitempty"`
	WorkspaceRoot string `json:"workspaceRoot,omitempty"`
}

type Profile struct {
	Name   string                 `json:"name,omitempty"`
	Values map[string]interface{} `json:"values,omitempty"`
}

type Adapter interface {
	Install(ctx context.Context, target Target) error
	Configure(ctx context.Context, target Target, profile Profile) error
	Run(ctx context.Context, request RunRequest) (ResultEnvelope, error)
	Health(ctx context.Context) error
	Version(ctx context.Context) (string, error)
	Supports(capability Capability) bool
}
