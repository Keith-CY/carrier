package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseCarrierCommandRoutesTriggers(t *testing.T) {
	cmd, args, err := parseCarrierCommand([]string{"carrier", "triggers", "show", "trigger-1"})
	if err != nil {
		t.Fatalf("parseCarrierCommand(triggers) error: %v", err)
	}
	if cmd != "triggers" {
		t.Fatalf("command = %q, want triggers", cmd)
	}
	if len(args) != 2 || args[0] != "show" || args[1] != "trigger-1" {
		t.Fatalf("args = %v, want [show trigger-1]", args)
	}
}

func TestParseTriggersCommandArgs(t *testing.T) {
	listOpts, err := parseTriggersCommandArgs([]string{"list", "--json"})
	if err != nil {
		t.Fatalf("parseTriggersCommandArgs(list) error: %v", err)
	}
	if listOpts.Action != "list" || !listOpts.JSON {
		t.Fatalf("unexpected list opts: %+v", listOpts)
	}

	showOpts, err := parseTriggersCommandArgs([]string{"show", "trigger-1"})
	if err != nil {
		t.Fatalf("parseTriggersCommandArgs(show) error: %v", err)
	}
	if showOpts.Action != "show" || showOpts.TriggerID != "trigger-1" {
		t.Fatalf("unexpected show opts: %+v", showOpts)
	}

	createOpts, err := parseTriggersCommandArgs([]string{
		"create",
		"--type", "github",
		"--template-id", "pr-triage",
		"--name", "pr triage comment",
		"--webhook-secret", "secret-1",
		"--github-command", "/carrier triage",
		"--github-repository", "Keith-CY/carrier",
		"--host-id", "host-1",
		"--host-label", "prod",
		"--memory-scope", "shared:code-review",
		"--distill-scope", "shared:pr-lessons",
		"--input", "repository={{payload.repository.full_name}}",
		"--input", "prNumber={{payload.issue.number}}",
		"--policy-approve",
		"--json",
	})
	if err != nil {
		t.Fatalf("parseTriggersCommandArgs(create) error: %v", err)
	}
	if createOpts.Action != "create" || createOpts.Type != "github" || createOpts.TemplateID != "pr-triage" || createOpts.Name != "pr triage comment" || createOpts.WebhookSecret != "secret-1" || !createOpts.PolicyApprove || !createOpts.JSON {
		t.Fatalf("unexpected create opts: %+v", createOpts)
	}
	if len(createOpts.HostIDs) != 1 || createOpts.HostIDs[0] != "host-1" {
		t.Fatalf("hostIDs=%v, want [host-1]", createOpts.HostIDs)
	}
	if len(createOpts.HostLabels) != 1 || createOpts.HostLabels[0] != "prod" {
		t.Fatalf("hostLabels=%v, want [prod]", createOpts.HostLabels)
	}
	if len(createOpts.RequiredMemory) != 1 || createOpts.RequiredMemory[0] != "shared:code-review" {
		t.Fatalf("requiredMemory=%v, want [shared:code-review]", createOpts.RequiredMemory)
	}
	if len(createOpts.DistillOutputs) != 1 || createOpts.DistillOutputs[0] != "shared:pr-lessons" {
		t.Fatalf("distillOutputs=%v, want [shared:pr-lessons]", createOpts.DistillOutputs)
	}
	if got := createOpts.Inputs["repository"]; got != "{{payload.repository.full_name}}" {
		t.Fatalf("inputs[repository]=%q", got)
	}

	updateOpts, err := parseTriggersCommandArgs([]string{"update", "trigger-1", "--disable", "--cron", "0 * * * *"})
	if err != nil {
		t.Fatalf("parseTriggersCommandArgs(update) error: %v", err)
	}
	if updateOpts.Action != "update" || updateOpts.TriggerID != "trigger-1" || !updateOpts.Disable || updateOpts.Cron != "0 * * * *" {
		t.Fatalf("unexpected update opts: %+v", updateOpts)
	}
}

