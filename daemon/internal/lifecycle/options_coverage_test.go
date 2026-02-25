package lifecycle

import (
	"testing"
	"time"

	"carrier/baseagent"
	"carrier/daemon/internal/memory"
)

func TestServiceOptionHelpers(t *testing.T) {
	memStore := memory.NewStore(memory.WithRootDir(t.TempDir()))
	idGen := func(prefix string) string { return prefix + "-fixed" }
	ttl := 48 * time.Hour
	logLimit := 777
	auditDir := t.TempDir()

	svc := NewService(
		baseagent.NoopTriager{},
		WithLogLimit(logLimit),
		WithIDGenerator(idGen),
		WithHandoffRetention(ttl),
		WithMemoryStore(memStore),
		WithAuditLogDir(auditDir),
	)

	if svc.logLimit != logLimit {
		t.Fatalf("logLimit=%d want=%d", svc.logLimit, logLimit)
	}
	if svc.idGenerator("x") != "x-fixed" {
		t.Fatalf("idGenerator was not applied")
	}
	if svc.handoffTTL != ttl {
		t.Fatalf("handoffTTL=%v want=%v", svc.handoffTTL, ttl)
	}
	if svc.memoryStore != memStore {
		t.Fatal("memory store option was not applied")
	}
	if svc.auditLogDir != auditDir {
		t.Fatalf("auditLogDir=%q want=%q", svc.auditLogDir, auditDir)
	}
}
