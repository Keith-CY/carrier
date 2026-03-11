package baseagent

import "strings"

const (
	structuredPolicyRuleExecBlockedCommand     = "exec.blocked_command"
	structuredPolicyRuleExecConfirmationNeeded = "exec.confirmation_required"
	structuredPolicyRuleExecPermissionDenied   = "exec.permission_denied"
	structuredPolicyRuleAgentConfirmation      = "agent.lifecycle_confirmation_required"
	structuredPolicyRuleAgentPermissionDenied  = "agent.lifecycle_permission_denied"
)

type StructuredPolicyDecision struct {
	Decision structuredToolDecision
	Reason   string
	RuleID   string
}

func evaluateStructuredToolPolicy(toolName string, args map[string]any, defaultDecision structuredToolDecision) StructuredPolicyDecision {
	name := strings.TrimSpace(toolName)
	switch name {
	case "exec":
		return evaluateStructuredExecPolicy(args, defaultDecision)
	case "agent_start", "agent_stop", "agent_upgrade", "agent_uninstall", "agent_diagnose":
		return evaluateStructuredAgentLifecyclePolicy(defaultDecision)
	default:
		return StructuredPolicyDecision{Decision: defaultDecision}
	}
}

func evaluateStructuredExecPolicy(args map[string]any, defaultDecision structuredToolDecision) StructuredPolicyDecision {
	command := strings.TrimSpace(stringifyStructuredToolArg(args["command"]))
	if isStructuredExecCommandDenied(command) {
		return StructuredPolicyDecision{
			Decision: structuredToolDecisionDeny,
			Reason:   "Command matches blocked execution policy.",
			RuleID:   structuredPolicyRuleExecBlockedCommand,
		}
	}

	switch defaultDecision {
	case structuredToolDecisionAsk:
		return StructuredPolicyDecision{
			Decision: defaultDecision,
			Reason:   "Shell execution requires confirmation.",
			RuleID:   structuredPolicyRuleExecConfirmationNeeded,
		}
	case structuredToolDecisionDeny:
		return StructuredPolicyDecision{
			Decision: defaultDecision,
			Reason:   "Shell execution is unavailable at the current permission level.",
			RuleID:   structuredPolicyRuleExecPermissionDenied,
		}
	default:
		return StructuredPolicyDecision{Decision: defaultDecision}
	}
}

func evaluateStructuredAgentLifecyclePolicy(defaultDecision structuredToolDecision) StructuredPolicyDecision {
	switch defaultDecision {
	case structuredToolDecisionAsk:
		return StructuredPolicyDecision{
			Decision: defaultDecision,
			Reason:   "Agent lifecycle mutation requires confirmation.",
			RuleID:   structuredPolicyRuleAgentConfirmation,
		}
	case structuredToolDecisionDeny:
		return StructuredPolicyDecision{
			Decision: defaultDecision,
			Reason:   "Agent lifecycle mutation is unavailable at the current permission level.",
			RuleID:   structuredPolicyRuleAgentPermissionDenied,
		}
	default:
		return StructuredPolicyDecision{Decision: defaultDecision}
	}
}
