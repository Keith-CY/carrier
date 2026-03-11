package baseagent

import "strings"

const (
	structuredPolicyRuleExecBlockedCommand     = "exec.blocked_command"
	structuredPolicyRuleExecConfirmationNeeded = "exec.confirmation_required"
	structuredPolicyRuleExecPermissionDenied   = "exec.permission_denied"
	structuredPolicyRuleAgentConfirmation      = "agent.lifecycle_confirmation_required"
	structuredPolicyRuleAgentPermissionDenied  = "agent.lifecycle_permission_denied"
	structuredPolicyRuleSendFileConfirmation   = "send_file.confirmation_required"
	structuredPolicyRuleSendFilePermissionDeny = "send_file.permission_denied"
	structuredPolicyRuleSubagentConfirmation   = "spawn_subagent.confirmation_required"
	structuredPolicyRuleSubagentPermissionDeny = "spawn_subagent.permission_denied"
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
	case "send_file":
		return evaluateStructuredSimpleHighRiskPolicy(defaultDecision, "Sending files requires confirmation.", structuredPolicyRuleSendFileConfirmation, "Sending files is unavailable at the current permission level.", structuredPolicyRuleSendFilePermissionDeny)
	case "spawn_subagent":
		return evaluateStructuredSimpleHighRiskPolicy(defaultDecision, "Subagent delegation requires confirmation.", structuredPolicyRuleSubagentConfirmation, "Subagent delegation is unavailable at the current permission level.", structuredPolicyRuleSubagentPermissionDeny)
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
	return evaluateStructuredSimpleHighRiskPolicy(defaultDecision, "Agent lifecycle mutation requires confirmation.", structuredPolicyRuleAgentConfirmation, "Agent lifecycle mutation is unavailable at the current permission level.", structuredPolicyRuleAgentPermissionDenied)
}

func evaluateStructuredSimpleHighRiskPolicy(defaultDecision structuredToolDecision, askReason, askRuleID, denyReason, denyRuleID string) StructuredPolicyDecision {
	switch defaultDecision {
	case structuredToolDecisionAsk:
		return StructuredPolicyDecision{
			Decision: defaultDecision,
			Reason:   askReason,
			RuleID:   askRuleID,
		}
	case structuredToolDecisionDeny:
		return StructuredPolicyDecision{
			Decision: defaultDecision,
			Reason:   denyReason,
			RuleID:   denyRuleID,
		}
	default:
		return StructuredPolicyDecision{Decision: defaultDecision}
	}
}
