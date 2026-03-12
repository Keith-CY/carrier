package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"carrier/baseagent"
	"carrier/daemon/internal/api"
	"carrier/daemon/internal/lifecycle"
	"carrier/daemon/internal/manifest"
	"carrier/daemon/internal/ratelimit"
)

type fakeBaseAgentRuntime struct {
	resp                baseagent.ChatResponse
	capabilities        baseagent.RuntimeCapabilitySummary
	approvalResp        baseagent.ChatResponse
	approvalErr         error
	installSkill        baseagent.SkillDefinition
	updateSkill         baseagent.SkillDefinition
	uninstallSkill      baseagent.SkillDefinition
	searchSkills        []baseagent.SkillDefinition
	subagentJobs        []baseagent.SubagentJob
	subagentJob         baseagent.SubagentJob
	cronJob             baseagent.CronJob
	cronJobs            []baseagent.CronJob
	cancelledCronJob    baseagent.CronJob
	pausedCronJob       baseagent.CronJob
	resumedCronJob      baseagent.CronJob
	ranCronJob          baseagent.CronJob
	mcpServerDetail     baseagent.MCPServerCapability
	cronErr             error
	skillToggleErr      error
	mcpToggleErr        error
	callCount           int
	approvalCall        int
	cronCall            int
	listCronCall        int
	cancelCronCall      int
	pauseCronCall       int
	resumeCronCall      int
	runCronCall         int
	skillToggleCall     int
	mcpToggleCall       int
	searchSkillsCall    int
	installSkillCall    int
	updateSkillCall     int
	uninstallSkillCall  int
	lastReq             baseagent.ChatRequest
	lastSession         string
	lastApproval        string
	lastDecision        string
	lastCronJob         baseagent.CronJob
	lastCronListSession string
	lastCancelledCronID string
	lastSkillName       string
	lastSkillEnabled    bool
	lastMCPServerName   string
	lastMCPEnabled      bool
	lastInstallSkill    string
	lastUpdateSkill     string
	lastUpdateVersion   string
	lastUninstallSkill  string
	lastSkillSearch     string
	lastSubagentJobID   string
	lastSubagentLimit   int
}

func (f *fakeBaseAgentRuntime) Chat(_ context.Context, req baseagent.ChatRequest) (baseagent.ChatResponse, error) {
	f.callCount++
	f.lastReq = req
	return f.resp, nil
}

func (f *fakeBaseAgentRuntime) CapabilitySummary(_ context.Context) baseagent.RuntimeCapabilitySummary {
	return f.capabilities
}

func (f *fakeBaseAgentRuntime) SetSkillEnabled(_ context.Context, name string, enabled bool) error {
	f.skillToggleCall++
	f.lastSkillName = name
	f.lastSkillEnabled = enabled
	if f.skillToggleErr != nil {
		return f.skillToggleErr
	}
	for i := range f.capabilities.Skills {
		if f.capabilities.Skills[i].Name == name {
			f.capabilities.Skills[i].Enabled = enabled
		}
	}
	f.capabilities.SkillSummary = baseagent.RuntimeSkillSummary{}
	for _, skill := range f.capabilities.Skills {
		f.capabilities.SkillSummary.InstalledCount++
		if skill.Enabled {
			f.capabilities.SkillSummary.EnabledCount++
			continue
		}
		f.capabilities.SkillSummary.DisabledCount++
	}
	return nil
}

func (f *fakeBaseAgentRuntime) SearchSkills(_ context.Context, query string) []baseagent.SkillDefinition {
	f.searchSkillsCall++
	f.lastSkillSearch = query
	return append([]baseagent.SkillDefinition(nil), f.searchSkills...)
}

func (f *fakeBaseAgentRuntime) InstallSkill(_ context.Context, name string) (baseagent.SkillDefinition, error) {
	f.installSkillCall++
	f.lastInstallSkill = name
	if f.installSkill.Name == "" {
		f.installSkill = baseagent.SkillDefinition{Name: name, Summary: "installed skill"}
	}
	return f.installSkill, nil
}

func (f *fakeBaseAgentRuntime) UpdateSkill(_ context.Context, name, version string) (baseagent.SkillDefinition, error) {
	f.updateSkillCall++
	f.lastUpdateSkill = name
	f.lastUpdateVersion = version
	if f.updateSkill.Name == "" {
		f.updateSkill = baseagent.SkillDefinition{Name: name, Summary: "updated skill", TargetVersion: version}
	}
	return f.updateSkill, nil
}

func (f *fakeBaseAgentRuntime) UninstallSkill(_ context.Context, name string) (baseagent.SkillDefinition, error) {
	f.uninstallSkillCall++
	f.lastUninstallSkill = name
	if f.uninstallSkill.Name == "" {
		f.uninstallSkill = baseagent.SkillDefinition{Name: name, Summary: "removed skill"}
	}
	return f.uninstallSkill, nil
}

func (f *fakeBaseAgentRuntime) RecentSubagentJobs(_ context.Context, limit int) []baseagent.SubagentJob {
	f.lastSubagentLimit = limit
	return append([]baseagent.SubagentJob(nil), f.subagentJobs...)
}

func (f *fakeBaseAgentRuntime) SubagentJob(_ context.Context, jobID string) (baseagent.SubagentJob, error) {
	f.lastSubagentJobID = jobID
	if f.subagentJob.JobID == "" {
		return baseagent.SubagentJob{}, fmt.Errorf("subagent job %s not found", jobID)
	}
	return f.subagentJob, nil
}

func (f *fakeBaseAgentRuntime) SetMCPServerEnabled(_ context.Context, name string, enabled bool) error {
	f.mcpToggleCall++
	f.lastMCPServerName = name
	f.lastMCPEnabled = enabled
	if f.mcpToggleErr != nil {
		return f.mcpToggleErr
	}
	for i := range f.capabilities.MCP.Servers {
		if f.capabilities.MCP.Servers[i].Name == name {
			f.capabilities.MCP.Servers[i].Enabled = enabled
			if enabled {
				f.capabilities.MCP.Servers[i].Health = "healthy"
			} else {
				f.capabilities.MCP.Servers[i].Health = "stopped"
			}
		}
	}
	if !enabled {
		f.capabilities.MCP.VisibleTools = nil
	}
	return nil
}

func (f *fakeBaseAgentRuntime) MCPServerDetail(_ context.Context, name string) (baseagent.MCPServerCapability, error) {
	f.lastMCPServerName = name
	if strings.TrimSpace(f.mcpServerDetail.Name) == "" {
		return baseagent.MCPServerCapability{}, fmt.Errorf("mcp server %s not found", name)
	}
	return f.mcpServerDetail, nil
}

func (f *fakeBaseAgentRuntime) RespondPendingApproval(_ context.Context, sessionKey, approvalID string, decision baseagent.ApprovalDecision) (baseagent.ChatResponse, error) {
	f.approvalCall++
	f.lastSession = sessionKey
	f.lastApproval = approvalID
	f.lastDecision = string(decision)
	return f.approvalResp, f.approvalErr
}

