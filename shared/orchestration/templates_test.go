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
		if template.Category == "" || template.Version == "" || template.SortOrder <= 0 {
			t.Fatalf("template missing preset metadata: %+v", template)
		}
		if !template.Featured {
			t.Fatalf("template should be featured: %+v", template)
		}
		if template.DefaultLaunchConfig.ApprovalScope == "" || template.DefaultLaunchConfig.MaxConcurrency <= 0 {
			t.Fatalf("template missing default launch config: %+v", template)
		}
		if len(template.RequiredMemory) == 0 {
			t.Fatalf("template missing requiredMemory: %+v", template)
		}
		if len(template.DistillOutputs) == 0 {
			t.Fatalf("template missing distillOutputs: %+v", template)
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
	if len(resolved.Template.RequiredMemory) == 0 || len(resolved.Template.DistillOutputs) == 0 {
		t.Fatalf("resolved template missing memory contract metadata: %+v", resolved.Template)
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
	if plan.TemplateVersion != "v1" {
		t.Fatalf("templateVersion = %q, want v1", plan.TemplateVersion)
	}
	if len(plan.RequiredMemory) == 0 {
		t.Fatalf("expected template-backed plan requiredMemory, got %+v", plan)
	}
	if len(plan.DistillOutputs) == 0 {
		t.Fatalf("expected template-backed plan distillOutputs, got %+v", plan)
	}
	if len(plan.TaskUnits) != 3 {
		t.Fatalf("taskUnits len=%d, want 3", len(plan.TaskUnits))
	}
}

func TestBuildPlanAppliesTemplateLaunchDefaults(t *testing.T) {
	resolved, err := ResolveExecutionTemplate("incident-diagnosis", map[string]string{
		"service":         "checkout",
		"environment":     "prod",
		"incidentSummary": "latency regression after deploy",
	})
	if err != nil {
		t.Fatalf("ResolveExecutionTemplate() error = %v", err)
	}
	plan, err := BuildPlan(BuildPlanInput{
		Goal:       resolved.Goal,
		TemplateID: resolved.Template.ID,
		Tasks:      resolved.Tasks,
	})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if plan.TemplateVersion != "v1" {
		t.Fatalf("templateVersion = %q, want v1", plan.TemplateVersion)
	}
	if plan.ApprovalScope != "infrastructure_only" {
		t.Fatalf("approvalScope = %q, want infrastructure_only", plan.ApprovalScope)
	}
	if plan.MaxConcurrency != resolved.Template.DefaultLaunchConfig.MaxConcurrency {
		t.Fatalf("maxConcurrency = %d, want %d", plan.MaxConcurrency, resolved.Template.DefaultLaunchConfig.MaxConcurrency)
	}
	if len(resolved.Template.DefaultLaunchConfig.HostLabels) == 0 {
		t.Fatalf("expected template default host labels in fixture, got %+v", resolved.Template.DefaultLaunchConfig)
	}
	if got := len(plan.HostLabels); got != len(resolved.Template.DefaultLaunchConfig.HostLabels) {
		t.Fatalf("hostLabels len = %d, want %d", got, len(resolved.Template.DefaultLaunchConfig.HostLabels))
	}
}
