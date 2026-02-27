package policy

import (
	"path/filepath"
	"strings"

	"carrier/codeagent/contract"
)

type StrictPolicy struct {
	workspaceRoot string
}

func NewStrictPolicy(workspaceRoot string) *StrictPolicy {
	return &StrictPolicy{
		workspaceRoot: filepath.Clean(strings.TrimSpace(workspaceRoot)),
	}
}

func (p *StrictPolicy) Decide(req contract.RunRequest) contract.PolicyDecisionEnvelope {
	switch req.Capability {
	case contract.CapabilityReadFile, contract.CapabilityWriteFile, contract.CapabilityApplyPatch:
		return p.decidePath(req.Path)
	case contract.CapabilityRunShell:
		return p.decideCommand(req.Command)
	case contract.CapabilityRunShellRedirect:
		if decision := p.decideCommand(req.Command); decision.Action != contract.PolicyDecisionAllow {
			return decision
		}
		if decision := p.decidePath(req.StdoutPath); decision.Action == contract.PolicyDecisionDeny {
			return decision
		}
		if decision := p.decidePath(req.StderrPath); decision.Action == contract.PolicyDecisionDeny {
			return decision
		}
		return contract.PolicyDecisionEnvelope{Action: contract.PolicyDecisionAllow}
	default:
		return contract.PolicyDecisionEnvelope{Action: contract.PolicyDecisionAllow}
	}
}

func (p *StrictPolicy) decidePath(rawPath string) contract.PolicyDecisionEnvelope {
	path := strings.TrimSpace(rawPath)
	if path == "" {
		return contract.PolicyDecisionEnvelope{Action: contract.PolicyDecisionAllow}
	}
	if p.workspaceRoot == "" || p.workspaceRoot == "." {
		return contract.PolicyDecisionEnvelope{Action: contract.PolicyDecisionAllow}
	}
	cleanedRoot := filepath.Clean(p.workspaceRoot)
	cleaned := path
	if !filepath.IsAbs(cleaned) {
		cleaned = filepath.Join(cleanedRoot, cleaned)
	}
	cleaned = filepath.Clean(cleaned)
	rel, err := filepath.Rel(cleanedRoot, cleaned)
	if err != nil || strings.HasPrefix(rel, "..") {
		return contract.PolicyDecisionEnvelope{
			Action: contract.PolicyDecisionDeny,
			Reason: "path escapes workspace root",
		}
	}
	return contract.PolicyDecisionEnvelope{Action: contract.PolicyDecisionAllow}
}

func (p *StrictPolicy) decideCommand(rawCommand string) contract.PolicyDecisionEnvelope {
	command := strings.ToLower(strings.TrimSpace(rawCommand))
	if command == "" {
		return contract.PolicyDecisionEnvelope{
			Action: contract.PolicyDecisionDeny,
			Reason: "shell command is required",
		}
	}
	if containsAny(command, []string{
		"rm -rf /",
		"mkfs",
		"shutdown",
		"reboot",
		"dd if=",
	}) {
		return contract.PolicyDecisionEnvelope{
			Action: contract.PolicyDecisionDeny,
			Reason: "command denied by strict policy",
		}
	}
	if containsAny(command, []string{
		"curl ",
		"wget ",
		"nc ",
		"ssh ",
		"scp ",
	}) {
		return contract.PolicyDecisionEnvelope{
			Action: contract.PolicyDecisionAsk,
			Reason: "command requires approval by strict policy",
		}
	}
	return contract.PolicyDecisionEnvelope{Action: contract.PolicyDecisionAllow}
}

func containsAny(input string, patterns []string) bool {
	for _, pattern := range patterns {
		if strings.Contains(input, pattern) {
			return true
		}
	}
	return false
}
