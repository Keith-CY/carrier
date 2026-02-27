package profilesync

import "testing"

func TestDefaultPicoClawArtifactManifest(t *testing.T) {
	t.Parallel()

	manifest := DefaultPicoClawArtifactManifest()
	if manifest.Runtime != "picoclaw" {
		t.Fatalf("expected runtime picoclaw, got %q", manifest.Runtime)
	}
	if !containsString(manifest.TrackedFiles, "~/.picoclaw/config.json") {
		t.Fatalf("expected tracked files to include ~/.picoclaw/config.json")
	}
	if !containsString(manifest.GeneratedFiles, "<workspace>/sessions") {
		t.Fatalf("expected generated files to include sessions")
	}
	if !containsString(manifest.GeneratedFiles, "<workspace>/memory") {
		t.Fatalf("expected generated files to include memory")
	}
	if !containsString(manifest.GeneratedFiles, "<default-workspace>/state") {
		t.Fatalf("expected generated files to include state path")
	}
}

func TestDefaultZeroClawArtifactManifest(t *testing.T) {
	t.Parallel()

	manifest := DefaultZeroClawArtifactManifest()
	if manifest.Runtime != "zeroclaw" {
		t.Fatalf("expected runtime zeroclaw, got %q", manifest.Runtime)
	}
	if !containsString(manifest.TrackedFiles, "~/.zeroclaw/config.toml") {
		t.Fatalf("expected tracked files to include ~/.zeroclaw/config.toml")
	}
	if !containsString(manifest.SecretBearingFiles, "~/.zeroclaw/.secret_key") {
		t.Fatalf("expected secret-bearing files to include ~/.zeroclaw/.secret_key")
	}
	if !containsString(manifest.OptionalTrackedFiles, "~/.zeroclaw/agents.db") {
		t.Fatalf("expected optional tracked files to include agents.db")
	}
}
