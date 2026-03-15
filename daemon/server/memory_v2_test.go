package server

import (
	"context"
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

func TestMemoryV2ListEndpoint(t *testing.T) {
	root := t.TempDir()
	memStore := memory.NewStore(memory.WithRootDir(root))
	if _, err := memStore.Create("public.team.v1", "Team Public Memory", "v1", memory.TypePublic, ""); err != nil {
		t.Fatalf("create public memory: %v", err)
	}
	if _, err := memStore.Create("agent-a.private.v1", "Agent A Private Memory", "v1", memory.TypePerAgent, "agent-a"); err != nil {
		t.Fatalf("create private memory: %v", err)
	}
	if err := memStore.SetAttachmentsFromLinks("agent-a", []string{"public.team.v1", "agent-a.private.v1"}); err != nil {
		t.Fatalf("set attachments: %v", err)
	}
	if _, err := memStore.GrantScope("agent-a", memory.Scope("shared:profile"), "tester", "allow shared profile"); err != nil {
		t.Fatalf("grant scope: %v", err)
	}

	svc := lifecycle.NewService(baseagent.NoopTriager{}, lifecycle.WithMemoryStore(memStore))
	ready := &atomic.Bool{}
	ready.Store(true)
	mux := buildHTTPMuxWithBaseAgent(svc, nil, ready, api.NewPairingCodeStore(nil), ratelimit.New())

	req := httptest.NewRequest(http.MethodGet, "/api/v2/memory?subject=agent-a", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Entries     []memory.Entry      `json:"entries"`
		Attachments []memory.Attachment `json:"attachments"`
		Grants      []memory.Grant      `json:"grants"`
		Audit       []memory.AuditEvent `json:"audit"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(payload.Entries) != 2 {
		t.Fatalf("entries=%d want 2", len(payload.Entries))
	}
	if len(payload.Attachments) != 2 {
		t.Fatalf("attachments=%d want 2", len(payload.Attachments))
	}
	if len(payload.Grants) != 1 {
		t.Fatalf("grants=%d want 1", len(payload.Grants))
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

	distillReq := httptest.NewRequest(http.MethodPost, "/api/v2/memory/instance/distill", strings.NewReader(`{
		"instanceId":"agent-a",
		"dryRun":true,
		"force":true,
		"actor":"tester",
		"requestId":"req-distill"
	}`))
	distillRR := httptest.NewRecorder()
	mux.ServeHTTP(distillRR, distillReq)
	if distillRR.Code != http.StatusOK {
		t.Fatalf("instance distill status=%d body=%s", distillRR.Code, distillRR.Body.String())
	}
	var distillResp struct {
		Result memory.DistillRunResult `json:"result"`
	}
	if err := json.Unmarshal(distillRR.Body.Bytes(), &distillResp); err != nil {
		t.Fatalf("decode distill response: %v", err)
	}
	if strings.TrimSpace(distillResp.Result.RunID) == "" {
		t.Fatalf("expected distill run id")
	}

	// Migration backup/validate/rollback.
	backupReq := httptest.NewRequest(http.MethodPost, "/api/v2/memory/migrate/backup", strings.NewReader(`{
		"actor":"tester",
		"requestId":"req-backup"
	}`))
	backupRR := httptest.NewRecorder()
	mux.ServeHTTP(backupRR, backupReq)
	if backupRR.Code != http.StatusOK {
		t.Fatalf("backup status=%d body=%s", backupRR.Code, backupRR.Body.String())
	}
	var backupResp struct {
		BackupPath string `json:"backupPath"`
	}
	if err := json.Unmarshal(backupRR.Body.Bytes(), &backupResp); err != nil {
		t.Fatalf("decode backup response: %v", err)
	}
	if strings.TrimSpace(backupResp.BackupPath) == "" {
		t.Fatalf("expected backup path")
	}

	validateReq := httptest.NewRequest(http.MethodGet, "/api/v2/memory/migrate/validate", nil)
	validateRR := httptest.NewRecorder()
	mux.ServeHTTP(validateRR, validateReq)
	if validateRR.Code != http.StatusOK {
		t.Fatalf("validate status=%d body=%s", validateRR.Code, validateRR.Body.String())
	}

	rollbackReq := httptest.NewRequest(http.MethodPost, "/api/v2/memory/migrate/rollback", strings.NewReader(`{
		"backupPath":"`+backupResp.BackupPath+`",
		"actor":"tester",
		"requestId":"req-rollback"
	}`))
	rollbackRR := httptest.NewRecorder()
	mux.ServeHTTP(rollbackRR, rollbackReq)
	if rollbackRR.Code != http.StatusOK {
		t.Fatalf("rollback status=%d body=%s", rollbackRR.Code, rollbackRR.Body.String())
	}
}

func TestMemoryV2InstanceAttachRejectsSnapshotScope(t *testing.T) {
	root := t.TempDir()
	memStore := memory.NewStore(memory.WithRootDir(root))
	if _, err := memStore.GrantScope("parent", memory.Scope("shared:team"), "tester", "seed shared team"); err != nil {
		t.Fatalf("GrantScope(shared:team): %v", err)
	}
	if _, err := memStore.UpsertRecord(memory.UpsertRecordInput{
		ID:             "shared-team-1",
		Subject:        "parent",
		Scope:          memory.Scope("shared:team"),
		Type:           memory.RecordTypeFact,
		ContentSummary: "team timezone is tokyo",
	}); err != nil {
		t.Fatalf("UpsertRecord(shared:team): %v", err)
	}
	snapshot, err := memStore.CreateSnapshotForInstance(context.Background(), memory.SnapshotOptions{
		Actor:            "tester",
		RequestID:        "req-snapshot-attach",
		SourceSubject:    "parent",
		SourceScopes:     []memory.Scope{memory.Scope("shared:team")},
		TargetInstanceID: "child",
		Reason:           "delegate task",
	})
	if err != nil {
		t.Fatalf("CreateSnapshotForInstance: %v", err)
	}

	svc := lifecycle.NewService(baseagent.NoopTriager{}, lifecycle.WithMemoryStore(memStore))
	ready := &atomic.Bool{}
	ready.Store(true)
	mux := buildHTTPMuxWithBaseAgent(svc, nil, ready, api.NewPairingCodeStore(nil), ratelimit.New())

	attachReq := httptest.NewRequest(http.MethodPost, "/api/v2/memory/instance/attach", strings.NewReader(`{"instanceId":"observer","scope":"`+string(snapshot.Scope)+`"}`))
	attachRR := httptest.NewRecorder()
	mux.ServeHTTP(attachRR, attachReq)
	if attachRR.Code == http.StatusOK {
		t.Fatalf("expected snapshot attach to fail, body=%s", attachRR.Body.String())
	}
}

func TestMemoryV2DelegatedSnapshotProvisioningEndpoints(t *testing.T) {
	root := t.TempDir()
	memStore := memory.NewStore(memory.WithRootDir(root))
	if _, err := memStore.GrantScope("parent", memory.Scope("shared:team"), "tester", "seed shared team"); err != nil {
		t.Fatalf("GrantScope(shared:team): %v", err)
	}
	if _, err := memStore.UpsertRecord(memory.UpsertRecordInput{
		ID:             "shared-team-1",
		Subject:        "parent",
		Scope:          memory.Scope("shared:team"),
		Type:           memory.RecordTypeFact,
		ContentSummary: "team timezone is tokyo",
	}); err != nil {
		t.Fatalf("UpsertRecord(shared:team): %v", err)
	}

	svc := lifecycle.NewService(baseagent.NoopTriager{}, lifecycle.WithMemoryStore(memStore))
	ready := &atomic.Bool{}
	ready.Store(true)
	mux := buildHTTPMuxWithBaseAgent(svc, nil, ready, api.NewPairingCodeStore(nil), ratelimit.New())

	createReq := httptest.NewRequest(http.MethodPost, "/api/v2/memory/entries/create", strings.NewReader(`{
		"id":"mem-child-1",
		"name":"Delegated Child Memory",
		"version":"v1",
		"type":"per_agent",
		"owner":"child-1"
	}`))
	createRR := httptest.NewRecorder()
	mux.ServeHTTP(createRR, createReq)
	if createRR.Code != http.StatusOK {
		t.Fatalf("create entry status=%d body=%s", createRR.Code, createRR.Body.String())
	}
	var createResp struct {
		Entry memory.Entry `json:"entry"`
	}
	if err := json.Unmarshal(createRR.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("decode create entry response: %v", err)
	}
	if createResp.Entry.ID != "mem-child-1" || createResp.Entry.Owner != "child-1" {
		t.Fatalf("unexpected created entry: %+v", createResp.Entry)
	}

	invalidCreateReq := httptest.NewRequest(http.MethodPost, "/api/v2/memory/entries/create", strings.NewReader(`{
		"id":"mem-invalid",
		"name":"Broken Memory",
		"version":"v1",
		"type":"banana",
		"owner":"child-1"
	}`))
	invalidCreateRR := httptest.NewRecorder()
	mux.ServeHTTP(invalidCreateRR, invalidCreateReq)
	if invalidCreateRR.Code == http.StatusOK {
		t.Fatalf("expected invalid memory type rejection, body=%s", invalidCreateRR.Body.String())
	}

	snapshotReq := httptest.NewRequest(http.MethodPost, "/api/v2/memory/instance/snapshot", strings.NewReader(`{
		"sourceSubject":"parent",
		"sourceScopes":["public","shared:team"],
		"targetInstanceId":"child-1",
		"actor":"tester",
		"requestId":"req-snapshot",
		"reason":"delegate task"
	}`))
	snapshotRR := httptest.NewRecorder()
	mux.ServeHTTP(snapshotRR, snapshotReq)
	if snapshotRR.Code != http.StatusOK {
		t.Fatalf("create snapshot status=%d body=%s", snapshotRR.Code, snapshotRR.Body.String())
	}
	var snapshotResp struct {
		Snapshot struct {
			ID               string       `json:"id"`
			Digest           string       `json:"digest"`
			Scope            memory.Scope `json:"scope"`
			TargetInstanceID string       `json:"targetInstanceId"`
		} `json:"snapshot"`
	}
	if err := json.Unmarshal(snapshotRR.Body.Bytes(), &snapshotResp); err != nil {
		t.Fatalf("decode snapshot response: %v", err)
	}
	if strings.TrimSpace(snapshotResp.Snapshot.ID) == "" {
		t.Fatalf("expected snapshot id in response: %+v", snapshotResp)
	}
	if snapshotResp.Snapshot.TargetInstanceID != "child-1" {
		t.Fatalf("snapshot targetInstanceId = %q, want child-1", snapshotResp.Snapshot.TargetInstanceID)
	}

	mountReq := httptest.NewRequest(http.MethodPost, "/api/v2/memory/instance/snapshot/mount", strings.NewReader(`{
		"instanceId":"child-1",
		"snapshotId":"`+snapshotResp.Snapshot.ID+`"
	}`))
	mountRR := httptest.NewRecorder()
	mux.ServeHTTP(mountRR, mountReq)
	if mountRR.Code != http.StatusOK {
		t.Fatalf("mount snapshot status=%d body=%s", mountRR.Code, mountRR.Body.String())
	}
	scopes := memStore.InstanceScopes("child-1")
	if len(scopes) != 1 || scopes[0] != snapshotResp.Snapshot.Scope {
		t.Fatalf("InstanceScopes(child-1) = %+v, want [%s]", scopes, snapshotResp.Snapshot.Scope)
	}
}
