package lifecycle

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"carrier/daemon/internal/manifest"
)

func TestUpgradeHelpersCoverageRound9(t *testing.T) {
	svc := NewService(nil)

	t.Run("detect post-upgrade version marker", func(t *testing.T) {
		if got := svc.detectPostUpgradeVersion("openclaw", manifest.Manifest{}, ""); got != "" {
			t.Fatalf("empty output should return empty version, got %q", got)
		}
		out := "some logs\nCARRIER_INSTALLED_VERSION=2.3.4\nmore logs"
		if got := svc.detectPostUpgradeVersion("openclaw", manifest.Manifest{}, out); got != "2.3.4" {
			t.Fatalf("detectPostUpgradeVersion() = %q, want 2.3.4", got)
		}
		if got := svc.detectPostUpgradeVersion("openclaw", manifest.Manifest{}, "ip=127.0.0.1"); got != "" {
			t.Fatalf("non-marker output should return empty version, got %q", got)
		}
	})

	t.Run("format upgrade failure", func(t *testing.T) {
		withBackup := svc.formatUpgradeFailure(errors.New("boom"), "/tmp/backup-file")
		if !strings.Contains(withBackup.Error(), "backup captured at /tmp/backup-file") {
			t.Fatalf("expected backup guidance, got %q", withBackup.Error())
		}
		withoutBackup := svc.formatUpgradeFailure(errors.New("boom"), "")
		if !strings.Contains(withoutBackup.Error(), "no backup was captured") {
			t.Fatalf("expected no-backup guidance, got %q", withoutBackup.Error())
		}
	})

	t.Run("next patch version", func(t *testing.T) {
		got, err := nextPatchVersion("1.2.3")
		if err != nil {
			t.Fatalf("nextPatchVersion success error: %v", err)
		}
		if got != "1.2.4" {
			t.Fatalf("nextPatchVersion = %q, want 1.2.4", got)
		}

		cases := []struct {
			version string
			want    string
		}{
			{version: "1.2", want: "invalid version format"},
			{version: "1.2.x", want: "invalid patch version"},
			{version: "x.2.3", want: "invalid major version"},
			{version: "1.x.3", want: "invalid minor version"},
		}
		for _, tc := range cases {
			if _, err := nextPatchVersion(tc.version); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("nextPatchVersion(%q) error = %v, want contains %q", tc.version, err, tc.want)
			}
		}
	})

	t.Run("env var keys dedupe and sort", func(t *testing.T) {
		m := manifest.Manifest{
			Env: manifest.EnvSpec{
				Required: []manifest.EnvVar{{Name: "BETA"}, {Name: "ALPHA"}},
				Optional: []manifest.EnvVar{{Name: "ALPHA"}, {Name: "GAMMA"}},
			},
		}
		got := svc.envVarKeys(m)
		want := []string{"ALPHA", "BETA", "GAMMA"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("envVarKeys = %#v, want %#v", got, want)
		}
	})
}

func TestStateFileSavePersistedErrorBranchesRound9(t *testing.T) {
	t.Run("nil state file is no-op", func(t *testing.T) {
		var sf *StateFile
		if err := sf.SavePersisted(map[string]PersistedAgentState{"a": {ID: "a"}}); err != nil {
			t.Fatalf("nil SavePersisted should not error, got %v", err)
		}
	})

	t.Run("mkdir failure", func(t *testing.T) {
		tmp := t.TempDir()
		blocker := filepath.Join(tmp, "blocker")
		if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
			t.Fatalf("write blocker: %v", err)
		}
		sf := NewStateFile(filepath.Join(blocker, "state.json"))
		err := sf.SavePersisted(map[string]PersistedAgentState{"a": {ID: "a"}})
		if err == nil || !strings.Contains(err.Error(), "create state directory") {
			t.Fatalf("expected create state directory error, got %v", err)
		}
	})

	t.Run("rename failure cleans temp file", func(t *testing.T) {
		tmp := t.TempDir()
		targetDir := filepath.Join(tmp, "target-as-dir")
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			t.Fatalf("mkdir target dir: %v", err)
		}
		sf := NewStateFile(targetDir)
		err := sf.SavePersisted(map[string]PersistedAgentState{"a": {ID: "a"}})
		if err == nil || !strings.Contains(err.Error(), "atomic rename state file") {
			t.Fatalf("expected atomic rename error, got %v", err)
		}
		if _, statErr := os.Stat(targetDir + ".tmp"); !os.IsNotExist(statErr) {
			t.Fatalf("temporary file should be cleaned up, stat err=%v", statErr)
		}
	})
}

