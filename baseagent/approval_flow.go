package baseagent

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type ApprovalDecision string

const (
	ApprovalDecisionConfirm ApprovalDecision = "confirm"
	ApprovalDecisionReject  ApprovalDecision = "reject"
)

var (
	ErrPendingApprovalNotFound = errors.New("pending approval not found")
	ErrInvalidApprovalDecision = errors.New("invalid approval decision")
)

func (r *Runtime) RespondPendingApproval(ctx context.Context, sessionKey, approvalID string, decision ApprovalDecision) (ChatResponse, error) {
	if r == nil || r.loop == nil {
		return ChatResponse{}, fmt.Errorf("base agent runtime is unavailable")
	}
	return r.loop.RespondPendingApproval(ctx, sessionKey, approvalID, decision)
}

func (l *AgentLoop) RespondPendingApproval(ctx context.Context, sessionKey, approvalID string, decision ApprovalDecision) (ChatResponse, error) {
	if l == nil || l.sessions == nil {
		return ChatResponse{}, ErrPendingApprovalNotFound
	}

	normalized, err := normalizeApprovalDecision(decision)
	if err != nil {
		return ChatResponse{}, err
	}

	switch normalized {
	case ApprovalDecisionReject:
		pending, ok := l.sessions.ConsumePendingApproval(sessionKey, approvalID)
		if !ok {
			return ChatResponse{}, ErrPendingApprovalNotFound
		}
		l.sessions.RecordApprovalDecision(sessionKey, pending, approvalDecisionRejected)
		return ChatResponse{
			Message: fmt.Sprintf("Canceled pending approval for %s.", pending.ToolName),
			Action:  "approval_cancel",
		}, nil
	case ApprovalDecisionConfirm:
		if l.structuredTools == nil {
			return ChatResponse{}, fmt.Errorf("structured tool surface is unavailable")
		}

		pending, ok := l.sessions.ConsumePendingApproval(sessionKey, approvalID)
		if !ok {
			return ChatResponse{}, ErrPendingApprovalNotFound
		}
		l.sessions.RecordApprovalDecision(sessionKey, pending, approvalDecisionConfirmed)
		result := l.structuredTools.ExecuteApproved(ctx, pending.ToolName, pending.Arguments)
		toolOutput := renderStructuredToolResultOutput(pending.ToolName, result)
		l.sessions.AddStructuredToolMessage(sessionKey, StructuredToolMessage{
			Role:             "tool",
			Content:          toolOutput,
			ToolName:         strings.TrimSpace(pending.ToolName),
			ToolResultStatus: normalizeExecutionToolResultStatus(result),
			ToolPolicyReason: strings.TrimSpace(result.PolicyReason),
			ToolPolicyRuleID: strings.TrimSpace(result.PolicyRuleID),
		})

		if resp, handled, err := l.processStructuredChat(ctx, sessionKey, l.sessions.History(sessionKey), "", l.resolvedMemorySubject("")); handled {
			resp.Action = "approval_confirm"
			return resp, err
		}
		return ChatResponse{
			Message: toolOutput,
			Action:  "approval_confirm",
		}, nil
	default:
		return ChatResponse{}, ErrInvalidApprovalDecision
	}
}

func normalizeApprovalDecision(decision ApprovalDecision) (ApprovalDecision, error) {
	switch ApprovalDecision(strings.ToLower(strings.TrimSpace(string(decision)))) {
	case ApprovalDecisionConfirm:
		return ApprovalDecisionConfirm, nil
	case ApprovalDecisionReject:
		return ApprovalDecisionReject, nil
	default:
		return "", ErrInvalidApprovalDecision
	}
}
