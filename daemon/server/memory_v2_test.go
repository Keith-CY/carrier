package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"carrier/baseagent"
	"carrier/daemon/internal/api"
	"carrier/daemon/internal/lifecycle"
	"carrier/daemon/internal/memory"
	"carrier/daemon/internal/ratelimit"
)

func TestMemoryV2SearchEndpoint(t *testing.T) {
	memStore := memory.NewStore(memory.WithRootDir(t.TempDir()))
	if _, err := memStore.UpsertRecord(memory.UpsertRecordInput{
		Subject:        "agent-a",
		Scope:          memory.Scope("agent:agent-a"),
		ContentSummary: "fusion memory test document",
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	svc := lifecycle.NewService(baseagent.NoopTriager{}, lifecycle.WithMemoryStore(memStore))
	ready := &atomic.Bool{}
	ready.Store(true)
	mux := buildHTTPMuxWithBaseAgent(svc, nil, ready, api.NewPairingCodeStore(nil), ratelimit.New())

	req := httptest.NewRequest(http.MethodPost, "/api/v2/memory/search", strings.NewReader(`{"subject":"agent-a","query":"fusion"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Results []memory.SearchHit `json:"results"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(payload.Results) == 0 {
		t.Fatalf("expected non-empty search results")
	}
}

func TestMemoryV2UpsertGetGrantRevokeAndInstanceAttach(t *testing.T) {
	root := t.TempDir()
	memStore := memory.NewStore(memory.WithRootDir(root))
	svc := lifecycle.NewService(baseagent.NoopTriager{}, lifecycle.WithMemoryStore(memStore))
	ready := &atomic.Bool{}
	ready.Store(true)
	mux := buildHTTPMuxWithBaseAgent(svc, nil, ready, api.NewPairingCodeStore(nil), ratelimit.New())

	// Upsert agent-private record.
	upsertReq := httptest.NewRequest(http.MethodPost, "/api/v2/memory/records/upsert", strings.NewReader(`{
		"subject":"agent-a",
		"scope":"agent:agent-a",
		"type":"fact",
		"contentSummary":"deployment region is tokyo",
		"provenance":"manual"
	}`))
	upsertRR := httptest.NewRecorder()
	mux.ServeHTTP(upsertRR, upsertReq)
	if upsertRR.Code != http.StatusOK {
		t.Fatalf("upsert status=%d body=%s", upsertRR.Code, upsertRR.Body.String())
	}
	var upsertResp struct {
		Record memory.MemoryRecord `json:"record"`
	}
	if err := json.Unmarshal(upsertRR.Body.Bytes(), &upsertResp); err != nil {
		t.Fatalf("decode upsert: %v", err)
	}
	if upsertResp.Record.ID == "" {
		t.Fatalf("expected record id")
	}

	// Get record as owner.
	getReq := httptest.NewRequest(http.MethodPost, "/api/v2/memory/get", strings.NewReader(`{"subject":"agent-a","id":"`+upsertResp.Record.ID+`"}`))
	getRR := httptest.NewRecorder()
	mux.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", getRR.Code, getRR.Body.String())
	}

	// Owner writes shared scope should fail before grant.
	sharedUpsertBody := `{
		"subject":"agent-a",
		"scope":"shared:profile",
		"type":"note",
		"contentSummary":"team language is zh-CN"
	}`
	sharedUpsertReq := httptest.NewRequest(http.MethodPost, "/api/v2/memory/records/upsert", strings.NewReader(sharedUpsertBody))
	sharedUpsertRR := httptest.NewRecorder()
	mux.ServeHTTP(sharedUpsertRR, sharedUpsertReq)
	if sharedUpsertRR.Code == http.StatusOK {
		t.Fatalf("expected shared upsert to fail before grant")
	}

	// Grant shared scope.
	grantReq := httptest.NewRequest(http.MethodPost, "/api/v2/memory/grants/grant", strings.NewReader(`{
		"subject":"agent-a",
		"scope":"shared:profile",
		"grantedBy":"tester",
		"reason":"allow team profile sync"
	}`))
	grantRR := httptest.NewRecorder()
	mux.ServeHTTP(grantRR, grantReq)
	if grantRR.Code != http.StatusOK {
		t.Fatalf("grant status=%d body=%s", grantRR.Code, grantRR.Body.String())
	}
	var grantResp struct {
		Grant memory.Grant `json:"grant"`
	}
	if err := json.Unmarshal(grantRR.Body.Bytes(), &grantResp); err != nil {
		t.Fatalf("decode grant: %v", err)
	}
	if grantResp.Grant.ID == "" {
		t.Fatalf("expected grant id")
	}

	// Shared upsert should now succeed.
	sharedUpsertReq = httptest.NewRequest(http.MethodPost, "/api/v2/memory/records/upsert", strings.NewReader(sharedUpsertBody))
	sharedUpsertRR = httptest.NewRecorder()
	mux.ServeHTTP(sharedUpsertRR, sharedUpsertReq)
	if sharedUpsertRR.Code != http.StatusOK {
		t.Fatalf("shared upsert after grant status=%d body=%s", sharedUpsertRR.Code, sharedUpsertRR.Body.String())
	}

	// Revoke and confirm shared write denied again.
	revokeReq := httptest.NewRequest(http.MethodPost, "/api/v2/memory/grants/revoke", strings.NewReader(`{"grantId":"`+grantResp.Grant.ID+`","revokedBy":"tester"}`))
	revokeRR := httptest.NewRecorder()
	mux.ServeHTTP(revokeRR, revokeReq)
	if revokeRR.Code != http.StatusOK {
		t.Fatalf("revoke status=%d body=%s", revokeRR.Code, revokeRR.Body.String())
	}
	sharedUpsertReq = httptest.NewRequest(http.MethodPost, "/api/v2/memory/records/upsert", strings.NewReader(sharedUpsertBody))
	sharedUpsertRR = httptest.NewRecorder()
	mux.ServeHTTP(sharedUpsertRR, sharedUpsertReq)
	if sharedUpsertRR.Code == http.StatusOK {
		t.Fatalf("expected shared upsert to fail after revoke")
	}

	// Instance attach/detach.
	attachReq := httptest.NewRequest(http.MethodPost, "/api/v2/memory/instance/attach", strings.NewReader(`{"instanceId":"agent-a","scope":"shared:profile"}`))
	attachRR := httptest.NewRecorder()
	mux.ServeHTTP(attachRR, attachReq)
	if attachRR.Code != http.StatusOK {
		t.Fatalf("instance attach status=%d body=%s", attachRR.Code, attachRR.Body.String())
	}
	detachReq := httptest.NewRequest(http.MethodPost, "/api/v2/memory/instance/detach", strings.NewReader(`{"instanceId":"agent-a","scope":"shared:profile"}`))
	detachRR := httptest.NewRecorder()
	mux.ServeHTTP(detachRR, detachReq)
	if detachRR.Code != http.StatusOK {
		t.Fatalf("instance detach status=%d body=%s", detachRR.Code, detachRR.Body.String())
	}

	// Instance import/export.
	src := filepath.Join(t.TempDir(), "seed.md")
	if err := os.WriteFile(src, []byte("# seed\nhello"), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	importReq := httptest.NewRequest(http.MethodPost, "/api/v2/memory/instance/import", strings.NewReader(`{
		"instanceId":"agent-a",
		"path":"`+src+`",
		"targetScope":"agent:agent-a",
		"actor":"tester",
		"requestId":"req-import"
	}`))
	importRR := httptest.NewRecorder()
	mux.ServeHTTP(importRR, importReq)
	if importRR.Code != http.StatusOK {
		t.Fatalf("instance import status=%d body=%s", importRR.Code, importRR.Body.String())
	}

	exportReq := httptest.NewRequest(http.MethodPost, "/api/v2/memory/instance/export", strings.NewReader(`{
		"instanceId":"agent-a",
		"format":"truth-only",
		"actor":"tester",
		"requestId":"req-export"
	}`))
	exportRR := httptest.NewRecorder()
	mux.ServeHTTP(exportRR, exportReq)
	if exportRR.Code != http.StatusOK {
		t.Fatalf("instance export status=%d body=%s", exportRR.Code, exportRR.Body.String())
	}
}
