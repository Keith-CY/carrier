package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseCarrierCommandRoutesWork(t *testing.T) {
	cmd, args, err := parseCarrierCommand([]string{"carrier", "work", "items", "show", "work_123"})
	if err != nil {
		t.Fatalf("parseCarrierCommand(work) error: %v", err)
	}
	if cmd != "work" {
		t.Fatalf("command=%q want work", cmd)
	}
	if len(args) != 3 || args[0] != "items" || args[1] != "show" || args[2] != "work_123" {
		t.Fatalf("args=%v want [items show work_123]", args)
	}
}

func TestParseWorkCommandArgs(t *testing.T) {
	projectAdd, err := parseWorkCommandArgs([]string{"projects", "add", "--name", "carrier", "--source-type", "local", "--source-ref", "/tmp/repo", "--json"})
	if err != nil {
		t.Fatalf("parseWorkCommandArgs(project add) error: %v", err)
	}
	if projectAdd.Action != "projects-add" || projectAdd.Name != "carrier" || projectAdd.SourceType != "local" || projectAdd.SourceRef != "/tmp/repo" || !projectAdd.JSON {
		t.Fatalf("unexpected project opts: %+v", projectAdd)
	}

	itemCreate, err := parseWorkCommandArgs([]string{"items", "create", "--project", "proj_123", "--title", "Implement queue", "--description", "Need durable work queue", "--acceptance", "tests pass", "--priority", "high", "--label", "backend", "--json"})
	if err != nil {
		t.Fatalf("parseWorkCommandArgs(item create) error: %v", err)
	}
	if itemCreate.Action != "items-create" || itemCreate.ProjectID != "proj_123" || itemCreate.Title != "Implement queue" || itemCreate.Priority != "high" || len(itemCreate.Acceptance) != 1 || len(itemCreate.Labels) != 1 || !itemCreate.JSON {
		t.Fatalf("unexpected item create opts: %+v", itemCreate)
	}

	runStart, err := parseWorkCommandArgs([]string{"runs", "start", "work_123", "--backend", "managed_isolated", "--json"})
	if err != nil {
		t.Fatalf("parseWorkCommandArgs(run start) error: %v", err)
	}
	if runStart.Action != "runs-start" || runStart.WorkItemID != "work_123" || runStart.Backend != "managed_isolated" || !runStart.JSON {
		t.Fatalf("unexpected run start opts: %+v", runStart)
	}

	ghImport, err := parseWorkCommandArgs([]string{"github", "import", "--project", "proj_123", "--repository", "Keith-CY/carrier", "--issue", "42", "--json"})
	if err != nil {
		t.Fatalf("parseWorkCommandArgs(github import) error: %v", err)
	}
	if ghImport.Action != "github-import" || ghImport.ProjectID != "proj_123" || ghImport.Repository != "Keith-CY/carrier" || ghImport.IssueNumber != 42 || !ghImport.JSON {
		t.Fatalf("unexpected github import opts: %+v", ghImport)
	}

	ghPublish, err := parseWorkCommandArgs([]string{"github", "publish", "run_123", "--target", "comment", "--target", "status_note", "--json"})
	if err != nil {
		t.Fatalf("parseWorkCommandArgs(github publish) error: %v", err)
	}
	if ghPublish.Action != "github-publish" || ghPublish.RunID != "run_123" || len(ghPublish.PublishTargets) != 2 || !ghPublish.JSON {
		t.Fatalf("unexpected github publish opts: %+v", ghPublish)
	}
}

