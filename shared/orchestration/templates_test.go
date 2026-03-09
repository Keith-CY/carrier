package orchestration

import "testing"

func TestListExecutionTemplatesIncludesBuiltins(t *testing.T) {
	templates := ListExecutionTemplates()
	if len(templates) < 4 {
		t.Fatalf("templates len=%d, want at least 4", len(templates))
	}
	wantIDs := []string{
		"pr-triage",
		"issue-investigation",
		"incident-diagnosis",
		"rollout-smoke-check",
	}
	seen := map[string]bool{}
	for _, template := range templates {
		seen[template.ID] = true
		if template.ID == "" || template.Name == "" || template.DefaultGoalTemplate == "" {
			t.Fatalf("template missing required metadata: %+v", template)
		}
	}
	for _, wantID := range wantIDs {
		if !seen[wantID] {
			t.Fatalf("missing template %q in %+v", wantID, templates)
		}
	}
}

func TestResolveExecutionTemplateRendersGoalAndTasks(t *testing.T) {
	resolved, err := ResolveExecutionTemplate("incident-diagnosis", map[string]string{
		"service":         "checkout",
		"environment":     "prod",
		"incidentSummary": "latency regression after deploy",
	})
	if err != nil {
		t.Fatalf("ResolveExecutionTemplate() error = %v", err)
	}
	if resolved.Template.ID != "incident-diagnosis" {
		t.Fatalf("template id = %q, want incident-diagnosis", resolved.Template.ID)
	}
	if resolved.Goal == "" || resolved.Goal == resolved.Template.DefaultGoalTemplate {
		t.Fatalf("goal was not rendered: %q", resolved.Goal)
	}
	if len(resolved.Tasks) != 3 {
		t.Fatalf("tasks len=%d, want 3", len(resolved.Tasks))
	}
	if resolved.Tasks[0].AgentID != "zeroclaw" {
		t.Fatalf("tasks[0].agentId=%q, want zeroclaw", resolved.Tasks[0].AgentID)
	}
	if resolved.Tasks[1].AgentID != "picoclaw" {
		t.Fatalf("tasks[1].agentId=%q, want picoclaw", resolved.Tasks[1].AgentID)
	}
	if resolved.Tasks[2].AgentID != "zeroclaw" {
		t.Fatalf("tasks[2].agentId=%q, want zeroclaw", resolved.Tasks[2].AgentID)
	}
	if got := resolved.Inputs["service"]; got != "checkout" {
		t.Fatalf("inputs[service]=%q, want checkout", got)
	}
}

func TestResolveExecutionTemplateRejectsMissingRequiredInputs(t *testing.T) {
	if _, err := ResolveExecutionTemplate("rollout-smoke-check", map[string]string{
		"service": "carrier-api",
	}); err == nil {
		t.Fatal("expected missing required input error")
	}
}

func TestBuildPlanPreservesTemplateID(t *testing.T) {
	resolved, err := ResolveExecutionTemplate("pr-triage", map[string]string{
		"repository": "Keith-CY/carrier",
		"prNumber":   "1550",
		"focus":      "conflicts and rollout risk",
	})
	if err != nil {
		t.Fatalf("ResolveExecutionTemplate() error = %v", err)
	}
	plan, err := BuildPlan(BuildPlanInput{
		Goal:           resolved.Goal,
		TemplateID:     resolved.Template.ID,
		MaxConcurrency: 8,
		Tasks:          resolved.Tasks,
	})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if plan.TemplateID != "pr-triage" {
		t.Fatalf("templateId = %q, want pr-triage", plan.TemplateID)
	}
	if len(plan.TaskUnits) != 3 {
		t.Fatalf("taskUnits len=%d, want 3", len(plan.TaskUnits))
	}
}
