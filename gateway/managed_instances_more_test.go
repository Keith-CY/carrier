package gateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestManagedInstancesPathFromEnv(t *testing.T) {
	t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(t.TempDir(), "agent-instances.json"))

	path, err := managedInstancesPath()
	if err != nil {
		t.Fatalf("managedInstancesPath error: %v", err)
	}
	if path != os.Getenv("CARRIER_INSTANCE_STORE") {
		t.Fatalf("expected custom path from env, got %q", path)
	}
}

func TestGenerateManagedInstanceID(t *testing.T) {
	originalReader := managedInstanceRandReader
	defer func() {
		managedInstanceRandReader = originalReader
	}()

	managedInstanceRandReader = bytes.NewReader([]byte("\x01\x02\x03\x04"))
	id, err := generateManagedInstanceID("  AgEnT  ")
	if err != nil {
		t.Fatalf("generateManagedInstanceID error: %v", err)
	}
	if got, want := id, "agent-01020304"; got != want {
		t.Fatalf("expected id %q, got %q", want, got)
	}

	managedInstanceRandReader = bytes.NewReader(nil)
	if _, err := generateManagedInstanceID("agent"); err == nil {
		t.Fatalf("expected error on short reader")
	}
}

func TestLoadSaveManagedInstances(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "instances", "instances.json")
	t.Setenv("CARRIER_INSTANCE_STORE", storePath)

	instances, path, err := loadManagedInstances()
	if err != nil {
		t.Fatalf("loadManagedInstances missing file error: %v", err)
	}
	if path != storePath {
		t.Fatalf("expected path %q, got %q", storePath, path)
	}
	if len(instances) != 0 {
		t.Fatalf("expected empty instances, got %d", len(instances))
	}

	if err := os.MkdirAll(filepath.Dir(storePath), 0o700); err != nil {
		t.Fatalf("prepare invalid JSON dir error: %v", err)
	}
	if err := os.WriteFile(storePath, []byte("{bad json}"), 0o600); err != nil {
		t.Fatalf("prepare invalid JSON file error: %v", err)
	}
	if _, _, err := loadManagedInstances(); err == nil {
		t.Fatalf("expected parse error for invalid JSON")
	}
	if err := os.Remove(storePath); err != nil {
		t.Fatalf("cleanup invalid json file error: %v", err)
	}

	if err := saveManagedInstances("", []managedAgentInstance{}); err == nil {
		t.Fatalf("expected empty path save error")
	}
	if err := saveManagedInstances(storePath, []managedAgentInstance{
		{ID: "agent-1", AgentID: "a1", CreatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatalf("saveManagedInstances error: %v", err)
	}

	raw, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatalf("read stored instances error: %v", err)
	}
	var file managedAgentInstanceFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("invalid stored JSON: %v", err)
	}
	if got := len(file.Instances); got != 1 || file.Instances[0].ID != "agent-1" {
		t.Fatalf("unexpected stored instances: %#v", file.Instances)
	}
}

func TestUpsertManagedInstance(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "instances.json")
	t.Setenv("CARRIER_INSTANCE_STORE", storePath)

	if err := upsertManagedInstance(managedAgentInstance{ID: "agent-a", AgentID: "agent-1", Workspace: "w1"}); err != nil {
		t.Fatalf("first upsert error: %v", err)
	}
	if err := upsertManagedInstance(managedAgentInstance{ID: "agent-a", AgentID: "agent-2", Workspace: "w2"}); err != nil {
		t.Fatalf("update upsert error: %v", err)
	}
	if err := upsertManagedInstance(managedAgentInstance{ID: "agent-b", AgentID: "agent-3", Workspace: "w3"}); err != nil {
		t.Fatalf("second insert error: %v", err)
	}

	existing, _, err := loadManagedInstances()
	if err != nil {
		t.Fatalf("loadManagedInstances after upsert error: %v", err)
	}
	if len(existing) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(existing))
	}
	idx := findManagedInstanceIndex(existing, "  AGENT-A ")
	if idx != 0 || existing[idx].Workspace != "w2" || existing[idx].AgentID != "agent-2" {
		t.Fatalf("unexpected updated instance: idx=%d %#v", idx, existing[idx])
	}
	if idx := findManagedInstanceIndexByAgentID(existing, "AGENT-3"); idx != 1 {
		t.Fatalf("expected agent-3 at index 1, got %d", idx)
	}
}

func TestCleanupManagedInstanceFilesAdditionalCases(t *testing.T) {
	testWorkspace := t.TempDir()
	recordFile := filepath.Join(testWorkspace, "record.jsonl")
	configFile := filepath.Join(testWorkspace, "config.toml")
	childDir := filepath.Join(testWorkspace, "state", "chat")
	if err := os.MkdirAll(childDir, 0o700); err != nil {
		t.Fatalf("prepare child dir error: %v", err)
	}
	if err := os.WriteFile(recordFile, []byte("{}"), 0o600); err != nil {
		t.Fatalf("prepare record file error: %v", err)
	}
	if err := os.WriteFile(configFile, []byte("{}"), 0o600); err != nil {
		t.Fatalf("prepare config file error: %v", err)
	}
	nestedFile := filepath.Join(childDir, "runtime.json")
	if err := os.WriteFile(nestedFile, []byte("{}"), 0o600); err != nil {
		t.Fatalf("prepare nested file error: %v", err)
	}

	if err := cleanupManagedInstanceFiles(managedAgentInstance{
		RecordPath: recordFile,
		ConfigPath: configFile,
		Workspace:  testWorkspace,
	}); err != nil {
		t.Fatalf("cleanupManagedInstanceFiles error: %v", err)
	}
	if _, err := os.Stat(recordFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("record file should be removed, err=%v", err)
	}
	if _, err := os.Stat(configFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("config file should be removed, err=%v", err)
	}
	if _, err := os.Stat(childDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("child workspace dir should be removed, err=%v", err)
	}
	if _, err := os.Stat(testWorkspace); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("workspace should be removed, err=%v", err)
	}

	if err := cleanupManagedInstanceFiles(managedAgentInstance{}); err != nil {
		t.Fatalf("empty-path cleanup should be no-op, got: %v", err)
	}
}
