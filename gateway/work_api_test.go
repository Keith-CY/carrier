package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleWorkLifecycle(t *testing.T) {
	t.Setenv("CARRIER_ROOT", t.TempDir())
	t.Setenv("CARRIER_APP_ROOT", "")
	t.Setenv("CARRIER_PROJECTS_ROOT", "")
	t.Setenv("CARRIER_WORKS_ROOT", "")

	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	projectReq := httptest.NewRequest(http.MethodPost, "/api/v1/work/projects", strings.NewReader(`{
		"id":"proj_123",
		"name":"carrier",
		"sourceType":"local",
		"sourceRef":"`+createGatewayTestGitRepo(t)+`"
	}`))
	projectReq.Header.Set("Content-Type", "application/json")
	projectReq.Header.Set("Authorization", "Bearer test-gateway-token")
	projectResp := httptest.NewRecorder()
	mux.ServeHTTP(projectResp, projectReq)
	if projectResp.Code != http.StatusCreated {
		t.Fatalf("create project status=%d body=%s", projectResp.Code, projectResp.Body.String())
	}

	syncReq := httptest.NewRequest(http.MethodPost, "/api/v1/work/projects/proj_123/sync", nil)
	syncReq.Header.Set("Authorization", "Bearer test-gateway-token")
	syncResp := httptest.NewRecorder()
	mux.ServeHTTP(syncResp, syncReq)
	if syncResp.Code != http.StatusOK {
		t.Fatalf("sync project status=%d body=%s", syncResp.Code, syncResp.Body.String())
	}

	itemReq := httptest.NewRequest(http.MethodPost, "/api/v1/work/items", strings.NewReader(`{
		"id":"work_123",
		"projectId":"proj_123",
		"title":"Add queue"
	}`))
	itemReq.Header.Set("Content-Type", "application/json")
	itemReq.Header.Set("Authorization", "Bearer test-gateway-token")
	itemResp := httptest.NewRecorder()
	mux.ServeHTTP(itemResp, itemReq)
	if itemResp.Code != http.StatusCreated {
		t.Fatalf("create item status=%d body=%s", itemResp.Code, itemResp.Body.String())
	}

	runReq := httptest.NewRequest(http.MethodPost, "/api/v1/work/items/work_123/runs", strings.NewReader(`{"backend":"local_sandboxed"}`))
	runReq.Header.Set("Content-Type", "application/json")
	runReq.Header.Set("Authorization", "Bearer test-gateway-token")
	runResp := httptest.NewRecorder()
	mux.ServeHTTP(runResp, runReq)
	if runResp.Code != http.StatusCreated {
		t.Fatalf("start run status=%d body=%s", runResp.Code, runResp.Body.String())
	}

	var started struct {
		Run struct {
			ID          string `json:"id"`
			ExecutionID string `json:"executionId"`
		} `json:"run"`
	}
	if err := json.Unmarshal(runResp.Body.Bytes(), &started); err != nil {
		t.Fatalf("decode run: %v", err)
	}
	if started.Run.ID == "" || started.Run.ExecutionID == "" {
		t.Fatalf("unexpected run payload: %s", runResp.Body.String())
	}

	getRunReq := httptest.NewRequest(http.MethodGet, "/api/v1/work/runs/"+started.Run.ID, nil)
	getRunReq.Header.Set("Authorization", "Bearer test-gateway-token")
	getRunResp := httptest.NewRecorder()
	mux.ServeHTTP(getRunResp, getRunReq)
	if getRunResp.Code != http.StatusOK {
		t.Fatalf("get run status=%d body=%s", getRunResp.Code, getRunResp.Body.String())
	}

	resumeReq := httptest.NewRequest(http.MethodGet, "/api/v1/work/runs/"+started.Run.ID+"/resume", nil)
	resumeReq.Header.Set("Authorization", "Bearer test-gateway-token")
	resumeResp := httptest.NewRecorder()
	mux.ServeHTTP(resumeResp, resumeReq)
	if resumeResp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("resume run status=%d want %d body=%s", resumeResp.Code, http.StatusMethodNotAllowed, resumeResp.Body.String())
	}
}

