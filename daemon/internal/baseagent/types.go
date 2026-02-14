package baseagent

import "context"

type Evidence struct {
	AgentID     string
	LastError   string
	ExitCode    *int
	LogTail     []string
	HealthProbe string
}

type TriageResult struct {
	Resolved               bool
	Summary                string
	SuggestedActions       []string
	RequiresRemoteDiagnosis bool
}

type Triager interface {
	Analyze(ctx context.Context, e Evidence) (TriageResult, error)
}

type NoopTriager struct{}

func (NoopTriager) Analyze(_ context.Context, e Evidence) (TriageResult, error) {
	return TriageResult{
		Resolved:               false,
		Summary:                "Base Agent could not resolve the issue in scaffold mode",
		SuggestedActions:       []string{"Run /diagnose", "Review latest logs", "Confirm remote diagnosis consent"},
		RequiresRemoteDiagnosis: true,
	}, nil
}
