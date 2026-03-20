package orchestration

import (
	"fmt"
	"sort"
	"strings"
)

type TemplateInputField struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Description  string `json:"description,omitempty"`
	Placeholder  string `json:"placeholder,omitempty"`
	Required     bool   `json:"required,omitempty"`
	DefaultValue string `json:"defaultValue,omitempty"`
}

type TemplateWorkerHint struct {
	AgentID     string   `json:"agentId,omitempty"`
	HostLabels  []string `json:"hostLabels,omitempty"`
	Description string   `json:"description,omitempty"`
}

type TemplateTask struct {
	ID            string `json:"id"`
	AgentID       string `json:"agentId,omitempty"`
	InputTemplate string `json:"inputTemplate"`
}

type TemplateDefaultLaunchConfig struct {
	Provider       string   `json:"provider,omitempty"`
	HostLabels     []string `json:"hostLabels,omitempty"`
	MaxConcurrency int      `json:"maxConcurrency,omitempty"`
	ApprovalScope  string   `json:"approvalScope,omitempty"`
}

type ExecutionTemplate struct {
	ID                  string                      `json:"id"`
	Name                string                      `json:"name"`
	Category            string                      `json:"category,omitempty"`
	SortOrder           int                         `json:"sortOrder,omitempty"`
	Featured            bool                        `json:"featured,omitempty"`
	Version             string                      `json:"version,omitempty"`
	Description         string                      `json:"description,omitempty"`
	InputSchema         []TemplateInputField        `json:"inputSchema,omitempty"`
	DefaultLaunchConfig TemplateDefaultLaunchConfig `json:"defaultLaunchConfig,omitempty"`
	DefaultGoalTemplate string                      `json:"defaultGoalTemplate"`
	DefaultPolicyHints  []string                    `json:"defaultPolicyHints,omitempty"`
	DefaultWorkerHints  []TemplateWorkerHint        `json:"defaultWorkerHints,omitempty"`
	DefaultOutputSchema map[string]interface{}      `json:"defaultOutputSchema,omitempty"`
	RequiredMemory      []string                    `json:"requiredMemory,omitempty"`
	DistillOutputs      []string                    `json:"distillOutputs,omitempty"`
	PlannerTasks        []TemplateTask              `json:"plannerTasks,omitempty"`
}

type ResolvedExecutionTemplate struct {
	Template ExecutionTemplate `json:"template"`
	Goal     string            `json:"goal"`
	Inputs   map[string]string `json:"inputs"`
	Tasks    []DecomposeTask   `json:"tasks"`
}

