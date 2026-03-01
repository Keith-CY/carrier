package lifecycle

import (
	"context"
	"testing"
)

func TestUninstallClearsIsolationState(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	svc := NewService(nil, WithRunner(&fakeRunner{}), WithRuntimeChecker(&fakeChecker{}))
	if err := svc.RegisterManifest(sampleManifest()); err != nil {
		t.Fatalf("RegisterManifest: %v", err)
	}
	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	svc.mu.Lock()
	state := svc.states["openclaw"]
	state.Isolated = true
	state.LimaInstanceName = "carrier-openclaw-a3f2"
	svc.states["openclaw"] = state
	svc.mu.Unlock()

	if err := svc.Uninstall(context.Background(), "openclaw"); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	_, after, err := svc.getManifestAndState("openclaw")
	if err != nil {
		t.Fatalf("getManifestAndState: %v", err)
	}
	if after.Isolated {
		t.Fatal("expected Isolated=false after uninstall")
	}
	if after.LimaInstanceName != "" {
		t.Fatalf("expected LimaInstanceName to be cleared, got %q", after.LimaInstanceName)
	}
}
