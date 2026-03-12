package baseagent

import "path/filepath"

const defaultMaxToolIterations = 4

type RuntimeOption func(*Runtime)

func WithWorkspaceRoot(path string) RuntimeOption {
	return func(r *Runtime) {
		if path != "" {
			r.workspaceRoot = filepath.Clean(path)
		}
	}
}

func WithMaxToolIterations(iterations int) RuntimeOption {
	return func(r *Runtime) {
		if iterations > 0 {
			r.maxToolIterations = iterations
		}
	}
}

func WithStructuredToolPolicy(policy StructuredToolPolicySpec) RuntimeOption {
	return func(r *Runtime) {
		r.structuredToolPolicyOverride = &StructuredToolPolicySpec{
			MetadataReadDecision:      policy.MetadataReadDecision,
			OperationalReadDecision:   policy.OperationalReadDecision,
			WorkspaceReadDecision:     policy.WorkspaceReadDecision,
			WorkspaceMutationDecision: policy.WorkspaceMutationDecision,
			HighRiskDecision:          policy.HighRiskDecision,
		}
	}
}

func WithMCPManager(manager MCPManager) RuntimeOption {
	return func(r *Runtime) {
		r.mcpManager = manager
	}
}

func WithSkillsLoader(loader SkillsLoader) RuntimeOption {
	return func(r *Runtime) {
		r.skillsLoader = loader
	}
}

func WithMediaRuntime(media MediaRuntime) RuntimeOption {
	return func(r *Runtime) {
		r.mediaRuntime = media
	}
}

func WithWebToolBackend(backend WebToolBackend) RuntimeOption {
	return func(r *Runtime) {
		r.webBackend = backend
	}
}

func WithSubagentSpawner(spawner SubagentSpawner) RuntimeOption {
	return func(r *Runtime) {
		r.subagentSpawner = spawner
	}
}

func WithSubagentManager(manager SubagentManager) RuntimeOption {
	return func(r *Runtime) {
		r.subagentManager = manager
	}
}
