package profilesync

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSaveAndRollbackInstanceProfile(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	t.Setenv("CARRIER_PROFILESYNC_REPO", t.TempDir())

	instanceID := "host-a_main"
	hostID := "host-a"
	agentID := "main"

	baseProfile := map[string]interface{}{
		"agents": map[string]interface{}{
			"defaults": map[string]interface{}{
				"model": "gpt-4.1",
			},
		},
	}
	commit1, changed, err := SaveInstanceProfile(instanceID, hostID, agentID, baseProfile, "sync-pull")
	if err != nil {
		t.Fatalf("SaveInstanceProfile commit1 failed: %v", err)
	}
	if !changed || commit1 == "" {
		t.Fatalf("expected first save to produce commit, changed=%v commit=%q", changed, commit1)
	}

	updatedProfile := map[string]interface{}{
		"agents": map[string]interface{}{
			"defaults": map[string]interface{}{
				"model": "gpt-5",
			},
		},
	}
	commit2, changed, err := SaveInstanceProfile(instanceID, hostID, agentID, updatedProfile, "sync-push")
	if err != nil {
		t.Fatalf("SaveInstanceProfile commit2 failed: %v", err)
	}
	if !changed || commit2 == "" || commit2 == commit1 {
		t.Fatalf("expected second save to produce different commit, changed=%v commit1=%q commit2=%q", changed, commit1, commit2)
	}

	profileAtCommit1, err := LoadInstanceProfileAtCommit(instanceID, commit1)
	if err != nil {
		t.Fatalf("LoadInstanceProfileAtCommit failed: %v", err)
	}
	if got := nestedString(profileAtCommit1, "agents", "defaults", "model"); got != "gpt-4.1" {
		t.Fatalf("expected model gpt-4.1 at commit1, got %q", got)
	}

	rollbackCommit, restored, err := RollbackInstanceProfile(instanceID, hostID, agentID, commit1)
	if err != nil {
		t.Fatalf("RollbackInstanceProfile failed: %v", err)
	}
	if rollbackCommit == "" || rollbackCommit == commit1 || rollbackCommit == commit2 {
		t.Fatalf("expected rollback to create a new commit, got %q", rollbackCommit)
	}
	if got := nestedString(restored, "agents", "defaults", "model"); got != "gpt-4.1" {
		t.Fatalf("expected restored model gpt-4.1, got %q", got)
	}

	latest, headCommit, err := LoadLatestInstanceProfile(instanceID)
	if err != nil {
		t.Fatalf("LoadLatestInstanceProfile failed: %v", err)
	}
	if headCommit != rollbackCommit {
		t.Fatalf("expected head commit %q, got %q", rollbackCommit, headCommit)
	}
	if got := nestedString(latest, "agents", "defaults", "model"); got != "gpt-4.1" {
		t.Fatalf("expected latest model gpt-4.1 after rollback, got %q", got)
	}
}

func TestSaveInstanceProfileNoChangeDoesNotCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	t.Setenv("CARRIER_PROFILESYNC_REPO", t.TempDir())
	instanceID := "host-b_main"
	hostID := "host-b"
	agentID := "main"

	profile := map[string]interface{}{
		"agents": map[string]interface{}{
			"defaults": map[string]interface{}{
				"model": "gpt-4.1-mini",
			},
		},
	}
	commit1, changed, err := SaveInstanceProfile(instanceID, hostID, agentID, profile, "sync-pull")
	if err != nil {
		t.Fatalf("first save failed: %v", err)
	}
	if !changed || commit1 == "" {
		t.Fatalf("expected first save to change state")
	}

	commit2, changed, err := SaveInstanceProfile(instanceID, hostID, agentID, profile, "sync-pull")
	if err != nil {
		t.Fatalf("second save failed: %v", err)
	}
	if changed {
		t.Fatalf("expected second save to have no changes")
	}
	if commit2 != commit1 {
		t.Fatalf("expected second save to keep same commit, commit1=%q commit2=%q", commit1, commit2)
	}
}

func TestSyncInstanceMemoryContractWithRemotePush(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repoRoot := t.TempDir()
	t.Setenv("CARRIER_PROFILESYNC_REPO", repoRoot)
	instanceID := "host-c_main"
	hostID := "host-c"
	agentID := "main"

	if _, _, err := SaveInstanceProfile(instanceID, hostID, agentID, map[string]interface{}{"agents": map[string]interface{}{}}, "seed"); err != nil {
		t.Fatalf("seed profile failed: %v", err)
	}

	remoteBare := filepath.Join(t.TempDir(), "remote.git")
	if err := os.MkdirAll(remoteBare, 0o755); err != nil {
		t.Fatalf("mkdir remote bare: %v", err)
	}
	if out, err := exec.Command("git", "init", "--bare", remoteBare).CombinedOutput(); err != nil {
		t.Fatalf("init bare remote: %v, out=%s", err, string(out))
	}

	contract := map[string]interface{}{
		"agentId":  agentID,
		"hostId":   hostID,
		"mounts":   []interface{}{},
		"attached": []interface{}{},
	}
	commit, changed, err := SyncInstanceMemoryContract(instanceID, contract, remoteBare, "main", "sync")
	if err != nil {
		t.Fatalf("sync memory contract failed: %v", err)
	}
	if !changed || commit == "" {
		t.Fatalf("expected contract sync commit, changed=%v commit=%q", changed, commit)
	}

	if out, err := exec.Command("git", "--git-dir", remoteBare, "rev-parse", "refs/heads/main").CombinedOutput(); err != nil {
		t.Fatalf("expected pushed main branch, err=%v out=%s", err, string(out))
	}

	commit2, changed, err := SyncInstanceMemoryContract(instanceID, contract, remoteBare, "main", "sync")
	if err != nil {
		t.Fatalf("second sync failed: %v", err)
	}
	if changed {
		t.Fatalf("expected no-op sync on identical contract")
	}
	if commit2 != commit {
		t.Fatalf("expected commit to stay stable on no-op sync, commit1=%q commit2=%q", commit, commit2)
	}
}
