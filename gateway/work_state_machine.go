package gateway

import (
	"carrier/shared/work"
	"fmt"
	"strings"
)

func transitionWorkItemState(item work.WorkItem, next work.WorkItemState, runID string) (work.WorkItem, error) {
	normalized, err := work.NormalizeWorkItem(item)
	if err != nil {
		return work.WorkItem{}, err
	}
	target := work.WorkItemState(strings.TrimSpace(string(next)))
	if target == "" {
		return work.WorkItem{}, fmt.Errorf("work item target state is required")
	}
	if normalized.State == target {
		return normalized, nil
	}
	if !canTransitionWorkItemState(normalized.State, target) {
		return work.WorkItem{}, fmt.Errorf("invalid work item transition %s -> %s", normalized.State, target)
	}
	normalized.State = target
	trimmedRunID := strings.TrimSpace(runID)
	switch target {
	case work.WorkItemStateClaimed:
		if trimmedRunID == "" {
			return work.WorkItem{}, fmt.Errorf("run id is required when claiming a work item")
		}
		normalized.ClaimedByRunID = trimmedRunID
		normalized.LatestRunID = trimmedRunID
	case work.WorkItemStateRunning:
		if trimmedRunID == "" {
			trimmedRunID = normalized.ClaimedByRunID
		}
		if trimmedRunID == "" {
			return work.WorkItem{}, fmt.Errorf("run id is required when starting a work item run")
		}
		normalized.ClaimedByRunID = trimmedRunID
		normalized.LatestRunID = trimmedRunID
	default:
		normalized.ClaimedByRunID = ""
		if trimmedRunID != "" {
			normalized.LatestRunID = trimmedRunID
		}
	}
	return normalized, nil
}

func canTransitionWorkItemState(from, to work.WorkItemState) bool {
	switch from {
	case work.WorkItemStateNew:
		return to == work.WorkItemStateTriaged || to == work.WorkItemStateCancelled
	case work.WorkItemStateTriaged:
		return to == work.WorkItemStateQueued || to == work.WorkItemStateCancelled
	case work.WorkItemStateQueued:
		return to == work.WorkItemStateClaimed || to == work.WorkItemStateCancelled
	case work.WorkItemStateClaimed:
		return to == work.WorkItemStateRunning || to == work.WorkItemStateCancelled
	case work.WorkItemStateRunning:
		return to == work.WorkItemStateBlocked || to == work.WorkItemStateAwaitingReview || to == work.WorkItemStateDone || to == work.WorkItemStateCancelled
	case work.WorkItemStateBlocked:
		return to == work.WorkItemStateQueued || to == work.WorkItemStateCancelled
	case work.WorkItemStateAwaitingReview:
		return to == work.WorkItemStateDone || to == work.WorkItemStateCancelled
	default:
		return false
	}
}