func (f *fakeBaseAgentRuntime) ScheduleJob(_ context.Context, job baseagent.CronJob) (baseagent.CronJob, error) {
	f.cronCall++
	f.lastCronJob = job
	if f.cronJob.ID == "" {
		f.cronJob = job
	}
	return f.cronJob, f.cronErr
}

func (f *fakeBaseAgentRuntime) ListCronJobs(_ context.Context, sessionKey string) ([]baseagent.CronJob, error) {
	f.listCronCall++
	f.lastCronListSession = sessionKey
	return append([]baseagent.CronJob(nil), f.cronJobs...), nil
}

func (f *fakeBaseAgentRuntime) CancelCronJob(_ context.Context, jobID string) (baseagent.CronJob, error) {
	f.cancelCronCall++
	f.lastCancelledCronID = jobID
	if f.cancelledCronJob.ID == "" {
		f.cancelledCronJob = baseagent.CronJob{ID: jobID}
	}
	return f.cancelledCronJob, nil
}

func (f *fakeBaseAgentRuntime) PauseCronJob(_ context.Context, jobID string) (baseagent.CronJob, error) {
	f.pauseCronCall++
	f.lastCancelledCronID = jobID
	if f.pausedCronJob.ID == "" {
		f.pausedCronJob = baseagent.CronJob{ID: jobID, Paused: true, LastResult: "paused"}
	}
	return f.pausedCronJob, nil
}

func (f *fakeBaseAgentRuntime) ResumeCronJob(_ context.Context, jobID string) (baseagent.CronJob, error) {
	f.resumeCronCall++
	f.lastCancelledCronID = jobID
	if f.resumedCronJob.ID == "" {
		f.resumedCronJob = baseagent.CronJob{ID: jobID, LastResult: "resumed"}
	}
	return f.resumedCronJob, nil
}

func (f *fakeBaseAgentRuntime) RunCronJob(_ context.Context, jobID string) (baseagent.CronJob, error) {
	f.runCronCall++
	f.lastCancelledCronID = jobID
	if f.ranCronJob.ID == "" {
		f.ranCronJob = baseagent.CronJob{ID: jobID, LastResult: "succeeded", History: []baseagent.CronRun{{Trigger: "manual", Result: "succeeded"}}}
	}
	return f.ranCronJob, nil
}

func newTestService() *lifecycle.Service {
	return lifecycle.NewService(baseagent.NoopTriager{})
}

func newTestServiceWithAgent(t *testing.T) *lifecycle.Service {
	t.Helper()
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	m := manifest.Manifest{
		ID:      "test-agent",
		Name:    "Test Agent",
		Version: "1.0.0",
		Runtime: manifest.RuntimeSpec{
			Type:    manifest.RuntimeTypeLocalBinary,
			Install: manifest.CommandSpec{Command: "echo installed"},
			Start:   manifest.CommandSpec{Command: "echo started"},
			Stop:    manifest.CommandSpec{Command: "echo stopped"},
		},
		Memory: manifest.MemorySpec{
			Supports:  []manifest.MemoryType{manifest.MemoryTypePerAgent},
			MountPath: "/tmp/test-agent-memory",
		},
	}
	if err := svc.RegisterManifest(m); err != nil {
		t.Fatalf("register manifest: %v", err)
	}
	return svc
}

func newTestMux() *http.ServeMux {
	svc := newTestService()
	ready := &atomic.Bool{}
	ready.Store(true)
	pairStore := api.NewPairingCodeStore(nil)
	pairLimiter := ratelimit.New(ratelimit.WithMax(10), ratelimit.WithWindow(1*time.Minute))
	return buildHTTPMux(svc, ready, pairStore, pairLimiter)
}

func TestHealthz(t *testing.T) {
	mux := newTestMux()
	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Errorf("expected ok, got %q", body["status"])
	}
}

