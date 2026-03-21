package baseagent

import (
	"strings"
	"time"
)

type GuardrailScope string
type GuardrailDecision string

const (
	GuardrailScopeExecutionLaunch GuardrailScope = "execution_launch"
	GuardrailScopeAgentInput      GuardrailScope = "agent_input"
	GuardrailScopeToolCall        GuardrailScope = "tool_call"
	GuardrailScopeAgentOutput     GuardrailScope = "agent_output"

	GuardrailDecisionAllow GuardrailDecision = "allow"
	GuardrailDecisionWarn  GuardrailDecision = "warn"
	GuardrailDecisionAsk   GuardrailDecision = "ask"
	GuardrailDecisionDeny  GuardrailDecision = "deny"
)

type GuardrailEvent struct {
	Scope       GuardrailScope    `json:"scope"`
	Decision    GuardrailDecision `json:"decision"`
	RuleID      string            `json:"ruleId,omitempty"`
	Reason      string            `json:"reason,omitempty"`
	ToolName    string            `json:"toolName,omitempty"`
	ApprovalID  string            `json:"approvalId,omitempty"`
	TriggeredAt string            `json:"triggeredAt,omitempty"`
	ResolvedAt  string            `json:"resolvedAt,omitempty"`
	Resolution  string            `json:"resolution,omitempty"`
}

func NormalizeGuardrailEvents(in []GuardrailEvent) []GuardrailEvent {
	if len(in) == 0 {
		return nil
	}
	out := make([]GuardrailEvent, 0, len(in))
	for _, item := range in {
		scope := GuardrailScope(strings.TrimSpace(string(item.Scope)))
		decision := GuardrailDecision(strings.TrimSpace(string(item.Decision)))
		if scope == "" || decision == "" {
			continue
		}
		out = append(out, GuardrailEvent{
			Scope:       scope,
			Decision:    decision,
			RuleID:      strings.TrimSpace(item.RuleID),
			Reason:      strings.TrimSpace(item.Reason),
			ToolName:    strings.TrimSpace(item.ToolName),
			ApprovalID:  strings.TrimSpace(item.ApprovalID),
			TriggeredAt: strings.TrimSpace(item.TriggeredAt),
			ResolvedAt:  strings.TrimSpace(item.ResolvedAt),
			Resolution:  strings.TrimSpace(item.Resolution),
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func guardrailDecisionFromStructuredDecision(decision structuredToolDecision) GuardrailDecision {
	switch decision {
	case structuredToolDecisionAsk:
		return GuardrailDecisionAsk
	case structuredToolDecisionDeny:
		return GuardrailDecisionDeny
	default:
		return GuardrailDecisionAllow
	}
}

func structuredPolicyGuardrailEvents(toolName string, decision StructuredPolicyDecision) []GuardrailEvent {
	guardrailDecision := guardrailDecisionFromStructuredDecision(decision.Decision)
	if guardrailDecision == GuardrailDecisionAllow {
		return nil
	}
	return []GuardrailEvent{{
		Scope:       GuardrailScopeToolCall,
		Decision:    guardrailDecision,
		RuleID:      strings.TrimSpace(decision.RuleID),
		Reason:      strings.TrimSpace(decision.Reason),
		ToolName:    strings.TrimSpace(toolName),
		TriggeredAt: time.Now().UTC().Format(time.RFC3339Nano),
	}}
}

func guardrailResolutionEventFromPending(pending *PendingToolApproval, resolution string, now time.Time) GuardrailEvent {
	event := GuardrailEvent{
		Scope:      GuardrailScopeToolCall,
		Decision:   GuardrailDecisionAsk,
		Resolution: strings.TrimSpace(resolution),
	}
	if pending != nil {
		event.RuleID = strings.TrimSpace(pending.RuleID)
		event.Reason = strings.TrimSpace(pending.Reason)
		event.ToolName = strings.TrimSpace(pending.ToolName)
		event.ApprovalID = strings.TrimSpace(pending.ID)
		if !pending.RequestedAt.IsZero() {
			event.TriggeredAt = pending.RequestedAt.UTC().Format(time.RFC3339Nano)
		}
	}
	if !now.IsZero() {
		event.ResolvedAt = now.UTC().Format(time.RFC3339Nano)
	}
	return event
}
