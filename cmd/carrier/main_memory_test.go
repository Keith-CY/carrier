package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseCarrierCommandRoutesMemory(t *testing.T) {
	cmd, args, err := parseCarrierCommand([]string{"carrier", "memory", "list"})
	if err != nil {
		t.Fatalf("parseCarrierCommand(memory) error: %v", err)
	}
	if cmd != "memory" {
		t.Fatalf("command=%q want memory", cmd)
	}
	if len(args) != 1 || args[0] != "list" {
		t.Fatalf("args=%v want [list]", args)
	}
}

func TestParseMemoryCommandArgs(t *testing.T) {
	listOpts, err := parseMemoryCommandArgs([]string{"list", "--subject", "agent-a", "--json"})
	if err != nil {
		t.Fatalf("parseMemoryCommandArgs(list) error: %v", err)
	}
	if listOpts.Action != "list" || listOpts.Subject != "agent-a" || !listOpts.JSON {
		t.Fatalf("unexpected list opts: %+v", listOpts)
	}

	searchOpts, err := parseMemoryCommandArgs([]string{"search", "--subject", "agent-a", "--query", "fusion", "--limit", "7", "--min-score", "0.6"})
	if err != nil {
		t.Fatalf("parseMemoryCommandArgs(search) error: %v", err)
	}
	if searchOpts.Action != "search" || searchOpts.Subject != "agent-a" || searchOpts.Query != "fusion" || searchOpts.Limit != 7 || searchOpts.MinScore != 0.6 {
		t.Fatalf("unexpected search opts: %+v", searchOpts)
	}

	attachOpts, err := parseMemoryCommandArgs([]string{"attach", "--instance", "picoclaw-main", "--scope", "shared:profile"})
	if err != nil {
		t.Fatalf("parseMemoryCommandArgs(attach) error: %v", err)
	}
	if attachOpts.Action != "attach" || attachOpts.InstanceID != "picoclaw-main" || attachOpts.Scope != "shared:profile" {
		t.Fatalf("unexpected attach opts: %+v", attachOpts)
	}

	distillOpts, err := parseMemoryCommandArgs([]string{"distill", "--instance", "picoclaw-main", "--dry-run", "--force", "--reason", "promote learnings"})
	if err != nil {
		t.Fatalf("parseMemoryCommandArgs(distill) error: %v", err)
	}
	if distillOpts.Action != "distill" || distillOpts.InstanceID != "picoclaw-main" || !distillOpts.DryRun || !distillOpts.Force || distillOpts.Reason != "promote learnings" {
		t.Fatalf("unexpected distill opts: %+v", distillOpts)
	}
}

func TestRunMemoryCommand(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/v1/memory":
			_, _ = w.Write([]byte(`{"result":"ok","entries":[{"id":"public.team.v1","type":"public"},{"id":"agent-a.private.v1","type":"per_agent"}],"attachments":[{"agent_id":"agent-a","memory_id":"public.team.v1"}],"grants":[{"id":"grant-1","subject":"agent-a","scope":"shared:profile"}],"audit":[]}`))
		case "/api/v1/memory/search":
			_, _ = w.Write([]byte(`{"result":"ok","results":[{"id":"rec-1","scope":"agent:agent-a","score":0.95,"snippet":"fusion memory"}]}`))
		case "/api/v1/memory/instance/attach":
			_, _ = w.Write([]byte(`{"result":"ok","status":"attached"}`))
		case "/api/v1/memory/instance/detach":
			_, _ = w.Write([]byte(`{"result":"ok","status":"detached"}`))
		case "/api/v1/memory/instance/distill":
			_, _ = w.Write([]byte(`{"result":"ok","result":{"runId":"distill-1","instanceId":"picoclaw-main","status":"dry_run","dryRun":true}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer gateway.Close()

	setProbeEnvFromURL(t, "CARRIER_GATEWAY_HOST", "CARRIER_GATEWAY_PORT", gateway.URL)

	var out bytes.Buffer
	if err := runMemoryCommand(&out, memoryCommandOptions{Action: "list", Subject: "agent-a"}); err != nil {
		t.Fatalf("runMemoryCommand(list) error: %v", err)
	}
	if !strings.Contains(out.String(), "public.team.v1") || !strings.Contains(out.String(), "shared:profile") {
		t.Fatalf("list output=%s", out.String())
	}

	out.Reset()
	if err := runMemoryCommand(&out, memoryCommandOptions{Action: "search", Subject: "agent-a", Query: "fusion"}); err != nil {
		t.Fatalf("runMemoryCommand(search) error: %v", err)
	}
	if !strings.Contains(out.String(), "rec-1") || !strings.Contains(out.String(), "fusion memory") {
		t.Fatalf("search output=%s", out.String())
	}

	out.Reset()
	if err := runMemoryCommand(&out, memoryCommandOptions{Action: "attach", InstanceID: "picoclaw-main", Scope: "shared:profile"}); err != nil {
		t.Fatalf("runMemoryCommand(attach) error: %v", err)
	}
	if !strings.Contains(out.String(), "attached shared:profile to picoclaw-main") {
		t.Fatalf("attach output=%s", out.String())
	}

	out.Reset()
	if err := runMemoryCommand(&out, memoryCommandOptions{Action: "detach", InstanceID: "picoclaw-main", Scope: "shared:profile"}); err != nil {
		t.Fatalf("runMemoryCommand(detach) error: %v", err)
	}
	if !strings.Contains(out.String(), "detached shared:profile from picoclaw-main") {
		t.Fatalf("detach output=%s", out.String())
	}

	out.Reset()
	if err := runMemoryCommand(&out, memoryCommandOptions{Action: "distill", InstanceID: "picoclaw-main", DryRun: true}); err != nil {
		t.Fatalf("runMemoryCommand(distill) error: %v", err)
	}
	if !strings.Contains(out.String(), "distill-1") || !strings.Contains(out.String(), "picoclaw-main") {
		t.Fatalf("distill output=%s", out.String())
	}
}