func TestBaseAgentCapabilitiesEndpoint(t *testing.T) {
	svc := newTestService()
	ready := &atomic.Bool{}
	ready.Store(true)
	rt := &fakeBaseAgentRuntime{
		capabilities: baseagent.RuntimeCapabilitySummary{
			Skills: []baseagent.RuntimeSkillCapability{
				{Name: "go-testing", Enabled: true},
			},
			SkillSummary: baseagent.RuntimeSkillSummary{InstalledCount: 1, EnabledCount: 1},
			MCP: baseagent.MCPCapabilitySummary{
				Servers: []baseagent.MCPServerCapability{
					{Name: "repo", Health: "healthy", VisibleToolCount: 1},
				},
				VisibleTools: []baseagent.MCPToolCapability{
					{Name: "repo_search"},
				},
			},
		},
	}
	mux := buildHTTPMuxWithBaseAgent(svc, rt, ready, api.NewPairingCodeStore(nil), ratelimit.New())

	req := httptest.NewRequest(http.MethodGet, "/api/base-agent/capabilities", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"go-testing"`) || !strings.Contains(w.Body.String(), `"repo_search"`) || !strings.Contains(w.Body.String(), `"enabledCount":1`) {
		t.Fatalf("unexpected capabilities body: %s", w.Body.String())
	}
}

func TestAgentSkillToggleEndpoint(t *testing.T) {
	svc := newTestServiceWithAgent(t)
	ready := &atomic.Bool{}
	ready.Store(true)
	rt := &fakeBaseAgentRuntime{
		capabilities: baseagent.RuntimeCapabilitySummary{
			Skills: []baseagent.RuntimeSkillCapability{
				{Name: "go-testing", Enabled: true},
				{Name: "workspace-inspection", Enabled: false},
			},
			SkillSummary: baseagent.RuntimeSkillSummary{InstalledCount: 2, EnabledCount: 1, DisabledCount: 1},
		},
	}
	mux := buildHTTPMuxWithBaseAgent(svc, rt, ready, api.NewPairingCodeStore(nil), ratelimit.New())

	methodReq := httptest.NewRequest(http.MethodGet, "/api/v1/agents/test-agent/skills/go-testing", nil)
	methodRec := httptest.NewRecorder()
	mux.ServeHTTP(methodRec, methodReq)
	if methodRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", methodRec.Code)
	}

	badReq := httptest.NewRequest(http.MethodPost, "/api/v1/agents/test-agent/skills/go-testing", strings.NewReader("{"))
	badRec := httptest.NewRecorder()
	mux.ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", badRec.Code)
	}

	okReq := httptest.NewRequest(http.MethodPost, "/api/v1/agents/test-agent/skills/go-testing", strings.NewReader(`{"enabled":false}`))
	okReq.Header.Set("Content-Type", "application/json")
	okRec := httptest.NewRecorder()
	mux.ServeHTTP(okRec, okReq)
	if okRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", okRec.Code, okRec.Body.String())
	}
	if rt.skillToggleCall != 1 || rt.lastSkillName != "go-testing" || rt.lastSkillEnabled {
		t.Fatalf("unexpected skill toggle state: %+v", rt)
	}
	if !strings.Contains(okRec.Body.String(), `"disabledCount":2`) || !strings.Contains(okRec.Body.String(), `"enabled":false`) {
		t.Fatalf("unexpected skill toggle body: %s", okRec.Body.String())
	}
}

func TestAgentSkillSearchAndInstallEndpoints(t *testing.T) {
	svc := newTestServiceWithAgent(t)
	ready := &atomic.Bool{}
	ready.Store(true)
	rt := &fakeBaseAgentRuntime{
		searchSkills: []baseagent.SkillDefinition{
			{Name: "go-testing", Summary: "Use go test before claiming success.", Source: "catalog", Version: "builtin"},
			{Name: "workspace-inspection", Summary: "Inspect workspace state.", Source: "catalog", Version: "v1.2.3"},
		},
		installSkill:   baseagent.SkillDefinition{Name: "workspace-inspection", Summary: "Inspect workspace state.", Source: "catalog", Version: "v1.2.3"},
		updateSkill:    baseagent.SkillDefinition{Name: "workspace-inspection", Summary: "Inspect workspace state.", Source: "catalog", Version: "v1.2.3", TargetVersion: "v2.0.0"},
		uninstallSkill: baseagent.SkillDefinition{Name: "workspace-inspection", Summary: "Inspect workspace state.", Source: "catalog", Version: "v1.2.3"},
	}
	mux := buildHTTPMuxWithBaseAgent(svc, rt, ready, api.NewPairingCodeStore(nil), ratelimit.New())

	searchReq := httptest.NewRequest(http.MethodGet, "/api/v1/agents/test-agent/skills/search?q=workspace", nil)
	searchRec := httptest.NewRecorder()
	mux.ServeHTTP(searchRec, searchReq)
	if searchRec.Code != http.StatusOK {
		t.Fatalf("expected search 200, got %d body=%s", searchRec.Code, searchRec.Body.String())
	}
	if rt.searchSkillsCall != 1 || rt.lastSkillSearch != "workspace" {
		t.Fatalf("unexpected skill search state: %+v", rt)
	}
	if !strings.Contains(searchRec.Body.String(), `"workspace-inspection"`) {
		t.Fatalf("unexpected skill search body: %s", searchRec.Body.String())
	}
	if !strings.Contains(searchRec.Body.String(), `"source":"catalog"`) || !strings.Contains(searchRec.Body.String(), `"version":"v1.2.3"`) {
		t.Fatalf("expected search metadata in body: %s", searchRec.Body.String())
	}

	installBadReq := httptest.NewRequest(http.MethodPost, "/api/v1/agents/test-agent/skills/install", strings.NewReader(`{}`))
	installBadRec := httptest.NewRecorder()
	mux.ServeHTTP(installBadRec, installBadReq)
	if installBadRec.Code != http.StatusBadRequest {
		t.Fatalf("expected install 400 for missing name, got %d", installBadRec.Code)
	}

	installReq := httptest.NewRequest(http.MethodPost, "/api/v1/agents/test-agent/skills/install", strings.NewReader(`{"name":"workspace-inspection"}`))
	installReq.Header.Set("Content-Type", "application/json")
	installRec := httptest.NewRecorder()
	mux.ServeHTTP(installRec, installReq)
	if installRec.Code != http.StatusOK {
		t.Fatalf("expected install 200, got %d body=%s", installRec.Code, installRec.Body.String())
	}
	if rt.installSkillCall != 1 || rt.lastInstallSkill != "workspace-inspection" {
		t.Fatalf("unexpected skill install state: %+v", rt)
	}
	if !strings.Contains(installRec.Body.String(), `"workspace-inspection"`) {
		t.Fatalf("unexpected skill install body: %s", installRec.Body.String())
	}
	if !strings.Contains(installRec.Body.String(), `"source":"catalog"`) || !strings.Contains(installRec.Body.String(), `"version":"v1.2.3"`) {
		t.Fatalf("expected install metadata in body: %s", installRec.Body.String())
	}

	updateReq := httptest.NewRequest(http.MethodPost, "/api/v1/agents/test-agent/skills/update", strings.NewReader(`{"name":"workspace-inspection","version":"v2.0.0"}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	mux.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected update 200, got %d body=%s", updateRec.Code, updateRec.Body.String())
	}
	if rt.updateSkillCall != 1 || rt.lastUpdateSkill != "workspace-inspection" || rt.lastUpdateVersion != "v2.0.0" {
		t.Fatalf("unexpected skill update state: %+v", rt)
	}
	if !strings.Contains(updateRec.Body.String(), `"workspace-inspection"`) || !strings.Contains(updateRec.Body.String(), `"targetVersion":"v2.0.0"`) {
		t.Fatalf("unexpected skill update body: %s", updateRec.Body.String())
	}

	uninstallReq := httptest.NewRequest(http.MethodPost, "/api/v1/agents/test-agent/skills/uninstall", strings.NewReader(`{"name":"workspace-inspection"}`))
	uninstallReq.Header.Set("Content-Type", "application/json")
	uninstallRec := httptest.NewRecorder()
	mux.ServeHTTP(uninstallRec, uninstallReq)
	if uninstallRec.Code != http.StatusOK {
		t.Fatalf("expected uninstall 200, got %d body=%s", uninstallRec.Code, uninstallRec.Body.String())
	}
	if rt.uninstallSkillCall != 1 || rt.lastUninstallSkill != "workspace-inspection" {
		t.Fatalf("unexpected skill uninstall state: %+v", rt)
	}
	if !strings.Contains(uninstallRec.Body.String(), `"workspace-inspection"`) || !strings.Contains(uninstallRec.Body.String(), `"version":"v1.2.3"`) {
		t.Fatalf("unexpected skill uninstall body: %s", uninstallRec.Body.String())
	}
}

