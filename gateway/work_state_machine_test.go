package gateway

import (
	"carrier/shared/work"
	"testing"
)

func TestWorkStateMachineAllowsNominalProgression(t *testing.T) {
	item := work.WorkItem{ProjectID: "proj_123", Title: "Add queue", State: work.WorkItemStateNew}
	for _, next := range []work.WorkItemState{
		work.WorkItemStateTriaged,
		work.WorkItemStateQueued,
		work.WorkItemStateClaimed,
		work.WorkItemStateRunning,
		work.WorkItemStateAwaitingReview,
		work.WorkItemStateDone,
	} {
		runID := ""
		if next == work.WorkItemStateClaimed || next == work.WorkItemStateRunning {
			runID = "run_123"
		}
		nextItem, err := transitionWorkItemState(item, next, runID)
		if err != nil {
			t.Fatalf("transition to %s failed: %v", next, err)
		}
		item = nextItem
	}
}

func TestWorkStateMachineAllowsCancelFromActiveState(t *testing.T) {
	item := work.WorkItem{ProjectID: "proj_123", Title: "Add queue", State: work.WorkItemStateRunning}
	nextItem, err := transitionWorkItemState(item, work.WorkItemStateCancelled, "")
	if err != nil {
		t.Fatalf("cancel failed: %v", err)
	}
	if nextItem.State != work.WorkItemStateCancelled {
		t.Fatalf("state=%q", nextItem.State)
	}
}

func TestWorkStateMachineRejectsInvalidTransition(t *testing.T) {
	item := work.WorkItem{ProjectID: "proj_123", Title: "Add queue", State: work.WorkItemStateNew}
	if _, err := transitionWorkItemState(item, work.WorkItemStateRunning, ""); err == nil {
		t.Fatal("expected invalid transition error")
	}
}
