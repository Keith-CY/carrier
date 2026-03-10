package lifecycle

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestBuildManagedIsolationFileSyncCommandWritesIntoExpandedHomePath(t *testing.T) {
	home := t.TempDir()
	raw := []byte("default_provider = \"openrouter\"\n[gateway]\nport = 9091\n")
	command, err := buildManagedIsolationFileSyncCommand("$HOME/.zeroclaw", "$HOME/.zeroclaw/config.toml", raw)
	if err != nil {
		t.Fatalf("buildManagedIsolationFileSyncCommand: %v", err)
	}

	cmd := exec.Command("sh", "-lc", command)
	cmd.Env = append(os.Environ(), "HOME="+home)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run sync command: %v\n%s", err, string(output))
	}

	got, err := os.ReadFile(filepath.Join(home, ".zeroclaw", "config.toml"))
	if err != nil {
		t.Fatalf("read synced file: %v", err)
	}
	if string(got) != string(raw) {
		t.Fatalf("synced file = %q, want %q", string(got), string(raw))
	}

	if _, err := os.Stat(filepath.Join(home, "$HOME")); !os.IsNotExist(err) {
		t.Fatalf("expected no literal $HOME directory, stat err=%v", err)
	}
}