func ListExecutionTemplates() []ExecutionTemplate {
	out := make([]ExecutionTemplate, 0, len(builtinExecutionTemplates))
	for _, template := range builtinExecutionTemplates {
		out = append(out, cloneExecutionTemplate(template))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Featured != out[j].Featured {
			return out[i].Featured
		}
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func GetExecutionTemplate(templateID string) (ExecutionTemplate, bool) {
	key := strings.ToLower(strings.TrimSpace(templateID))
	template, ok := builtinExecutionTemplates[key]
	if !ok {
		return ExecutionTemplate{}, false
	}
	return cloneExecutionTemplate(template), true
}

func ResolveExecutionTemplate(templateID string, rawInputs map[string]string) (ResolvedExecutionTemplate, error) {
	template, ok := GetExecutionTemplate(templateID)
	if !ok {
		return ResolvedExecutionTemplate{}, fmt.Errorf("template %s not found", strings.TrimSpace(templateID))
	}
	inputs := normalizeTemplateInputs(template.InputSchema, rawInputs)
	for _, field := range template.InputSchema {
		if field.Required && strings.TrimSpace(inputs[field.ID]) == "" {
			return ResolvedExecutionTemplate{}, fmt.Errorf("template input %s is required", field.ID)
		}
	}
	goal := renderTemplateString(template.DefaultGoalTemplate, inputs)
	if strings.TrimSpace(goal) == "" {
		return ResolvedExecutionTemplate{}, fmt.Errorf("template %s rendered empty goal", template.ID)
	}
	tasks := make([]DecomposeTask, 0, len(template.PlannerTasks))
	for idx, task := range template.PlannerTasks {
		input := renderTemplateString(task.InputTemplate, inputs)
		if strings.TrimSpace(input) == "" {
			continue
		}
		taskID := strings.TrimSpace(task.ID)
		if taskID == "" {
			taskID = fmt.Sprintf("task-%d", idx+1)
		}
		tasks = append(tasks, DecomposeTask{
			ID:      taskID,
			Input:   input,
			AgentID: NormalizeAgentID(task.AgentID),
		})
	}
	if len(tasks) == 0 {
		tasks = FallbackTasks(goal)
	}
	return ResolvedExecutionTemplate{
		Template: template,
		Goal:     strings.TrimSpace(goal),
		Inputs:   inputs,
		Tasks:    tasks,
	}, nil
}

func normalizeTemplateInputs(schema []TemplateInputField, rawInputs map[string]string) map[string]string {
	out := make(map[string]string, len(schema))
	for _, field := range schema {
		key := strings.TrimSpace(field.ID)
		if key == "" {
			continue
		}
		value := ""
		if rawInputs != nil {
			value = strings.TrimSpace(rawInputs[key])
		}
		if value == "" {
			value = strings.TrimSpace(field.DefaultValue)
		}
		out[key] = value
	}
	return out
}

func renderTemplateString(text string, inputs map[string]string) string {
	rendered := strings.TrimSpace(text)
	for key, value := range inputs {
		rendered = strings.ReplaceAll(rendered, "{{"+key+"}}", strings.TrimSpace(value))
	}
	return strings.Join(strings.Fields(rendered), " ")
}

func cloneExecutionTemplate(template ExecutionTemplate) ExecutionTemplate {
	out := template
	out.InputSchema = append([]TemplateInputField(nil), template.InputSchema...)
	out.DefaultPolicyHints = append([]string(nil), template.DefaultPolicyHints...)
	out.DefaultWorkerHints = append([]TemplateWorkerHint(nil), template.DefaultWorkerHints...)
	out.DefaultLaunchConfig = TemplateDefaultLaunchConfig{
		Provider:       strings.TrimSpace(template.DefaultLaunchConfig.Provider),
		HostLabels:     append([]string(nil), template.DefaultLaunchConfig.HostLabels...),
		MaxConcurrency: template.DefaultLaunchConfig.MaxConcurrency,
		ApprovalScope:  strings.TrimSpace(template.DefaultLaunchConfig.ApprovalScope),
	}
	out.RequiredMemory = append([]string(nil), template.RequiredMemory...)
	out.DistillOutputs = append([]string(nil), template.DistillOutputs...)
	out.PlannerTasks = append([]TemplateTask(nil), template.PlannerTasks...)
	if template.DefaultOutputSchema != nil {
		out.DefaultOutputSchema = cloneTemplateMap(template.DefaultOutputSchema)
	}
	return out
}

func cloneTemplateMap(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for key, value := range in {
		switch typed := value.(type) {
		case map[string]interface{}:
			out[key] = cloneTemplateMap(typed)
		case []string:
			out[key] = append([]string(nil), typed...)
		case []interface{}:
			copied := make([]interface{}, len(typed))
			copy(copied, typed)
			out[key] = copied
		default:
			out[key] = typed
		}
	}
	return out
}

var builtinExecutionTemplates = map[string]ExecutionTemplate{
	"pr-triage": {
		ID:          "pr-triage",
		Name:        "PR Triage",
		Category:    "engineering",
		SortOrder:   10,
		Featured:    true,
		Version:     "v1",
		Description: "Collect pull request context, inspect risk, and draft a recommendation.",
		InputSchema: []TemplateInputField{
			{ID: "repository", Label: "Repository", Placeholder: "Keith-CY/carrier", Required: true},
			{ID: "prNumber", Label: "PR Number", Placeholder: "1554", Required: true},
			{ID: "focus", Label: "Focus", Placeholder: "conflicts, rollback risk", DefaultValue: "general risk assessment"},
		},
		DefaultLaunchConfig: TemplateDefaultLaunchConfig{
			MaxConcurrency: 2,
			ApprovalScope:  defaultApprovalScopeName,
		},
		DefaultGoalTemplate: "Triage pull request {{repository}}#{{prNumber}} with focus on {{focus}}.",
		DefaultPolicyHints: []string{
			"Prefer read-only tools unless a follow-up patch is explicitly approved.",
			"Require policy approval before production-affecting remediation.",
		},
		DefaultWorkerHints: []TemplateWorkerHint{
			{AgentID: "zeroclaw", Description: "Collect context and summarize findings."},
			{AgentID: "picoclaw", Description: "Inspect changed files and risk hotspots."},
		},
		RequiredMemory: []string{"shared:code-review", "shared:pull-requests"},
		DistillOutputs: []string{"shared:pr-lessons"},
		DefaultOutputSchema: map[string]interface{}{
			"summary":     "string",
			"risks":       []interface{}{"string"},
			"nextActions": []interface{}{"string"},
		},
		PlannerTasks: []TemplateTask{
			{ID: "task-1", AgentID: "zeroclaw", InputTemplate: "Collect PR context for {{repository}}#{{prNumber}}."},
			{ID: "task-2", AgentID: "picoclaw", InputTemplate: "Inspect changed files and risk hotspots for {{repository}}#{{prNumber}} with focus on {{focus}}."},
			{ID: "task-3", AgentID: "zeroclaw", InputTemplate: "Draft a triage recommendation for {{repository}}#{{prNumber}}."},
		},
	},
	"issue-investigation": {
		ID:          "issue-investigation",
		Name:        "Issue Investigation",
		Category:    "engineering",
		SortOrder:   20,
		Featured:    true,
		Version:     "v1",
		Description: "Investigate an issue report and produce findings plus next steps.",
		InputSchema: []TemplateInputField{
			{ID: "repository", Label: "Repository", Placeholder: "Keith-CY/carrier", Required: true},
			{ID: "issueNumber", Label: "Issue Number", Placeholder: "1548", Required: true},
			{ID: "symptom", Label: "Observed Symptom", Placeholder: "provider launch failed", Required: true},
		},
		DefaultLaunchConfig: TemplateDefaultLaunchConfig{
			MaxConcurrency: 2,
			ApprovalScope:  defaultApprovalScopeName,
		},
		DefaultGoalTemplate: "Investigate issue {{repository}}#{{issueNumber}}. Primary symptom: {{symptom}}.",
		DefaultPolicyHints: []string{
			"Keep investigation read-only unless operator explicitly requests a patch plan.",
		},
		DefaultWorkerHints: []TemplateWorkerHint{
			{AgentID: "zeroclaw", Description: "Gather issue context and summarize the outcome."},
			{AgentID: "picoclaw", Description: "Trace likely code paths and regressions."},
		},
		RequiredMemory: []string{"shared:issue-triage", "shared:repository-context"},
		DistillOutputs: []string{"shared:issue-lessons"},
		DefaultOutputSchema: map[string]interface{}{
			"summary":         "string",
			"rootCauses":      []interface{}{"string"},
			"recommendations": []interface{}{"string"},
		},
		PlannerTasks: []TemplateTask{
			{ID: "task-1", AgentID: "zeroclaw", InputTemplate: "Collect issue context for {{repository}}#{{issueNumber}} and extract reproduction hints."},
			{ID: "task-2", AgentID: "picoclaw", InputTemplate: "Investigate likely root causes for symptom {{symptom}} in {{repository}}."},
			{ID: "task-3", AgentID: "zeroclaw", InputTemplate: "Draft findings and next steps for {{repository}}#{{issueNumber}}."},
		},
	},
	"incident-diagnosis": {
		ID:          "incident-diagnosis",
		Name:        "Incident Diagnosis",
		Category:    "operations",
		SortOrder:   30,
		Featured:    true,
		Version:     "v1",
		Description: "Triage a live incident and produce an operator-facing diagnosis summary.",
		InputSchema: []TemplateInputField{
			{ID: "service", Label: "Service", Placeholder: "checkout", Required: true},
			{ID: "environment", Label: "Environment", Placeholder: "prod", Required: true},
			{ID: "incidentSummary", Label: "Incident Summary", Placeholder: "latency regression after deploy", Required: true},
		},
		DefaultLaunchConfig: TemplateDefaultLaunchConfig{
			HostLabels:     []string{"prod"},
			MaxConcurrency: 3,
			ApprovalScope:  defaultApprovalScopeName,
		},
		DefaultGoalTemplate: "Diagnose incident for service {{service}} in {{environment}}. Summary: {{incidentSummary}}.",
		DefaultPolicyHints: []string{
			"Prefer log and metric inspection before any mitigation proposal.",
		},
		DefaultWorkerHints: []TemplateWorkerHint{
			{AgentID: "zeroclaw", Description: "Collect incident context and summarize output."},
			{AgentID: "picoclaw", Description: "Perform deeper failure-path analysis."},
		},
		RequiredMemory: []string{"shared:incident-response", "shared:service-catalog"},
		DistillOutputs: []string{"shared:incident-lessons"},
		DefaultOutputSchema: map[string]interface{}{
			"summary":     "string",
			"findings":    []interface{}{"string"},
			"nextActions": []interface{}{"string"},
		},
		PlannerTasks: []TemplateTask{
			{ID: "task-1", AgentID: "zeroclaw", InputTemplate: "Collect incident context for {{service}} in {{environment}}."},
			{ID: "task-2", AgentID: "picoclaw", InputTemplate: "Analyze probable failure paths for {{service}} in {{environment}} given {{incidentSummary}}."},
			{ID: "task-3", AgentID: "zeroclaw", InputTemplate: "Draft diagnosis summary and operator next steps for {{service}}."},
		},
	},
	"rollout-smoke-check": {
		ID:          "rollout-smoke-check",
		Name:        "Rollout Smoke Check",
		Category:    "operations",
		SortOrder:   40,
		Featured:    true,
		Version:     "v1",
		Description: "Run a rollout-focused smoke checklist and summarize risk before promotion.",
		InputSchema: []TemplateInputField{
			{ID: "service", Label: "Service", Placeholder: "carrier-api", Required: true},
			{ID: "environment", Label: "Environment", Placeholder: "prod", Required: true},
			{ID: "releaseVersion", Label: "Release Version", Placeholder: "2026.03.09", Required: true},
		},
		DefaultLaunchConfig: TemplateDefaultLaunchConfig{
			HostLabels:     []string{"prod"},
			MaxConcurrency: 3,
			ApprovalScope:  defaultApprovalScopeName,
		},
		DefaultGoalTemplate: "Run rollout smoke checks for {{service}} in {{environment}} for release {{releaseVersion}}.",
		DefaultPolicyHints: []string{
			"Keep rollout verification read-only.",
		},
		DefaultWorkerHints: []TemplateWorkerHint{
			{AgentID: "zeroclaw", Description: "Collect rollout signals and summarize outcome."},
			{AgentID: "picoclaw", Description: "Inspect anomalies and regression clues."},
		},
		RequiredMemory: []string{"shared:release-runbook", "shared:service-catalog"},
		DistillOutputs: []string{"shared:rollout-lessons"},
		DefaultOutputSchema: map[string]interface{}{
			"summary":       "string",
			"checks":        []interface{}{"string"},
			"promotionRisk": "string",
		},
		PlannerTasks: []TemplateTask{
			{ID: "task-1", AgentID: "zeroclaw", InputTemplate: "Collect rollout health signals for {{service}} in {{environment}} for release {{releaseVersion}}."},
			{ID: "task-2", AgentID: "picoclaw", InputTemplate: "Inspect anomalies and regression indicators for {{service}} release {{releaseVersion}}."},
			{ID: "task-3", AgentID: "zeroclaw", InputTemplate: "Draft smoke-check summary and promotion recommendation for {{service}} in {{environment}}."},
		},
	},
}