func TestAgentMCPServerToggleEndpoint(t *testing.T) {
	svc := newTestServiceWithAgent(t)
	ready := &atomic.Bool{}
	ready.Store(true)
	rt := &fakeBaseAgentRuntime{
		capabilities: baseagent.RuntimeCapabilitySummary{
			MCP: baseagent.MCPCapabilitySummary{
				Servers: []baseagent.MCPServerCapability{
					{Name: "repo", Health: "healthy", Enabled: true, Manageable: true, VisibleToolCount: 1},
				},
				VisibleTools: []baseagent.MCPToolCapability{{Name: "repo_search"}},
			},
		},
		mcpServerDetail: baseagent.MCPServerCapability{
			Name:            "repo",
			Health:          "healthy",
			Enabled:         true,
			Manageable:      true,
			VisibleToolCount: 1,
			HiddenToolCount: 1,
			HealthDetail:    "connected to repository index",
			RemediationHint: "Disable MCP if repository indexing becomes noisy.",
			VisibleTools:    []baseagent.MCPToolCapability{{Name: "repo_search", Description: "Search code"}},
			HiddenTools:     []baseagent.MCPToolCapability{{Name: "repo_admin", Description: "Admin index"}},
		},
	}
	mux := buildHTTPMuxWithBaseAgent(svc, rt, ready, api.NewPairingCodeStore(nil), ratelimit.New())

	detailReq := httptest.NewRequest(http.MethodGet, "/api/v1/agents/test-agent/mcp/repo", nil)
	detailRec := httptest.NewRecorder()
	mux.ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", detailRec.Code, detailRec.Body.String())
	}
	if !strings.Contains(detailRec.Body.String(), `"healthDetail":"connected to repository index"`) ||
		!strings.Contains(detailRec.Body.String(), `"remediationHint":"Disable MCP if repository indexing becomes noisy."`) ||
		!strings.Contains(detailRec.Body.String(), `"hiddenToolCount":1`) {
		t.Fatalf("unexpected mcp detail body: %s", detailRec.Body.String())
	}

	okReq := httptest.NewRequest(http.MethodPost, "/api/v1/agents/test-agent/mcp/repo", strings.NewReader(`{"enabled":false}`))
	okReq.Header.Set("Content-Type", "application/json")
	okRec := httptest.NewRecorder()
	mux.ServeHTTP(okRec, okReq)
	if okRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", okRec.Code, okRec.Body.String())
	}
	if rt.mcpToggleCall != 1 || rt.lastMCPServerName != "repo" || rt.lastMCPEnabled {
		t.Fatalf("unexpected mcp toggle state: %+v", rt)
	}
	if !strings.Contains(okRec.Body.String(), `"name":"repo"`) || !strings.Contains(okRec.Body.String(), `"enabled":false`) {
		t.Fatalf("unexpected mcp toggle body: %s", okRec.Body.String())
	}
}

func TestAgentSubagentHistoryEndpoints(t *testing.T) {
	svc := newTestServiceWithAgent(t)
	ready := &atomic.Bool{}
	ready.Store(true)
	rt := &fakeBaseAgentRuntime{
		subagentJobs: []baseagent.SubagentJob{
			{JobID: "subagent-2", Task: "summarize", Status: baseagent.SubagentJobStatusCancelled, Error: "subagent job cancelled"},
			{JobID: "subagent-3", Task: "collect", Status: baseagent.SubagentJobStatusCompleted, Result: "done"},
		},
		subagentJob: baseagent.SubagentJob{JobID: "subagent-3", Task: "collect", Status: baseagent.SubagentJobStatusCompleted, Result: "done"},
	}
	mux := buildHTTPMuxWithBaseAgent(svc, rt, ready, api.NewPairingCodeStore(nil), ratelimit.New())

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/agents/test-agent/subagents?limit=2", nil)
	listRec := httptest.NewRecorder()
	mux.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected subagent list 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	if rt.lastSubagentLimit != 2 {
		t.Fatalf("expected limit to be forwarded, got %d", rt.lastSubagentLimit)
	}
	if !strings.Contains(listRec.Body.String(), `"subagent-2"`) || !strings.Contains(listRec.Body.String(), `"subagent-3"`) {
		t.Fatalf("unexpected subagent list body: %s", listRec.Body.String())
	}

	jobReq := httptest.NewRequest(http.MethodGet, "/api/v1/agents/test-agent/subagents/subagent-3", nil)
	jobRec := httptest.NewRecorder()
	mux.ServeHTTP(jobRec, jobReq)
	if jobRec.Code != http.StatusOK {
		t.Fatalf("expected subagent get 200, got %d body=%s", jobRec.Code, jobRec.Body.String())
	}
	if rt.lastSubagentJobID != "subagent-3" {
		t.Fatalf("expected subagent job id to be forwarded, got %q", rt.lastSubagentJobID)
	}
	if !strings.Contains(jobRec.Body.String(), `"subagent-3"`) || !strings.Contains(jobRec.Body.String(), `"completed"`) {
		t.Fatalf("unexpected subagent job body: %s", jobRec.Body.String())
	}

	badLimitReq := httptest.NewRequest(http.MethodGet, "/api/v1/agents/test-agent/subagents?limit=0", nil)
	badLimitRec := httptest.NewRecorder()
	mux.ServeHTTP(badLimitRec, badLimitReq)
	if badLimitRec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid limit 400, got %d body=%s", badLimitRec.Code, badLimitRec.Body.String())
	}
}