func TestHandleWorkRunActionReturnsInternalErrorForUnexpectedStoreFailure(t *testing.T) {
	t.Setenv("CARRIER_ROOT", t.TempDir())
	t.Setenv("CARRIER_APP_ROOT", "")
	t.Setenv("CARRIER_PROJECTS_ROOT", "")
	t.Setenv("CARRIER_WORKS_ROOT", "")

	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	projectReq := httptest.NewRequest(http.MethodPost, "/api/v1/work/projects", strings.NewReader(`{
		"id":"proj_123",
		"name":"carrier",
		"sourceType":"local",
		"sourceRef":"`+createGatewayTestGitRepo(t)+`"
	}`))
	projectReq.Header.Set("Content-Type", "application/json")
	projectReq.Header.Set("Authorization", "Bearer test-gateway-token")
	projectResp := httptest.NewRecorder()
	mux.ServeHTTP(projectResp, projectReq)
	if projectResp.Code != http.StatusCreated {
		t.Fatalf("create project status=%d body=%s", projectResp.Code, projectResp.Body.String())
	}

	syncReq := httptest.NewRequest(http.MethodPost, "/api/v1/work/projects/proj_123/sync", nil)
	syncReq.Header.Set("Authorization", "Bearer test-gateway-token")
	syncResp := httptest.NewRecorder()
	mux.ServeHTTP(syncResp, syncReq)
	if syncResp.Code != http.StatusOK {
		t.Fatalf("sync project status=%d body=%s", syncResp.Code, syncResp.Body.String())
	}

	itemReq := httptest.NewRequest(http.MethodPost, "/api/v1/work/items", strings.NewReader(`{
		"id":"work_123",
		"projectId":"proj_123",
		"title":"Add queue"
	}`))
	itemReq.Header.Set("Content-Type", "application/json")
	itemReq.Header.Set("Authorization", "Bearer test-gateway-token")
	itemResp := httptest.NewRecorder()
	mux.ServeHTTP(itemResp, itemReq)
	if itemResp.Code != http.StatusCreated {
		t.Fatalf("create item status=%d body=%s", itemResp.Code, itemResp.Body.String())
	}

	runReq := httptest.NewRequest(http.MethodPost, "/api/v1/work/items/work_123/runs", strings.NewReader(`{"backend":"local_sandboxed"}`))
	runReq.Header.Set("Content-Type", "application/json")
	runReq.Header.Set("Authorization", "Bearer test-gateway-token")
	runResp := httptest.NewRecorder()
	mux.ServeHTTP(runResp, runReq)
	if runResp.Code != http.StatusCreated {
		t.Fatalf("start run status=%d body=%s", runResp.Code, runResp.Body.String())
	}

	var started struct {
		Run struct {
			ID string `json:"id"`
		} `json:"run"`
	}
	if err := json.Unmarshal(runResp.Body.Bytes(), &started); err != nil {
		t.Fatalf("decode run: %v", err)
	}

	brokenWorksRoot := filepath.Join(t.TempDir(), "works-root-file")
	if err := os.WriteFile(brokenWorksRoot, []byte("not-a-directory"), 0o644); err != nil {
		t.Fatalf("write broken works root: %v", err)
	}
	t.Setenv("CARRIER_WORKS_ROOT", brokenWorksRoot)

	resumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/work/runs/"+started.Run.ID+"/resume", nil)
	resumeReq.Header.Set("Authorization", "Bearer test-gateway-token")
	resumeResp := httptest.NewRecorder()
	mux.ServeHTTP(resumeResp, resumeReq)
	if resumeResp.Code != http.StatusInternalServerError {
		t.Fatalf("resume run status=%d want %d body=%s", resumeResp.Code, http.StatusInternalServerError, resumeResp.Body.String())
	}
}

