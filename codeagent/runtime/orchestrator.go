package runtime

import (
	"context"

	"carrier/codeagent/contract"
)

type Middleware func(ctx context.Context, req contract.RunRequest) contract.PolicyDecisionEnvelope

type Orchestrator struct {
	adapter     contract.Adapter
	middlewares []Middleware
}

func NewOrchestrator(adapter contract.Adapter, middlewares []Middleware) *Orchestrator {
	return &Orchestrator{
		adapter:     adapter,
		middlewares: middlewares,
	}
}

func (o *Orchestrator) Run(ctx context.Context, req contract.RunRequest) (contract.ResultEnvelope, error) {
	for _, middleware := range o.middlewares {
		decision := middleware(ctx, req)
		if decision.Action == "" {
			decision.Action = contract.PolicyDecisionAllow
		}
		if decision.Action == contract.PolicyDecisionAsk || decision.Action == contract.PolicyDecisionDeny {
			return contract.ResultEnvelope{
				Ok:             false,
				PolicyDecision: decision.Action,
				PolicyReason:   decision.Reason,
			}, nil
		}
	}

	result, err := o.adapter.Run(ctx, req)
	if result.PolicyDecision == "" {
		result.PolicyDecision = contract.PolicyDecisionAllow
	}
	return result, err
}
