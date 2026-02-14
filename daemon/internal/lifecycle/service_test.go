package lifecycle

import (
	"context"
	"testing"

	"carrier/daemon/internal/manifest"
)

func sampleManifest() manifest.Manifest {
	return manifest.Manifest{
		ID:      "openclaw",
		Name:    "OpenClaw",
		Version: "1.0.0",
		Runtime: manifest.RuntimeSpec{
			Type:    manifest.RuntimeTypeLocalBinary,
			Install: manifest.CommandSpec{Command: "./install.sh"},
			Start:   manifest.CommandSpec{Command: "./openclaw start"},
			Stop:    manifest.CommandSpec{Command: "./openclaw stop"},
		},
		Memory: manifest.MemorySpec{
			Supports:  []manifest.MemoryType{manifest.MemoryTypePerAgent, manifest.MemoryTypeShared, manifest.MemoryTypePublic},
			MountPath: "./memory",
		},
	}
}

func TestLifecycleInstallStartStop(t *testing.T) {
	svc := NewService(nil)
	if err := svc.RegisterManifest(sampleManifest()); err != nil {
		t.Fatalf("register manifest: %v", err)
	}

	if err := svc.Install("openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := svc.Start("openclaw"); err != nil {
		t.Fatalf("start: %v", err)
	}

	status, err := svc.Status("openclaw")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Runtime != RuntimeStateRunning {
		t.Fatalf("expected running, got %s", status.Runtime)
	}

	if err := svc.Stop("openclaw"); err != nil {
		t.Fatalf("stop: %v", err)
	}
}

func TestHandleFailure(t *testing.T) {
	svc := NewService(nil)
	if err := svc.RegisterManifest(sampleManifest()); err != nil {
		t.Fatalf("register manifest: %v", err)
	}
	if err := svc.Install("openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}

	result, err := svc.HandleFailure(context.Background(), "openclaw", "port in use")
	if err != nil {
		t.Fatalf("handle failure: %v", err)
	}
	if result.Resolved {
		t.Fatal("expected unresolved in noop triager")
	}
	if !result.RequiresRemoteDiagnosis {
		t.Fatal("expected remote diagnosis requirement")
	}
}
