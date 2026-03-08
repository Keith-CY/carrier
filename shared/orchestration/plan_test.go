package orchestration

import "testing"

func TestBuildPlanAssignsWorkersByAgentAcrossHosts(t *testing.T) {
	plan, err := BuildPlan(BuildPlanInput{
		Goal:           "triage incident",
		Provider:       "openrouter",
		HostIDs:        []string{"host-a", "host-a", "host-b"},
		MaxConcurrency: 9,
		Tasks: []DecomposeTask{
			{ID: "task-1", Input: "collect logs", AgentID: "zeroclaw"},
			{ID: "task-2", Input: "summarize logs", AgentID: "zeroclaw"},
			{ID: "task-3", Input: "extract action items", AgentID: "picoclaw"},
		},
	})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}

	if plan.Goal != "triage incident" {
		t.Fatalf("goal = %q, want triage incident", plan.Goal)
	}
	if plan.Provider != "openrouter" {
		t.Fatalf("provider = %q, want openrouter", plan.Provider)
	}
	if len(plan.HostIDs) != 2 || plan.HostIDs[0] != "host-a" || plan.HostIDs[1] != "host-b" {
		t.Fatalf("hostIDs = %v, want [host-a host-b]", plan.HostIDs)
	}
	if plan.ApprovalScope != "infrastructure_only" {
		t.Fatalf("approvalScope = %q, want infrastructure_only", plan.ApprovalScope)
	}
	if plan.MaxConcurrency != 3 {
		t.Fatalf("maxConcurrency = %d, want 3", plan.MaxConcurrency)
	}
	if len(plan.TaskUnits) != 3 {
		t.Fatalf("taskUnits = %d, want 3", len(plan.TaskUnits))
	}

	if got := plan.TaskUnits[0]; got.HostID != "host-a" || got.AgentID != "zeroclaw" {
		t.Fatalf("taskUnits[0] = %+v, want host-a/zeroclaw", got)
	}
	if got := plan.TaskUnits[1]; got.HostID != "host-b" || got.AgentID != "zeroclaw" {
		t.Fatalf("taskUnits[1] = %+v, want host-b/zeroclaw", got)
	}
	if got := plan.TaskUnits[2]; got.HostID != "host-a" || got.AgentID != "picoclaw" {
		t.Fatalf("taskUnits[2] = %+v, want host-a/picoclaw", got)
	}

	if len(plan.RequiredWorkers) != 3 {
		t.Fatalf("requiredWorkers = %d, want 3", len(plan.RequiredWorkers))
	}
}

func TestBuildPlanFallsBackToSingleZeroclawTask(t *testing.T) {
	plan, err := BuildPlan(BuildPlanInput{
		Goal:           "check weather in tokyo",
		MaxConcurrency: 0,
		Tasks:          nil,
	})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}

	if len(plan.PlannerTasks) != 1 {
		t.Fatalf("plannerTasks = %d, want 1", len(plan.PlannerTasks))
	}
	if got := plan.PlannerTasks[0]; got.AgentID != "zeroclaw" || got.Input != "check weather in tokyo" {
		t.Fatalf("fallback planner task = %+v", got)
	}
	if len(plan.TaskUnits) != 1 {
		t.Fatalf("taskUnits = %d, want 1", len(plan.TaskUnits))
	}
	if got := plan.TaskUnits[0]; got.HostID != LocalHostID || got.AgentID != "zeroclaw" {
		t.Fatalf("fallback task unit = %+v, want local/zeroclaw", got)
	}
	if plan.MaxConcurrency != 1 {
		t.Fatalf("maxConcurrency = %d, want 1", plan.MaxConcurrency)
	}
}

func TestBuildPlanRejectsEmptyGoal(t *testing.T) {
	if _, err := BuildPlan(BuildPlanInput{}); err == nil {
		t.Fatal("expected empty goal error")
	}
}
