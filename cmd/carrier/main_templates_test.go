package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseCarrierCommandRoutesTemplates(t *testing.T) {
	cmd, args, err := parseCarrierCommand([]string{"carrier", "templates", "show", "pr-triage"})
	if err != nil {
		t.Fatalf("parseCarrierCommand(templates) error: %v", err)
	}
	if cmd != "templates" {
		t.Fatalf("command = %q, want templates", cmd)
	}
	if len(args) != 2 || args[0] != "show" || args[1] != "pr-triage" {
		t.Fatalf("args = %v, want [show pr-triage]", args)
	}
}

func TestParseTemplatesCommandArgs(t *testing.T) {
	listOpts, err := parseTemplatesCommandArgs([]string{"list", "--json"})
	if err != nil {
		t.Fatalf("parseTemplatesCommandArgs(list) error: %v", err)
	}
	if listOpts.Action != "list" || !listOpts.JSON {
		t.Fatalf("unexpected list opts: %+v", listOpts)
	}

	showOpts, err := parseTemplatesCommandArgs([]string{"show", "incident-diagnosis"})
	if err != nil {
		t.Fatalf("parseTemplatesCommandArgs(show) error: %v", err)
	}
	if showOpts.Action != "show" || showOpts.TemplateID != "incident-diagnosis" {
		t.Fatalf("unexpected show opts: %+v", showOpts)
	}

	runOpts, err := parseTemplatesCommandArgs([]string{
		"run", "pr-triage",
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
		t.Fatalf("parseTemplatesCommandArgs(run) error: %v", err)
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
}

func TestRunTemplatesCommand(t *testing.T) {
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
	if err := runTemplatesCommand(&out, templatesCommandOptions{Action: "list"}); err != nil {
		t.Fatalf("runTemplatesCommand(list) error: %v", err)
	}
	if !strings.Contains(out.String(), "incident-diagnosis") {
		t.Fatalf("list output=%s, want incident-diagnosis", out.String())
	}

	out.Reset()
	if err := runTemplatesCommand(&out, templatesCommandOptions{Action: "show", TemplateID: "incident-diagnosis"}); err != nil {
		t.Fatalf("runTemplatesCommand(show) error: %v", err)
	}
	if !strings.Contains(out.String(), "Incident Diagnosis") || !strings.Contains(out.String(), "service") {
		t.Fatalf("show output=%s, want template details", out.String())
	}

	out.Reset()
	if err := runTemplatesCommand(&out, templatesCommandOptions{
		Action:         "run",
		TemplateID:     "incident-diagnosis",
		Inputs:         map[string]string{"service": "checkout"},
		Provider:       "openrouter",
		MaxConcurrency: 2,
		PolicyApprove:  true,
	}); err != nil {
		t.Fatalf("runTemplatesCommand(run) error: %v", err)
	}
	if !strings.Contains(out.String(), "exec-template-1") || !strings.Contains(out.String(), "carrier executions show exec-template-1") {
		t.Fatalf("run output=%s, want execution id and next step", out.String())
	}
}