func TestReadyz_Ready(t *testing.T) {
	mux := newTestMux()
	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestReadyz_NotReady(t *testing.T) {
	svc := newTestService()
	ready := &atomic.Bool{}
	ready.Store(false)
	pairStore := api.NewPairingCodeStore(nil)
	mux := buildHTTPMux(svc, ready, pairStore, nil)

	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestDaemonServerApprovalEndpoint(t *testing.T) {
	svc := newTestServiceWithAgent(t)
	ready := &atomic.Bool{}
	ready.Store(true)
	rt := &fakeBaseAgentRuntime{
		resp:         baseagent.ChatResponse{Message: "unused", Action: "chat"},
		approvalResp: baseagent.ChatResponse{Message: "confirmed", Action: "approval_confirm"},
	}
	mux := buildHTTPMuxWithBaseAgent(svc, rt, ready, api.NewPairingCodeStore(nil), ratelimit.New())

	req := httptest.NewRequest(http.MethodPost, "/api/base-agent/approvals/consume", strings.NewReader(`{"sessionKey":"cli:approval-api","approvalId":"approval-1","decision":"confirm"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", w.Code, w.Body.String())
	}
	if rt.approvalCall != 1 || rt.lastSession != "cli:approval-api" || rt.lastApproval != "approval-1" || rt.lastDecision != "confirm" {
		t.Fatalf("unexpected approval runtime call state: %+v", rt)
	}
	if !strings.Contains(w.Body.String(), `"action":"approval_confirm"`) {
		t.Fatalf("unexpected approval response body=%s", w.Body.String())
	}
}

func TestDaemonServerApprovalEndpointReject(t *testing.T) {
	svc := newTestServiceWithAgent(t)
	ready := &atomic.Bool{}
	ready.Store(true)
	rt := &fakeBaseAgentRuntime{
		approvalResp: baseagent.ChatResponse{Message: "rejected", Action: "approval_cancel"},
	}
	mux := buildHTTPMuxWithBaseAgent(svc, rt, ready, api.NewPairingCodeStore(nil), ratelimit.New())

	req := httptest.NewRequest(http.MethodPost, "/api/base-agent/approvals/consume", strings.NewReader(`{"provider":"cli","chatId":"approval-api","approvalId":"approval-2","decision":"reject"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", w.Code, w.Body.String())
	}
	if rt.lastSession != baseagent.ResolveSessionKey("cli", "approval-api") || rt.lastDecision != "reject" {
		t.Fatalf("unexpected derived approval request state: %+v", rt)
	}
	if !strings.Contains(w.Body.String(), `"action":"approval_cancel"`) {
		t.Fatalf("unexpected reject response body=%s", w.Body.String())
	}
}

func TestDaemonServerApprovalEndpointMethodAndValidationBranches(t *testing.T) {
	svc := newTestServiceWithAgent(t)
	ready := &atomic.Bool{}
	ready.Store(true)
	mux := buildHTTPMuxWithBaseAgent(svc, &fakeBaseAgentRuntime{}, ready, api.NewPairingCodeStore(nil), ratelimit.New())

	t.Run("method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/base-agent/approvals/consume", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", w.Code)
		}
	})

	t.Run("missing session identity", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/base-agent/approvals/consume", strings.NewReader(`{"approvalId":"approval-1","decision":"confirm"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("missing approval id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/base-agent/approvals/consume", strings.NewReader(`{"sessionKey":"cli:approval-api","decision":"confirm"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("missing decision", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/base-agent/approvals/consume", strings.NewReader(`{"sessionKey":"cli:approval-api","approvalId":"approval-1"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestDaemonServerApprovalEndpointMapsRuntimeErrors(t *testing.T) {
	svc := newTestServiceWithAgent(t)
	ready := &atomic.Bool{}
	ready.Store(true)

	t.Run("invalid decision", func(t *testing.T) {
		rt := &fakeBaseAgentRuntime{approvalErr: baseagent.ErrInvalidApprovalDecision}
		mux := buildHTTPMuxWithBaseAgent(svc, rt, ready, api.NewPairingCodeStore(nil), ratelimit.New())
		req := httptest.NewRequest(http.MethodPost, "/api/base-agent/approvals/consume", strings.NewReader(`{"sessionKey":"cli:approval-api","approvalId":"approval-1","decision":"later"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("not found", func(t *testing.T) {
		rt := &fakeBaseAgentRuntime{approvalErr: baseagent.ErrPendingApprovalNotFound}
		mux := buildHTTPMuxWithBaseAgent(svc, rt, ready, api.NewPairingCodeStore(nil), ratelimit.New())
		req := httptest.NewRequest(http.MethodPost, "/api/base-agent/approvals/consume", strings.NewReader(`{"sessionKey":"cli:approval-api","approvalId":"approval-1","decision":"confirm"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestDaemonServerCronScheduleEndpoint(t *testing.T) {
	svc := newTestServiceWithAgent(t)
	ready := &atomic.Bool{}
	ready.Store(true)
	rt := &fakeBaseAgentRuntime{
		cronJob: baseagent.CronJob{ID: "cron-1", SessionKey: "cli:cron-api", Prompt: "perform maintenance planning"},
	}
	mux := buildHTTPMuxWithBaseAgent(svc, rt, ready, api.NewPairingCodeStore(nil), ratelimit.New())

	req := httptest.NewRequest(http.MethodPost, "/api/base-agent/cron/schedule", strings.NewReader(`{"sessionKey":"cli:cron-api","prompt":"perform maintenance planning"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", w.Code, w.Body.String())
	}
	if rt.cronCall != 1 || rt.lastCronJob.SessionKey != "cli:cron-api" || rt.lastCronJob.Prompt != "perform maintenance planning" {
		t.Fatalf("unexpected cron runtime call state: %+v", rt)
	}
	if !strings.Contains(w.Body.String(), `"id":"cron-1"`) {
		t.Fatalf("unexpected cron response body=%s", w.Body.String())
	}
}

func TestDaemonServerCronScheduleEndpointDerivesSessionKey(t *testing.T) {
	svc := newTestServiceWithAgent(t)
	ready := &atomic.Bool{}
	ready.Store(true)
	rt := &fakeBaseAgentRuntime{
		cronJob: baseagent.CronJob{ID: "cron-2", SessionKey: "cli:cron-derived", Prompt: "perform maintenance planning"},
	}
	mux := buildHTTPMuxWithBaseAgent(svc, rt, ready, api.NewPairingCodeStore(nil), ratelimit.New())

	req := httptest.NewRequest(http.MethodPost, "/api/base-agent/cron/schedule", strings.NewReader(`{"provider":"cli","chatId":"cron-derived","prompt":"perform maintenance planning"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", w.Code, w.Body.String())
	}
	if rt.lastCronJob.SessionKey != baseagent.ResolveSessionKey("cli", "cron-derived") {
		t.Fatalf("unexpected derived cron job state: %+v", rt.lastCronJob)
	}
}

func TestDaemonServerCronListAndCancelEndpoints(t *testing.T) {
	svc := newTestServiceWithAgent(t)
	ready := &atomic.Bool{}
	ready.Store(true)
	lastRunAt := time.Date(2026, 3, 12, 10, 0, 0, 0, time.UTC)
	rt := &fakeBaseAgentRuntime{
		cronJobs: []baseagent.CronJob{{
			ID:         "cron-3",
			SessionKey: "agent:picoclaw",
			Prompt:     "check launcher",
			NextRunAt:  time.Date(2026, 3, 12, 11, 0, 0, 0, time.UTC),
			LastRunAt:  &lastRunAt,
			LastResult: "succeeded",
		}},
		cancelledCronJob: baseagent.CronJob{
			ID:          "cron-3",
			SessionKey:  "agent:picoclaw",
			Prompt:      "check launcher",
			LastResult:  "cancelled",
			CancelledAt: &lastRunAt,
		},
	}
	mux := buildHTTPMuxWithBaseAgent(svc, rt, ready, api.NewPairingCodeStore(nil), ratelimit.New())

	listReq := httptest.NewRequest(http.MethodGet, "/api/base-agent/cron/jobs?sessionKey=agent:picoclaw", nil)
	listRec := httptest.NewRecorder()
	mux.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listRec.Code, listRec.Body.String())
	}
	if rt.listCronCall != 1 || rt.lastCronListSession != "agent:picoclaw" {
		t.Fatalf("unexpected cron list call state: %+v", rt)
	}
	if !strings.Contains(listRec.Body.String(), `"lastResult":"succeeded"`) {
		t.Fatalf("unexpected cron list body=%s", listRec.Body.String())
	}

	cancelReq := httptest.NewRequest(http.MethodPost, "/api/base-agent/cron/cron-3/cancel", nil)
	cancelRec := httptest.NewRecorder()
	mux.ServeHTTP(cancelRec, cancelReq)
	if cancelRec.Code != http.StatusOK {
		t.Fatalf("cancel status=%d body=%s", cancelRec.Code, cancelRec.Body.String())
	}
	if rt.cancelCronCall != 1 || rt.lastCancelledCronID != "cron-3" {
		t.Fatalf("unexpected cron cancel call state: %+v", rt)
	}
	if !strings.Contains(cancelRec.Body.String(), `"lastResult":"cancelled"`) {
		t.Fatalf("unexpected cron cancel body=%s", cancelRec.Body.String())
	}

	rt.pausedCronJob = baseagent.CronJob{
		ID:         "cron-3",
		SessionKey: "agent:picoclaw",
		Prompt:     "check launcher",
		LastResult: "paused",
		Paused:     true,
	}
	pauseReq := httptest.NewRequest(http.MethodPost, "/api/base-agent/cron/cron-3/pause", nil)
	pauseRec := httptest.NewRecorder()
	mux.ServeHTTP(pauseRec, pauseReq)
	if pauseRec.Code != http.StatusOK {
		t.Fatalf("pause status=%d body=%s", pauseRec.Code, pauseRec.Body.String())
	}
	if rt.pauseCronCall != 1 || rt.lastCancelledCronID != "cron-3" {
		t.Fatalf("unexpected cron pause call state: %+v", rt)
	}
	if !strings.Contains(pauseRec.Body.String(), `"lastResult":"paused"`) {
		t.Fatalf("unexpected cron pause body=%s", pauseRec.Body.String())
	}

	rt.resumedCronJob = baseagent.CronJob{
		ID:         "cron-3",
		SessionKey: "agent:picoclaw",
		Prompt:     "check launcher",
		LastResult: "resumed",
		Paused:     false,
	}
	resumeReq := httptest.NewRequest(http.MethodPost, "/api/base-agent/cron/cron-3/resume", nil)
	resumeRec := httptest.NewRecorder()
	mux.ServeHTTP(resumeRec, resumeReq)
	if resumeRec.Code != http.StatusOK {
		t.Fatalf("resume status=%d body=%s", resumeRec.Code, resumeRec.Body.String())
	}
	if rt.resumeCronCall != 1 || rt.lastCancelledCronID != "cron-3" {
		t.Fatalf("unexpected cron resume call state: %+v", rt)
	}
	if !strings.Contains(resumeRec.Body.String(), `"lastResult":"resumed"`) {
		t.Fatalf("unexpected cron resume body=%s", resumeRec.Body.String())
	}

	rt.ranCronJob = baseagent.CronJob{
		ID:         "cron-3",
		SessionKey: "agent:picoclaw",
		Prompt:     "check launcher",
		LastResult: "succeeded",
		History:    []baseagent.CronRun{{Trigger: "manual", Result: "succeeded"}},
	}
	runReq := httptest.NewRequest(http.MethodPost, "/api/base-agent/cron/cron-3/run", nil)
	runRec := httptest.NewRecorder()
	mux.ServeHTTP(runRec, runReq)
	if runRec.Code != http.StatusOK {
		t.Fatalf("run status=%d body=%s", runRec.Code, runRec.Body.String())
	}
	if rt.runCronCall != 1 || rt.lastCancelledCronID != "cron-3" {
		t.Fatalf("unexpected cron run call state: %+v", rt)
	}
	if !strings.Contains(runRec.Body.String(), `"trigger":"manual"`) {
		t.Fatalf("unexpected cron run body=%s", runRec.Body.String())
	}
}

func TestListAgents(t *testing.T) {
	mux := newTestMux()
	req := httptest.NewRequest("GET", "/api/agents", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestListAgents_V1(t *testing.T) {
	mux := newTestMux()
	req := httptest.NewRequest("GET", "/api/v1/agents/status", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestListAgents_MethodNotAllowed(t *testing.T) {
	mux := newTestMux()
	req := httptest.NewRequest("POST", "/api/agents", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestInstall_MissingAgentID(t *testing.T) {
	mux := newTestMux()
	body := `{"agentId":""}`
	req := httptest.NewRequest("POST", "/api/install", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestInstall_MethodNotAllowed(t *testing.T) {
	mux := newTestMux()
	req := httptest.NewRequest("GET", "/api/install", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestStart_MethodNotAllowed(t *testing.T) {
	mux := newTestMux()
	req := httptest.NewRequest("GET", "/api/start", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestStop_MethodNotAllowed(t *testing.T) {
	mux := newTestMux()
	req := httptest.NewRequest("GET", "/api/stop", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestPairingCodes_List(t *testing.T) {
	mux := newTestMux()
	req := httptest.NewRequest("GET", "/api/pairing/codes", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestPairingVerifyConsume_MethodNotAllowed(t *testing.T) {
	mux := newTestMux()
	req := httptest.NewRequest("GET", "/api/pairing/verify-consume", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestPairingVerifyConsume_InvalidCode(t *testing.T) {
	mux := newTestMux()
	body := `{"code":"invalid-code"}`
	req := httptest.NewRequest("POST", "/api/pairing/verify-consume", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestPairingVerifyConsume_Success(t *testing.T) {
	svc := newTestService()
	ready := &atomic.Bool{}
	ready.Store(true)
	pairStore := api.NewPairingCodeStore(nil)
	record, _ := pairStore.Issue(5 * time.Minute)
	pairLimiter := ratelimit.New(ratelimit.WithMax(10), ratelimit.WithWindow(1*time.Minute))
	mux := buildHTTPMux(svc, ready, pairStore, pairLimiter)

	body := `{"code":"` + record.Code + `"}`
	req := httptest.NewRequest("POST", "/api/pairing/verify-consume", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestDiagnosisHandoffs_MethodNotAllowed(t *testing.T) {
	mux := newTestMux()
	req := httptest.NewRequest("GET", "/api/v1/diagnosis/handoffs", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestDiagnosisHandoffs_MissingAgentID(t *testing.T) {
	mux := newTestMux()
	body := `{"agentId":"","consent":true}`
	req := httptest.NewRequest("POST", "/api/v1/diagnosis/handoffs", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestV1AgentAction_NotFound(t *testing.T) {
	mux := newTestMux()
	req := httptest.NewRequest("GET", "/api/v1/agents/myagent/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestV1AgentAction_InvalidPath(t *testing.T) {
	mux := newTestMux()
	req := httptest.NewRequest("GET", "/api/v1/agents/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// Test helper functions

func TestValidateAgentID(t *testing.T) {
	tests := []struct {
		id      string
		wantErr bool
	}{
		{"valid-agent", false},
		{"agent.v1", false},
		{"agent_test", false},
		{"", true},
		{"../bad", true},
		{"a/b", true},
		{"a\\b", true},
		{".invalid", true},
		{"-invalid", true},
	}
	for _, tc := range tests {
		err := validateAgentID(tc.id)
		if tc.wantErr && err == nil {
			t.Errorf("validateAgentID(%q): expected error", tc.id)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("validateAgentID(%q): unexpected error: %v", tc.id, err)
		}
	}
}

func TestParseAgentActionPath(t *testing.T) {
	tests := []struct {
		path   string
		agent  string
		action string
		wantOK bool
	}{
		{"/api/v1/agents/myagent/start", "myagent", "start", true},
		{"/api/v1/agents/agent.v1/stop", "agent.v1", "stop", true},
		{"/api/v1/agents/", "", "", false},
		{"/api/v1/agents/a", "", "", false},        // no action
		{"/api/v1/agents/a/b/c", "", "", false},    // too many parts
		{"/api/v1/agents//start", "", "", false},   // double slash
		{"/api/v1/agents/../start", "", "", false}, // path traversal
		{"/other/path", "", "", false},
	}
	for _, tc := range tests {
		agent, action, ok := parseAgentActionPath(tc.path)
		if ok != tc.wantOK {
			t.Errorf("parseAgentActionPath(%q): ok=%v, want %v", tc.path, ok, tc.wantOK)
			continue
		}
		if ok {
			if agent != tc.agent {
				t.Errorf("parseAgentActionPath(%q): agent=%q, want %q", tc.path, agent, tc.agent)
			}
			if action != tc.action {
				t.Errorf("parseAgentActionPath(%q): action=%q, want %q", tc.path, action, tc.action)
			}
		}
	}
}

func TestParseLogsTail(t *testing.T) {
	tests := []struct {
		raw  string
		want int
	}{
		{"", defaultLogsTail},
		{"100", 100},
		{"abc", defaultLogsTail},
		{"0", defaultLogsTail},
		{"-1", defaultLogsTail},
		{"99999", maxLogsTail},
	}
	for _, tc := range tests {
		got := parseLogsTail(tc.raw)
		if got != tc.want {
			t.Errorf("parseLogsTail(%q) = %d, want %d", tc.raw, got, tc.want)
		}
	}
}

func TestParsePathAgentID(t *testing.T) {
	tests := []struct {
		raw     string
		wantErr bool
	}{
		{"", true},
		{"valid-agent", false},
		{"../bad", true},
		{"a%2fb", true}, // contains /
	}
	for _, tc := range tests {
		_, err := parsePathAgentID(tc.raw)
		if tc.wantErr && err == nil {
			t.Errorf("parsePathAgentID(%q): expected error", tc.raw)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("parsePathAgentID(%q): unexpected error: %v", tc.raw, err)
		}
	}
}

func TestTrimPathByPrefixes(t *testing.T) {
	got := trimPathByPrefixes("/api/logs/myagent", "/api/logs/", "/api/v1/logs/")
	if got != "myagent" {
		t.Errorf("expected myagent, got %q", got)
	}
	got = trimPathByPrefixes("/other/path", "/api/logs/")
	if got != "/other/path" {
		t.Errorf("expected unchanged, got %q", got)
	}
}

func TestBearerAuthMiddleware(t *testing.T) {
	inner := http.NewServeMux()
	inner.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	inner.HandleFunc("/api/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	inner.HandleFunc("/other", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := bearerAuthMiddleware("secret", inner)

	// /api/ without token → 401
	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("/api/test no token: expected 401, got %d", w.Code)
	}

	// /api/ with correct token → 200
	req = httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("/api/test with token: expected 200, got %d", w.Code)
	}

	// /api/ with X-Gateway-Token → 200
	req = httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-Gateway-Token", "secret")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("/api/test with X-Gateway-Token: expected 200, got %d", w.Code)
	}

	// /other doesn't need auth
	req = httptest.NewRequest("GET", "/other", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("/other: expected 200, got %d", w.Code)
	}

	// /healthz requires auth when token is set
	req = httptest.NewRequest("GET", "/healthz", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("/healthz no token: expected 401, got %d", w.Code)
	}
}

func TestDecodeBody(t *testing.T) {
	// Valid body
	body := `{"agentId":"myagent"}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	var result agentIDBody
	ok := decodeBody(w, req, &result)
	if !ok {
		t.Fatal("expected ok")
	}
	if result.AgentID != "myagent" {
		t.Errorf("expected myagent, got %q", result.AgentID)
	}

	// Empty body
	req = httptest.NewRequest("POST", "/", strings.NewReader(""))
	w = httptest.NewRecorder()
	ok = decodeBody(w, req, &result)
	if ok {
		t.Error("expected fail for empty body")
	}

	// Invalid JSON
	req = httptest.NewRequest("POST", "/", strings.NewReader("{invalid"))
	w = httptest.NewRecorder()
	ok = decodeBody(w, req, &result)
	if ok {
		t.Error("expected fail for invalid JSON")
	}

	// Too large content length
	req = httptest.NewRequest("POST", "/", strings.NewReader("x"))
	req.ContentLength = maxBodySize + 1
	w = httptest.NewRecorder()
	ok = decodeBody(w, req, &result)
	if ok {
		t.Error("expected fail for too large body")
	}
}

func TestDecodeBody_TrailingData(t *testing.T) {
	body := `{"agentId":"a1"}{"extra":"data"}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	w := httptest.NewRecorder()
	var result agentIDBody
	ok := decodeBody(w, req, &result)
	if ok {
		t.Error("expected fail for trailing data")
	}
}

func TestDecodeBody_TooLargeActual(t *testing.T) {
	big := `{"agentId":"` + strings.Repeat("a", maxBodySize+10) + `"}`
	req := httptest.NewRequest("POST", "/", bytes.NewReader([]byte(big)))
	w := httptest.NewRecorder()
	var result agentIDBody
	ok := decodeBody(w, req, &result)
	if ok {
		t.Error("expected fail for actually too large body")
	}
}

func TestParseInstallOptionsFromRequest(t *testing.T) {
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/agents/openclaw/install",
		strings.NewReader(`{"instance_name":"dev-a","multi_instance":true,"isolation":true}`),
	)

	opts, instanceName, multiInstance, err := parseInstallOptionsFromRequest(req)
	if err != nil {
		t.Fatalf("parseInstallOptionsFromRequest returned error: %v", err)
	}
	if !opts.Isolation {
		t.Fatalf("expected isolation=true, got %+v", opts)
	}
	if instanceName != "dev-a" {
		t.Fatalf("expected instance name dev-a, got %q", instanceName)
	}
	if !multiInstance {
		t.Fatalf("expected multi_instance=true, got false")
	}
}

func TestParseInstallOptionsFromRequestRejectsInvalidJSON(t *testing.T) {
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/agents/openclaw/install",
		strings.NewReader(`{invalid`),
	)
	if _, _, _, err := parseInstallOptionsFromRequest(req); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

func TestV1AgentInstallRejectsInvalidJSONBody(t *testing.T) {
	mux := newTestMuxWithAgent(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/test-agent/install", strings.NewReader(`{invalid`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestRemoteIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	if got := remoteIP(req); got != "192.168.1.1" {
		t.Errorf("expected 192.168.1.1, got %q", got)
	}

	req.Header.Set("X-Forwarded-For", "10.0.0.1, 10.0.0.2")
	if got := remoteIP(req); got != "10.0.0.1" {
		t.Errorf("expected 10.0.0.1, got %q", got)
	}
}

func TestIsLoopback(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"", true},
		{"localhost", true},
		{"127.0.0.1", true},
		{"::1", true},
		{"0.0.0.0", false},
		{"192.168.1.1", false},
	}
	for _, tc := range tests {
		got := isLoopback(tc.host)
		if got != tc.want {
			t.Errorf("isLoopback(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}

func TestWriteJSON_Server(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusCreated, map[string]string{"k": "v"})
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}
}

func TestWriteJSONError(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSONError(w, http.StatusBadRequest, "bad request")
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "bad request" {
		t.Errorf("expected 'bad request', got %q", body["error"])
	}
}

func TestNewHTTPServer(t *testing.T) {
	srv := newHTTPServer(":0", http.NewServeMux())
	if srv.ReadHeaderTimeout != defaultReadHeaderTimeout {
		t.Errorf("unexpected ReadHeaderTimeout: %v", srv.ReadHeaderTimeout)
	}
	if srv.WriteTimeout != defaultWriteTimeout {
		t.Errorf("unexpected WriteTimeout: %v", srv.WriteTimeout)
	}
	if srv.IdleTimeout != defaultIdleTimeout {
		t.Errorf("unexpected IdleTimeout: %v", srv.IdleTimeout)
	}
}

func newTestMuxWithAgent(t *testing.T) *http.ServeMux {
	t.Helper()
	svc := newTestServiceWithAgent(t)
	ready := &atomic.Bool{}
	ready.Store(true)
	pairStore := api.NewPairingCodeStore(nil)
	pairLimiter := ratelimit.New(ratelimit.WithMax(10), ratelimit.WithWindow(1*time.Minute))
	return buildHTTPMux(svc, ready, pairStore, pairLimiter)
}

func TestV1AgentInstall(t *testing.T) {
	mux := newTestMuxWithAgent(t)
	body := `{"agentId":"test-agent"}`
	req := httptest.NewRequest("POST", "/api/v1/agents/test-agent/install", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestV1AgentStatus(t *testing.T) {
	mux := newTestMuxWithAgent(t)
	req := httptest.NewRequest("GET", "/api/v1/agents/test-agent/status", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestV1AgentStatus_IncludesHeartbeat(t *testing.T) {
	mux := newTestMuxWithAgent(t)
	req := httptest.NewRequest("GET", "/api/v1/agents/test-agent/status", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode status body: %v", err)
	}
	heartbeat, ok := body["heartbeat"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected heartbeat object, got %+v", body["heartbeat"])
	}
	if strings.TrimSpace(fmt.Sprint(heartbeat["state"])) == "" {
		t.Fatalf("expected heartbeat.state, got %+v", heartbeat)
	}
	if strings.TrimSpace(fmt.Sprint(heartbeat["lastActivityAt"])) == "" {
		t.Fatalf("expected heartbeat.lastActivityAt, got %+v", heartbeat)
	}
}

func TestV1AgentStart_NotInstalled(t *testing.T) {
	mux := newTestMuxWithAgent(t)
	body := `{"agentId":"test-agent"}`
	req := httptest.NewRequest("POST", "/api/v1/agents/test-agent/start", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Agent not installed yet → conflict
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestV1AgentStop_NotRunning(t *testing.T) {
	mux := newTestMuxWithAgent(t)
	body := `{"agentId":"test-agent"}`
	req := httptest.NewRequest("POST", "/api/v1/agents/test-agent/stop", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Not running → conflict
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestV1AgentLogs(t *testing.T) {
	mux := newTestMuxWithAgent(t)
	req := httptest.NewRequest("GET", "/api/v1/agents/test-agent/logs", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Should succeed or return empty logs
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestV1AgentUpgrade_NotSupported(t *testing.T) {
	mux := newTestMuxWithAgent(t)
	body := `{"agentId":"test-agent"}`
	req := httptest.NewRequest("POST", "/api/v1/agents/test-agent/upgrade", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Agent not installed → conflict
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestV1AgentDiagnose(t *testing.T) {
	mux := newTestMuxWithAgent(t)
	body := `{"agentId":"test-agent"}`
	req := httptest.NewRequest("POST", "/api/v1/agents/test-agent/diagnose", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestV1AgentUninstall(t *testing.T) {
	mux := newTestMuxWithAgent(t)
	body := `{"agentId":"test-agent"}`
	req := httptest.NewRequest("POST", "/api/v1/agents/test-agent/uninstall", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestV1AgentInstall_MethodNotAllowed(t *testing.T) {
	mux := newTestMuxWithAgent(t)
	req := httptest.NewRequest("GET", "/api/v1/agents/test-agent/install", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestApiStatus_WithAgentID(t *testing.T) {
	mux := newTestMuxWithAgent(t)
	req := httptest.NewRequest("GET", "/api/status/test-agent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestApiLogs_WithAgentID(t *testing.T) {
	mux := newTestMuxWithAgent(t)
	req := httptest.NewRequest("GET", "/api/logs/test-agent?tail=50", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestApiInstall_WithBody(t *testing.T) {
	mux := newTestMuxWithAgent(t)
	body := `{"agentId":"test-agent"}`
	req := httptest.NewRequest("POST", "/api/install", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestApiStart_NotInstalled(t *testing.T) {
	mux := newTestMuxWithAgent(t)
	body := `{"agentId":"test-agent"}`
	req := httptest.NewRequest("POST", "/api/start", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestApiStop_NotRunning(t *testing.T) {
	mux := newTestMuxWithAgent(t)
	body := `{"agentId":"test-agent"}`
	req := httptest.NewRequest("POST", "/api/stop", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Not running → conflict
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestApiUpgrade_WithBody(t *testing.T) {
	mux := newTestMuxWithAgent(t)
	body := `{"agentId":"test-agent"}`
	req := httptest.NewRequest("POST", "/api/upgrade", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Agent not installed → conflict
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestApiDiagnose_WithBody(t *testing.T) {
	mux := newTestMuxWithAgent(t)
	body := `{"agentId":"test-agent"}`
	req := httptest.NewRequest("POST", "/api/diagnose", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestV1AgentStatus_NotFound(t *testing.T) {
	mux := newTestMux()
	req := httptest.NewRequest("GET", "/api/v1/agents/nonexistent/status", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestWriteServiceError(t *testing.T) {
	tests := []struct {
		err    error
		status int
	}{
		{lifecycle.ErrAgentNotFound, http.StatusNotFound},
		{lifecycle.ErrNotInstalled, http.StatusConflict},
		{lifecycle.ErrAlreadyRunning, http.StatusConflict},
		{lifecycle.ErrAlreadyStopped, http.StatusConflict},
		{lifecycle.ErrCrashLoop, http.StatusConflict},
		{lifecycle.ErrAgentRunning, http.StatusConflict},
		{lifecycle.ErrUpgradeNotSupported, http.StatusBadRequest},
		{lifecycle.ErrIsolationUnavailable, http.StatusUnprocessableEntity},
		{lifecycle.ErrIsolationStartFailed, http.StatusBadGateway},
	}
	for _, tc := range tests {
		w := httptest.NewRecorder()
		writeServiceError(w, tc.err)
		if w.Code != tc.status {
			t.Errorf("writeServiceError(%v): expected %d, got %d", tc.err, tc.status, w.Code)
		}
	}
}