func TestEvidenceSaveToFileErrorBranchRound9(t *testing.T) {
	ev := Evidence{Version: "1.0.0", AgentID: "openclaw"}
	dirPath := t.TempDir()
	err := ev.SaveToFile(dirPath) // writing file to directory path should fail
	if err == nil || !strings.Contains(err.Error(), "write evidence file") {
		t.Fatalf("expected write evidence file error, got %v", err)
	}
}

func TestCappedFileWriterAdditionalBranchesRound9(t *testing.T) {
	t.Run("default max bytes when non-positive", func(t *testing.T) {
		logPath := filepath.Join(t.TempDir(), "cap.log")
		w, err := newCappedFileWriter(logPath, 0)
		if err != nil {
			t.Fatalf("newCappedFileWriter: %v", err)
		}
		defer w.Close()
		if w.maxBytes != defaultMaxLogBytes {
			t.Fatalf("maxBytes = %d, want %d", w.maxBytes, defaultMaxLogBytes)
		}
	})

	t.Run("open file error", func(t *testing.T) {
		dirPath := filepath.Join(t.TempDir(), "dir-as-file")
		if err := os.MkdirAll(dirPath, 0o755); err != nil {
			t.Fatalf("mkdir dir: %v", err)
		}
		if _, err := newCappedFileWriter(dirPath, 16); err == nil {
			t.Fatal("expected error when path is a directory")
		}
	})
}

func TestAuditPersistenceBranchesRound9(t *testing.T) {
	t.Run("persist no-op when audit dir empty", func(t *testing.T) {
		svc := NewService(nil, WithNow(func() time.Time { return time.Date(2026, 2, 25, 10, 0, 0, 0, time.UTC) }))
		svc.auditLogDir = ""
		svc.persistAuditEntry(AuditLog{RequestID: "noop", Action: "test", Result: AuditResultSuccess, Timestamp: time.Now()})
	})

	t.Run("persist mkdir error path", func(t *testing.T) {
		tmp := t.TempDir()
		blocker := filepath.Join(tmp, "audit-blocker")
		if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
			t.Fatalf("write blocker: %v", err)
		}
		svc := NewService(nil, WithAuditLogDir(blocker))
		svc.persistAuditEntry(AuditLog{RequestID: "mkdir-fail", Action: "test", Result: AuditResultSuccess, Timestamp: time.Now()})
	})

	t.Run("persist open-file error path", func(t *testing.T) {
		auditDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(auditDir, "audit.jsonl"), 0o755); err != nil {
			t.Fatalf("prepare path directory: %v", err)
		}
		svc := NewService(nil, WithAuditLogDir(auditDir))
		svc.persistAuditEntry(AuditLog{RequestID: "open-fail", Action: "test", Result: AuditResultSuccess, Timestamp: time.Now()})
	})

	t.Run("rotate rename failure path", func(t *testing.T) {
		auditDir := t.TempDir()
		filePath := filepath.Join(auditDir, "audit.jsonl")
		if err := os.WriteFile(filePath, []byte("seed"), 0o600); err != nil {
			t.Fatalf("write seed file: %v", err)
		}
		if err := os.Truncate(filePath, maxAuditLogBytes+1); err != nil {
			t.Fatalf("truncate file: %v", err)
		}
		rotatedDir := filePath + ".1"
		if err := os.MkdirAll(filepath.Join(rotatedDir, "nested"), 0o755); err != nil {
			t.Fatalf("prepare rotated dir: %v", err)
		}

		svc := NewService(nil, WithAuditLogDir(auditDir))
		svc.rotateAuditLogIfNeeded(filePath)

		if _, err := os.Stat(filePath); err != nil {
			t.Fatalf("audit file should remain on rename failure: %v", err)
		}
		if _, err := os.Stat(filepath.Join(rotatedDir, "nested")); err != nil {
			t.Fatalf("rotated directory should remain intact: %v", err)
		}
	})
}
