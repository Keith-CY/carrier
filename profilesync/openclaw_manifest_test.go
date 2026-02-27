package profilesync

import "testing"

func TestDefaultOpenClawArtifactManifest(t *testing.T) {
	t.Parallel()

	manifest := DefaultOpenClawArtifactManifest()
	if len(manifest.TrackedFiles) == 0 {
		t.Fatalf("expected tracked files to be non-empty")
	}
	required := []string{
		"openclaw.json",
		"AGENTS.md",
		"SOUL.md",
		"TOOLS.md",
		"IDENTITY.md",
		"USER.md",
		"HEARTBEAT.md",
		".openclaw/workspace-state.json",
	}
	for _, path := range required {
		if !containsString(manifest.TrackedFiles, path) {
			t.Fatalf("expected tracked files to include %q", path)
		}
	}
	if !containsString(manifest.OptionalTrackedFiles, "BOOTSTRAP.md") {
		t.Fatalf("expected optional tracked files to include BOOTSTRAP.md")
	}
	if !containsString(manifest.GeneratedFiles, "models.json") {
		t.Fatalf("expected generated files to include models.json")
	}
	if !containsString(manifest.TransientFiles, "*.lock") {
		t.Fatalf("expected transient files to include lock patterns")
	}
	if !containsString(manifest.SecretBearingFiles, "auth-profiles.json") {
		t.Fatalf("expected secret-bearing files to include auth-profiles.json")
	}
}