func TestRunWorkCommand(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/v1/work/projects":
			switch r.Method {
			case http.MethodGet:
				_, _ = w.Write([]byte(`{"result":"ok","projects":[{"id":"proj_123","name":"carrier","sourceType":"local","sourceRef":"/tmp/repo","defaultBranch":"main","workflowPath":"WORKFLOW.md","state":"ready","workflowDigest":"sha256:abc"}]}`))
			case http.MethodPost:
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				if body["name"] != "carrier" {
					t.Fatalf("project name=%v want carrier", body["name"])
				}
				_, _ = w.Write([]byte(`{"result":"ok","project":{"id":"proj_123","name":"carrier","sourceType":"local","sourceRef":"/tmp/repo","defaultBranch":"main","workflowPath":"WORKFLOW.md","state":"registered"}}`))
			default:
				http.NotFound(w, r)
			}
		case "/api/v1/work/items":
			switch r.Method {
			case http.MethodGet:
				_, _ = w.Write([]byte(`{"result":"ok","items":[{"id":"work_123","projectId":"proj_123","title":"Implement queue","priority":"normal","source":"local","sourceRef":"local:manual","state":"queued"}]}`))
			case http.MethodPost:
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				if body["projectId"] != "proj_123" {
					t.Fatalf("projectId=%v want proj_123", body["projectId"])
				}
				_, _ = w.Write([]byte(`{"result":"ok","item":{"id":"work_123","projectId":"proj_123","title":"Implement queue","priority":"high","source":"local","sourceRef":"local:manual","state":"new"}}`))
			default:
				http.NotFound(w, r)
			}
		case "/api/v1/work/items/work_123/runs":
			if r.Method != http.MethodPost {
				http.NotFound(w, r)
				return
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["backend"] != "local_sandboxed" {
				t.Fatalf("backend=%v want local_sandboxed", body["backend"])
			}
			_, _ = w.Write([]byte(`{"result":"ok","run":{"id":"run_123","projectId":"proj_123","workItemId":"work_123","executionId":"exec_123","workspaceId":"ws_123","workspacePath":"/tmp/workspace","backend":"local_sandboxed","phase":"executing","verificationStatus":"pending","publishStatus":"pending","workflowDigest":"sha256:abc"}}`))
		case "/api/v1/work/adapters/github/import":
			if r.Method != http.MethodPost {
				http.NotFound(w, r)
				return
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["repository"] != "Keith-CY/carrier" {
				t.Fatalf("repository=%v want Keith-CY/carrier", body["repository"])
			}
			_, _ = w.Write([]byte(`{"result":"ok","item":{"id":"work_456","projectId":"proj_123","title":"Imported issue","priority":"normal","source":"github","sourceRef":"github:Keith-CY/carrier/issues/42","state":"new"}}`))
		case "/api/v1/work/adapters/github/publish":
			if r.Method != http.MethodPost {
				http.NotFound(w, r)
				return
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			targets, _ := body["targets"].([]any)
			if len(targets) != 2 {
				t.Fatalf("targets=%v want len 2", body["targets"])
			}
			_, _ = w.Write([]byte(`{"result":"ok","record":{"id":"publish_123","projectId":"proj_123","workItemId":"work_456","runId":"run_123","repository":"Keith-CY/carrier","sourceRef":"github:Keith-CY/carrier/issues/42","createdAt":"2026-03-14T00:00:00Z","targets":[{"target":"comment","status":"published"},{"target":"status_note","status":"published"}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer gateway.Close()

	setProbeEnvFromURL(t, "CARRIER_GATEWAY_HOST", "CARRIER_GATEWAY_PORT", gateway.URL)

	var out bytes.Buffer
	if err := runWorkCommand(&out, workCommandOptions{Action: "projects-list"}); err != nil {
		t.Fatalf("runWorkCommand(projects list) error: %v", err)
	}
	if !strings.Contains(out.String(), "proj_123") {
		t.Fatalf("projects list output=%s", out.String())
	}

	out.Reset()
	if err := runWorkCommand(&out, workCommandOptions{Action: "items-create", ProjectID: "proj_123", Title: "Implement queue", Priority: "high"}); err != nil {
		t.Fatalf("runWorkCommand(items create) error: %v", err)
	}
	if !strings.Contains(out.String(), "work_item=work_123") {
		t.Fatalf("items create output=%s", out.String())
	}

	out.Reset()
	if err := runWorkCommand(&out, workCommandOptions{Action: "runs-start", WorkItemID: "work_123", Backend: "local_sandboxed"}); err != nil {
		t.Fatalf("runWorkCommand(runs start) error: %v", err)
	}
	if !strings.Contains(out.String(), "execution=exec_123") {
		t.Fatalf("runs start output=%s", out.String())
	}

	out.Reset()
	if err := runWorkCommand(&out, workCommandOptions{Action: "github-import", ProjectID: "proj_123", Repository: "Keith-CY/carrier", IssueNumber: 42}); err != nil {
		t.Fatalf("runWorkCommand(github import) error: %v", err)
	}
	if !strings.Contains(out.String(), "github:Keith-CY/carrier/issues/42") {
		t.Fatalf("github import output=%s", out.String())
	}

	out.Reset()
	if err := runWorkCommand(&out, workCommandOptions{Action: "github-publish", RunID: "run_123", PublishTargets: []string{"comment", "status_note"}}); err != nil {
		t.Fatalf("runWorkCommand(github publish) error: %v", err)
	}
	if !strings.Contains(out.String(), "target[comment]=published") || !strings.Contains(out.String(), "target[status_note]=published") {
		t.Fatalf("github publish output=%s", out.String())
	}
}