func TestWorkAPIRBAC(t *testing.T) {
	t.Setenv("CARRIER_ROOT", t.TempDir())
	t.Setenv("CARRIER_APP_ROOT", "")
	t.Setenv("CARRIER_PROJECTS_ROOT", "")
	t.Setenv("CARRIER_WORKS_ROOT", "")

	mux := buildRemoteFeatureMuxWithConfigAndDaemonHandlers(t, testRBACGatewayConfig(), nil)
	repoPath := createGatewayTestGitRepo(t)

	projectRec := runJSONRequestWithToken(t, mux, "admin-token", http.MethodPost, "/api/v1/work/projects", `{
		"id":"proj_123",
		"name":"carrier",
		"sourceType":"local",
		"sourceRef":"`+repoPath+`"
	}`)
	if projectRec.Code != http.StatusCreated {
		t.Fatalf("admin create project status=%d body=%s", projectRec.Code, projectRec.Body.String())
	}

	syncRec := runJSONRequestWithToken(t, mux, "operator-token", http.MethodPost, "/api/v1/work/projects/proj_123/sync", "")
	if syncRec.Code != http.StatusOK {
		t.Fatalf("operator sync project status=%d body=%s", syncRec.Code, syncRec.Body.String())
	}

	itemRec := runJSONRequestWithToken(t, mux, "operator-token", http.MethodPost, "/api/v1/work/items", `{
		"id":"work_123",
		"projectId":"proj_123",
		"title":"Add queue"
	}`)
	if itemRec.Code != http.StatusCreated {
		t.Fatalf("operator create item status=%d body=%s", itemRec.Code, itemRec.Body.String())
	}

	runRec := runJSONRequestWithToken(t, mux, "operator-token", http.MethodPost, "/api/v1/work/items/work_123/runs", `{"backend":"local_sandboxed"}`)
	if runRec.Code != http.StatusCreated {
		t.Fatalf("operator start run status=%d body=%s", runRec.Code, runRec.Body.String())
	}

	var started struct {
		Run struct {
			ID string `json:"id"`
		} `json:"run"`
	}
	if err := json.Unmarshal(runRec.Body.Bytes(), &started); err != nil {
		t.Fatalf("decode run: %v", err)
	}
	if strings.TrimSpace(started.Run.ID) == "" {
		t.Fatalf("missing run id payload=%s", runRec.Body.String())
	}

	readCases := []struct {
		name string
		path string
	}{
		{name: "projects list", path: "/api/v1/work/projects"},
		{name: "project detail", path: "/api/v1/work/projects/proj_123"},
		{name: "items list", path: "/api/v1/work/items"},
		{name: "item detail", path: "/api/v1/work/items/work_123"},
		{name: "runs list", path: "/api/v1/work/runs"},
		{name: "run detail", path: "/api/v1/work/runs/" + started.Run.ID},
	}
	for _, tc := range readCases {
		rec := runJSONRequestWithToken(t, mux, "viewer-token", http.MethodGet, tc.path, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("viewer %s status=%d body=%s", tc.name, rec.Code, rec.Body.String())
		}
	}

	mutateCases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "project sync", method: http.MethodPost, path: "/api/v1/work/projects/proj_123/sync"},
		{name: "project archive", method: http.MethodPost, path: "/api/v1/work/projects/proj_123/archive"},
		{name: "item create", method: http.MethodPost, path: "/api/v1/work/items", body: `{"id":"work_999","projectId":"proj_123","title":"viewer blocked"}`},
		{name: "item patch", method: http.MethodPatch, path: "/api/v1/work/items/work_123", body: `{"title":"blocked"}`},
		{name: "item claim", method: http.MethodPost, path: "/api/v1/work/items/work_123/claim", body: `{"runId":"run_123"}`},
		{name: "item cancel", method: http.MethodPost, path: "/api/v1/work/items/work_123/cancel"},
		{name: "item complete", method: http.MethodPost, path: "/api/v1/work/items/work_123/complete"},
		{name: "item start run", method: http.MethodPost, path: "/api/v1/work/items/work_123/runs", body: `{"backend":"local_sandboxed"}`},
		{name: "run resume", method: http.MethodPost, path: "/api/v1/work/runs/" + started.Run.ID + "/resume"},
		{name: "run cancel", method: http.MethodPost, path: "/api/v1/work/runs/" + started.Run.ID + "/cancel"},
		{name: "run reclaim", method: http.MethodPost, path: "/api/v1/work/runs/" + started.Run.ID + "/reclaim"},
		{name: "run cleanup", method: http.MethodPost, path: "/api/v1/work/runs/" + started.Run.ID + "/cleanup"},
	}
	for _, tc := range mutateCases {
		rec := runJSONRequestWithToken(t, mux, "viewer-token", tc.method, tc.path, tc.body)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("viewer %s status=%d body=%s", tc.name, rec.Code, rec.Body.String())
		}
		payload := decodeJSONMap(t, rec)
		errMap, _ := payload["error"].(map[string]interface{})
		if got := strings.TrimSpace(anyToString(errMap["code"])); got != "E_RBAC_EXECUTION_LAUNCH" {
			t.Fatalf("viewer %s error code=%q payload=%+v", tc.name, got, payload)
		}
	}
}

func TestWorkStoresRejectUnsafeIDs(t *testing.T) {
	t.Setenv("CARRIER_ROOT", t.TempDir())
	t.Setenv("CARRIER_APP_ROOT", "")
	t.Setenv("CARRIER_PROJECTS_ROOT", "")
	t.Setenv("CARRIER_WORKS_ROOT", "")

	if _, err := workItemPath("proj_123", "../escape"); err == nil {
		t.Fatalf("workItemPath accepted unsafe item id")
	}
	if _, err := workRunPath("proj_123", "../escape"); err == nil {
		t.Fatalf("workRunPath accepted unsafe run id")
	}
	if _, _, err := loadWorkItemByID("../escape"); err == nil {
		t.Fatalf("loadWorkItemByID accepted unsafe item id")
	}
	if _, _, err := loadWorkRunByID("../escape"); err == nil {
		t.Fatalf("loadWorkRunByID accepted unsafe run id")
	}
}