func TestRunTriggersCommand(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/v1/triggers":
			switch r.Method {
			case http.MethodGet:
				_, _ = w.Write([]byte(`{"result":"ok","triggers":[{"id":"trigger-1","name":"incident webhook","type":"webhook","templateId":"incident-diagnosis","enabled":true,"config":{"hostIds":["host-1"],"inputs":{"service":"{{payload.service}}"}}}]}`))
			case http.MethodPost:
				_, _ = w.Write([]byte(`{"result":"ok","trigger":{"id":"trigger-2","name":"nightly smoke","type":"schedule","templateId":"rollout-smoke-check","enabled":true,"config":{"cron":"0 * * * *","timezone":"UTC"}}}`))
			default:
				http.NotFound(w, r)
			}
		case "/api/v1/triggers/trigger-1":
			switch r.Method {
			case http.MethodGet:
				_, _ = w.Write([]byte(`{"result":"ok","trigger":{"id":"trigger-1","name":"incident webhook","type":"webhook","templateId":"incident-diagnosis","enabled":true,"config":{"hostIds":["host-1"],"inputs":{"service":"{{payload.service}}"}}}}`))
			case http.MethodPatch:
				_, _ = w.Write([]byte(`{"result":"ok","trigger":{"id":"trigger-1","name":"incident webhook","type":"webhook","templateId":"incident-diagnosis","enabled":false,"config":{"hostIds":["host-1"],"inputs":{"service":"{{payload.service}}"}}}}`))
			case http.MethodDelete:
				_, _ = w.Write([]byte(`{"result":"ok","deleted":true}`))
			default:
				http.NotFound(w, r)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer gateway.Close()

	setProbeEnvFromURL(t, "CARRIER_GATEWAY_HOST", "CARRIER_GATEWAY_PORT", gateway.URL)

	var out bytes.Buffer
	if err := runTriggersCommand(&out, triggersCommandOptions{Action: "list"}); err != nil {
		t.Fatalf("runTriggersCommand(list) error: %v", err)
	}
	if !strings.Contains(out.String(), "trigger-1") || !strings.Contains(out.String(), "incident-diagnosis") {
		t.Fatalf("list output=%s", out.String())
	}

	out.Reset()
	if err := runTriggersCommand(&out, triggersCommandOptions{Action: "show", TriggerID: "trigger-1"}); err != nil {
		t.Fatalf("runTriggersCommand(show) error: %v", err)
	}
	if !strings.Contains(out.String(), "incident webhook") || !strings.Contains(out.String(), "service={{payload.service}}") {
		t.Fatalf("show output=%s", out.String())
	}

	out.Reset()
	if err := runTriggersCommand(&out, triggersCommandOptions{
		Action:        "create",
		Type:          "schedule",
		TemplateID:    "rollout-smoke-check",
		Name:          "nightly smoke",
		Cron:          "0 * * * *",
		Timezone:      "UTC",
		PolicyApprove: true,
	}); err != nil {
		t.Fatalf("runTriggersCommand(create) error: %v", err)
	}
	if !strings.Contains(out.String(), "trigger-2") || !strings.Contains(out.String(), "nightly smoke") {
		t.Fatalf("create output=%s", out.String())
	}

	out.Reset()
	if err := runTriggersCommand(&out, triggersCommandOptions{Action: "update", TriggerID: "trigger-1", Disable: true}); err != nil {
		t.Fatalf("runTriggersCommand(update) error: %v", err)
	}
	if !strings.Contains(out.String(), "enabled=false") {
		t.Fatalf("update output=%s", out.String())
	}

	out.Reset()
	if err := runTriggersCommand(&out, triggersCommandOptions{Action: "delete", TriggerID: "trigger-1"}); err != nil {
		t.Fatalf("runTriggersCommand(delete) error: %v", err)
	}
	if !strings.Contains(out.String(), "deleted trigger trigger-1") {
		t.Fatalf("delete output=%s", out.String())
	}
}
