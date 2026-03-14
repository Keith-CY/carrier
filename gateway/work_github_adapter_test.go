package gateway

import (
	"carrier/shared/work"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleWorkGitHubImportCreatesWorkItem(t *testing.T) {
	t.Setenv("CARRIER_ROOT", t.TempDir())
	t.Setenv("CARRIER_APP_ROOT", "")
	t.Setenv("CARRIER_PROJECTS_ROOT", "")
	t.Setenv("CARRIER_WORKS_ROOT", "")

	project, err := upsertWorkProject(work.Project{
		ID:         "proj_123",
		Name:       "carrier",
		SourceType: work.SourceTypeLocal,
		SourceRef:  createGatewayTestGitRepo(t),
	})
	if err != nil {
		t.Fatalf("upsertWorkProject error: %v", err)
	}

	previous := runWorkGitHubCommand
	t.Cleanup(func() { runWorkGitHubCommand = previous })
	runWorkGitHubCommand = func(args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "repos/Keith-CY/carrier/issues/42") {
			t.Fatalf("unexpected gh args: %v", args)
		}
		return []byte(`{"title":"Imported issue","body":"Need a durable queue","labels":[{"name":"backend"},{"name":"workflow"}]}`), nil
	}

	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/work/adapters/github/import", strings.NewReader(`{"projectId":"`+project.ID+`","repository":"Keith-CY/carrier","issueNumber":42}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("import status=%d body=%s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Item work.WorkItem `json:"item"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode import response: %v", err)
	}
	if payload.Item.Source != work.WorkSourceGitHub {
		t.Fatalf("source=%q want github", payload.Item.Source)
	}
	if payload.Item.SourceRef != "github:Keith-CY/carrier/issues/42" {
		t.Fatalf("sourceRef=%q", payload.Item.SourceRef)
	}
	if payload.Item.Title != "Imported issue" {
		t.Fatalf("title=%q", payload.Item.Title)
	}
}

func TestHandleWorkGitHubPublishCreatesRecordAndUpdatesRun(t *testing.T) {
	t.Setenv("CARRIER_ROOT", t.TempDir())
	t.Setenv("CARRIER_APP_ROOT", "")
	t.Setenv("CARRIER_PROJECTS_ROOT", "")
	t.Setenv("CARRIER_WORKS_ROOT", "")

	project, err := upsertWorkProject(work.Project{
		ID:         "proj_123",
		Name:       "carrier",
		SourceType: work.SourceTypeLocal,
		SourceRef:  createGatewayTestGitRepo(t),
	})
	if err != nil {
		t.Fatalf("upsertWorkProject error: %v", err)
	}
	if _, err := syncWorkProject(project.ID); err != nil {
		t.Fatalf("syncWorkProject error: %v", err)
	}
	item, err := upsertWorkItem(work.WorkItem{
		ID:        "work_123",
		ProjectID: project.ID,
		Title:     "Imported issue",
		Source:    work.WorkSourceGitHub,
		SourceRef: "github:Keith-CY/carrier/issues/42",
	})
	if err != nil {
		t.Fatalf("upsertWorkItem error: %v", err)
	}
	run, err := startWorkRun(item.ID, work.RunBackendLocalSandboxed)
	if err != nil {
		t.Fatalf("startWorkRun error: %v", err)
	}

	previous := runWorkGitHubCommand
	t.Cleanup(func() { runWorkGitHubCommand = previous })
	runWorkGitHubCommand = func(args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "repos/Keith-CY/carrier/issues/42/comments") {
			t.Fatalf("unexpected gh args: %v", args)
		}
		return []byte(`{"html_url":"https://github.com/Keith-CY/carrier/issues/42#issuecomment-1"}`), nil
	}

	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/work/adapters/github/publish", strings.NewReader(`{"runId":"`+run.ID+`","targets":["comment","branch"]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("publish status=%d body=%s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Record workGitHubPublishRecord `json:"record"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode publish response: %v", err)
	}
	if len(payload.Record.Targets) != 2 {
		t.Fatalf("targets=%d want 2", len(payload.Record.Targets))
	}
	updatedRun, ok, err := getWorkRun(run.ID)
	if err != nil {
		t.Fatalf("getWorkRun error: %v", err)
	}
	if !ok {
		t.Fatal("expected run to exist")
	}
	if updatedRun.PublishStatus != work.PublishStatusPublished {
		t.Fatalf("publishStatus=%q want published", updatedRun.PublishStatus)
	}
	if updatedRun.Phase != work.RunPhaseCompleted {
		t.Fatalf("phase=%q want completed", updatedRun.Phase)
	}
	updatedItem, ok, err := getWorkItem(item.ID)
	if err != nil {
		t.Fatalf("getWorkItem error: %v", err)
	}
	if !ok {
		t.Fatal("expected item to exist")
	}
	if updatedItem.State != work.WorkItemStateAwaitingReview {
		t.Fatalf("item state=%q want awaiting_review", updatedItem.State)
	}
}
