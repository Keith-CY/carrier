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
