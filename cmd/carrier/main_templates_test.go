package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseCarrierCommandRoutesTemplates(t *testing.T) {
	if _, _, err := parseCarrierCommand([]string{"carrier", "templates", "show", "pr-triage"}); err == nil || !strings.Contains(err.Error(), "unknown command: templates") {
		t.Fatalf("parseCarrierCommand(templates) error = %v, want unknown command", err)
	}
}

func TestParseOrchestrateTemplateCommandArgs(t *testing.T) {
	listOpts, err := parseOrchestrateCommandArgs([]string{"templates", "--json"})
	if err != nil {
		t.Fatalf("parseOrchestrateCommandArgs(list) error: %v", err)
	}
	if listOpts.Action != "templates_list" || !listOpts.JSON {
		t.Fatalf("unexpected list opts: %+v", listOpts)
	}

	showOpts, err := parseOrchestrateCommandArgs([]string{"templates", "show", "incident-diagnosis"})
	if err != nil {
		t.Fatalf("parseOrchestrateCommandArgs(show) error: %v", err)
	}
	if showOpts.Action != "templates_show" || showOpts.TemplateID != "incident-diagnosis" {
		t.Fatalf("unexpected show opts: %+v", showOpts)
	}

	runOpts, err := parseOrchestrateCommandArgs([]string{
		"--template", "pr-triage",
		"--input", "repository=Keith-CY/carrier",
		"--input", "prNumber=1554",
		"--input", "focus=rollback risk",
		"--host-id", "host-a",
		"--host-label", "prod",
		"--memory-scope", "shared:code-review",
		"--distill-scope", "shared:pr-lessons",
		"--provider", "openrouter",
		"--max-concurrency", "3",
		"--policy-approve",
		"--json",
	})
	if err != nil {
		t.Fatalf("parseOrchestrateCommandArgs(template run) error: %v", err)
	}
	if runOpts.Action != "run" || runOpts.TemplateID != "pr-triage" || runOpts.Provider != "openrouter" || runOpts.MaxConcurrency != 3 || !runOpts.PolicyApprove || !runOpts.JSON {
		t.Fatalf("unexpected run opts: %+v", runOpts)
	}
	if got := runOpts.Inputs["repository"]; got != "Keith-CY/carrier" {
		t.Fatalf("inputs[repository]=%q, want Keith-CY/carrier", got)
	}
	if len(runOpts.HostIDs) != 1 || runOpts.HostIDs[0] != "host-a" {
		t.Fatalf("hostIDs=%v, want [host-a]", runOpts.HostIDs)
	}
	if len(runOpts.HostLabels) != 1 || runOpts.HostLabels[0] != "prod" {
		t.Fatalf("hostLabels=%v, want [prod]", runOpts.HostLabels)
	}
	if len(runOpts.RequiredMemory) != 1 || runOpts.RequiredMemory[0] != "shared:code-review" {
		t.Fatalf("requiredMemory=%v, want [shared:code-review]", runOpts.RequiredMemory)
	}
	if len(runOpts.DistillOutputs) != 1 || runOpts.DistillOutputs[0] != "shared:pr-lessons" {
		t.Fatalf("distillOutputs=%v, want [shared:pr-lessons]", runOpts.DistillOutputs)
	}
	if runOpts.Goal != "" {
		t.Fatalf("goal = %q, want empty for template launch", runOpts.Goal)
	}
}

func TestRunOrchestrateTemplateCommands(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/v1/templates":
			_, _ = w.Write([]byte(`{"result":"ok","templates":[{"id":"incident-diagnosis","name":"Incident Diagnosis","description":"Diagnose incidents.","inputSchema":[{"id":"service","label":"Service","required":true}],"defaultGoalTemplate":"Diagnose {{service}}.","plannerTasks":[{"id":"task-1","agentId":"zeroclaw","inputTemplate":"Collect context"}]}]}`))
		case "/api/v1/templates/incident-diagnosis":
			_, _ = w.Write([]byte(`{"result":"ok","template":{"id":"incident-diagnosis","name":"Incident Diagnosis","description":"Diagnose incidents.","inputSchema":[{"id":"service","label":"Service","required":true}],"defaultGoalTemplate":"Diagnose {{service}}.","plannerTasks":[{"id":"task-1","agentId":"zeroclaw","inputTemplate":"Collect context"}]}}`))
		case "/api/v1/templates/incident-diagnosis/launch":
			if r.Method != http.MethodPost {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(`{"result":"ok","template":{"id":"incident-diagnosis","name":"Incident Diagnosis"},"execution":{"id":"exec-template-1","templateId":"incident-diagnosis","goal":"Diagnose checkout in prod","status":"provisioning","taskUnits":[{"id":"task-1","hostId":"local","agentId":"zeroclaw"}],"results":[]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer gateway.Close()

	setProbeEnvFromURL(t, "CARRIER_GATEWAY_HOST", "CARRIER_GATEWAY_PORT", gateway.URL)

	var out bytes.Buffer
	if err := runOrchestrateCommand(&out, orchestrateCommandOptions{Action: "templates_list"}); err != nil {
		t.Fatalf("runOrchestrateCommand(list) error: %v", err)
	}
	if !strings.Contains(out.String(), "incident-diagnosis") {
		t.Fatalf("list output=%s, want incident-diagnosis", out.String())
	}

	out.Reset()
	if err := runOrchestrateCommand(&out, orchestrateCommandOptions{Action: "templates_show", TemplateID: "incident-diagnosis"}); err != nil {
		t.Fatalf("runOrchestrateCommand(show) error: %v", err)
	}
	if !strings.Contains(out.String(), "Incident Diagnosis") || !strings.Contains(out.String(), "service") {
		t.Fatalf("show output=%s, want template details", out.String())
	}
}
