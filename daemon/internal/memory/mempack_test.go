package memory

import (
	"archive/zip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func newMemoryStoreWithRoot(t *testing.T) (*Store, string) {
	t.Helper()
	root := t.TempDir()
	fixed := time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC)
	s := NewStore(WithRootDir(root), WithNow(func() time.Time { return fixed }))
	return s, root
}

func writeMempack(t *testing.T, filePath string, manifest string, files map[string]string) {
	t.Helper()
	f, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("create mempack: %v", err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	w, err := zw.Create("memory.yaml")
	if err != nil {
		t.Fatalf("create memory.yaml: %v", err)
	}
	if _, err := io.WriteString(w, manifest); err != nil {
		t.Fatalf("write memory.yaml: %v", err)
	}

	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		ww, err := zw.Create(p)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", p, err)
		}
		if _, err := io.WriteString(ww, files[p]); err != nil {
			t.Fatalf("write zip entry %s: %v", p, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
}

func readZipFile(t *testing.T, zipPath, fileName string) (string, bool) {
	t.Helper()
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer r.Close()
	for _, f := range r.File {
		if f.Name != fileName {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open zip entry %s: %v", fileName, err)
		}
		defer rc.Close()
		b, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read zip entry %s: %v", fileName, err)
		}
		return string(b), true
	}
	return "", false
}

func baseManifest(region string) string {
	return strings.TrimSpace(`
schema_version: "1"
id: team-style
name: Team Style
version: 1.0.0
region: `+region+`
type: mixed
publisher: acme
provenance:
  source: market
  uri: https://example.com/team-style
  digest: sha256:abc123
collections:
  - id: prompts
    path: content/prompts
    sensitivity: medium
    default_mount: /prompts
  - id: kb
    path: content/kb
    sensitivity: low
    default_mount: /kb
mount:
  default_mode: ro
  default_slot: default
`) + "\n"
}

func TestImportMemoryInstallsMempackAndRegistersEntry(t *testing.T) {
	store, root := newMemoryStoreWithRoot(t)
	pack := filepath.Join(t.TempDir(), "team-style.mempack.zip")
	writeMempack(t, pack, baseManifest("public"), map[string]string{
		"README.md":                 "# Team Style",
		"content/prompts/system.md": "public prompt",
		"content/kb/onboarding.txt": "public kb",
		"meta/checksums.sha256":     "deadbeef",
		"meta/signature.ed25519":    "sig",
	})

	entry, err := store.ImportMemory(pack, ImportOptions{TargetRegion: TypePublic, Publisher: "acme"})
	if err != nil {
		t.Fatalf("import memory: %v", err)
	}

	if entry.Type != TypePublic {
		t.Fatalf("expected public type, got %s", entry.Type)
	}
	if entry.State != StateCreated {
		t.Fatalf("expected created state, got %s", entry.State)
	}

	installedPath := filepath.Join(root, "packages", "public", "acme", "team-style@1.0.0", "content", "prompts", "system.md")
	if _, statErr := os.Stat(installedPath); statErr != nil {
		t.Fatalf("expected extracted content file at %s: %v", installedPath, statErr)
	}

	got, getErr := store.Get(entry.ID)
	if getErr != nil {
		t.Fatalf("get imported entry: %v", getErr)
	}
	if got.ID != entry.ID {
		t.Fatalf("get mismatch: want %s got %s", entry.ID, got.ID)
	}
}

func TestAttachAndPrepareAgentMemoryComposesDeterministicView(t *testing.T) {
	store, root := newMemoryStoreWithRoot(t)
	tmp := t.TempDir()

	publicPack := filepath.Join(tmp, "public.mempack.zip")
	writeMempack(t, publicPack, baseManifest("public"), map[string]string{
		"content/prompts/system.md": "public",
	})
	sharedPack := filepath.Join(tmp, "shared.mempack.zip")
	writeMempack(t, sharedPack, strings.ReplaceAll(baseManifest("shared"), "id: team-style", "id: team-shared"), map[string]string{
		"content/prompts/system.md": "shared",
	})
	privatePack := filepath.Join(tmp, "private.mempack.zip")
	writeMempack(t, privatePack, strings.ReplaceAll(baseManifest("private"), "id: team-style", "id: team-private"), map[string]string{
		"content/prompts/system.md": "private",
		"content/profile/me.txt":    "owner notes",
	})

	pub, err := store.ImportMemory(publicPack, ImportOptions{TargetRegion: TypePublic, Publisher: "acme"})
	if err != nil {
		t.Fatalf("import public: %v", err)
	}
	sh, err := store.ImportMemory(sharedPack, ImportOptions{TargetRegion: TypeShared})
	if err != nil {
		t.Fatalf("import shared: %v", err)
	}
	priv, err := store.ImportMemory(privatePack, ImportOptions{TargetRegion: TypePerAgent, Owner: "agent-1"})
	if err != nil {
		t.Fatalf("import private: %v", err)
	}

	if _, err := store.AttachMemory("agent-1", pub.ID, AttachOptions{Mode: AccessReadWrite, Priority: 10}); err != nil {
		t.Fatalf("attach public: %v", err)
	}
	if _, err := store.AttachMemory("agent-1", sh.ID, AttachOptions{Mode: AccessReadOnly, Priority: 20}); err != nil {
		t.Fatalf("attach shared: %v", err)
	}
	if _, err := store.AttachMemory("agent-1", priv.ID, AttachOptions{Mode: AccessReadWrite, Priority: 30}); err != nil {
		t.Fatalf("attach private: %v", err)
	}

	contract, err := store.PrepareAgentMemory("agent-1")
	if err != nil {
		t.Fatalf("prepare agent memory: %v", err)
	}
	if contract.Env["AGENTD_MEMORY_PATH"] != "/app/memory" {
		t.Fatalf("unexpected AGENTD_MEMORY_PATH: %q", contract.Env["AGENTD_MEMORY_PATH"])
	}
	if contract.Env["AGENTD_MEMORY_WRITE_PATH"] != "/app/memory_private" {
		t.Fatalf("unexpected AGENTD_MEMORY_WRITE_PATH: %q", contract.Env["AGENTD_MEMORY_WRITE_PATH"])
	}
	if contract.ViewDigest == "" {
		t.Fatal("expected non-empty view digest")
	}
	if len(contract.Mounts) != 2 {
		t.Fatalf("expected 2 mounts, got %d", len(contract.Mounts))
	}

	effectiveFile := filepath.Join(root, "views", "agent-1", "effective", "prompts", "system.md")
	b, err := os.ReadFile(effectiveFile)
	if err != nil {
		t.Fatalf("read effective file: %v", err)
	}
	if string(b) != "private" {
		t.Fatalf("expected highest-precedence private content, got %q", string(b))
	}

	explain, err := store.ExplainView("agent-1")
	if err != nil {
		t.Fatalf("explain view: %v", err)
	}
	if explain.Digest != contract.ViewDigest {
		t.Fatalf("digest mismatch: prepare=%s explain=%s", contract.ViewDigest, explain.Digest)
	}
	if len(explain.Conflicts) == 0 {
		t.Fatal("expected conflict entries in explain output")
	}

	again, err := store.PrepareAgentMemory("agent-1")
	if err != nil {
		t.Fatalf("prepare memory again: %v", err)
	}
	if again.ViewDigest != contract.ViewDigest {
		t.Fatalf("expected deterministic digest, got %s then %s", contract.ViewDigest, again.ViewDigest)
	}
}

func TestExportMemoryRespectsCollectionFilter(t *testing.T) {
	store, _ := newMemoryStoreWithRoot(t)
	pack := filepath.Join(t.TempDir(), "export-source.mempack.zip")
	writeMempack(t, pack, baseManifest("shared"), map[string]string{
		"content/prompts/system.md": "prompt data",
		"content/kb/internal.txt":   "kb data",
		"README.md":                 "readme",
	})

	entry, err := store.ImportMemory(pack, ImportOptions{TargetRegion: TypeShared})
	if err != nil {
		t.Fatalf("import memory: %v", err)
	}

	exportPath, err := store.ExportMemory(entry.ID, ExportOptions{Collections: []string{"prompts"}})
	if err != nil {
		t.Fatalf("export memory: %v", err)
	}

	if _, ok := readZipFile(t, exportPath, "memory.yaml"); !ok {
		t.Fatal("expected memory.yaml in exported mempack")
	}
	if _, ok := readZipFile(t, exportPath, "content/prompts/system.md"); !ok {
		t.Fatal("expected prompts content in exported mempack")
	}
	if _, ok := readZipFile(t, exportPath, "content/kb/internal.txt"); ok {
		t.Fatal("did not expect filtered-out kb content in exported mempack")
	}
}

func TestAttachPolicyForcedReadOnlyAndPerAgentLimit(t *testing.T) {
	store, _ := newMemoryStoreWithRoot(t)
	tmp := t.TempDir()

	publicPack := filepath.Join(tmp, "pub.mempack.zip")
	writeMempack(t, publicPack, baseManifest("public"), map[string]string{"content/prompts/system.md": "public"})
	privatePackA := filepath.Join(tmp, "priv-a.mempack.zip")
	writeMempack(t, privatePackA, strings.ReplaceAll(baseManifest("private"), "id: team-style", "id: private-a"), map[string]string{"content/prompts/system.md": "a"})
	privatePackB := filepath.Join(tmp, "priv-b.mempack.zip")
	writeMempack(t, privatePackB, strings.ReplaceAll(baseManifest("private"), "id: team-style", "id: private-b"), map[string]string{"content/prompts/system.md": "b"})

	pub, _ := store.ImportMemory(publicPack, ImportOptions{TargetRegion: TypePublic, Publisher: "acme"})
	privA, _ := store.ImportMemory(privatePackA, ImportOptions{TargetRegion: TypePerAgent, Owner: "agent-1"})
	privB, _ := store.ImportMemory(privatePackB, ImportOptions{TargetRegion: TypePerAgent, Owner: "agent-1"})

	att, err := store.AttachMemory("agent-1", pub.ID, AttachOptions{Mode: AccessReadWrite})
	if err != nil {
		t.Fatalf("attach public: %v", err)
	}
	if att.Mode != AccessReadOnly {
		t.Fatalf("expected public to be forced read-only, got %s", att.Mode)
	}

	if _, err := store.AttachMemory("agent-1", privA.ID, AttachOptions{Mode: AccessReadWrite}); err != nil {
		t.Fatalf("attach first private: %v", err)
	}
	if _, err := store.AttachMemory("agent-1", privB.ID, AttachOptions{Mode: AccessReadWrite}); err == nil {
		t.Fatal("expected second private attach to fail")
	}
}

func TestImportInvalidManifestRejected(t *testing.T) {
	store, _ := newMemoryStoreWithRoot(t)
	pack := filepath.Join(t.TempDir(), "invalid.mempack.zip")
	writeMempack(t, pack, "id: missing-version\n", map[string]string{"content/prompts/a.txt": "x"})

	if _, err := store.ImportMemory(pack, ImportOptions{TargetRegion: TypeShared}); err == nil {
		t.Fatal("expected import error for invalid manifest")
	}
}

func TestExplainViewIncludesMountMapFile(t *testing.T) {
	store, root := newMemoryStoreWithRoot(t)
	pack := filepath.Join(t.TempDir(), "shared.mempack.zip")
	writeMempack(t, pack, baseManifest("shared"), map[string]string{
		"content/prompts/system.md": "hello",
	})
	entry, err := store.ImportMemory(pack, ImportOptions{TargetRegion: TypeShared})
	if err != nil {
		t.Fatalf("import memory: %v", err)
	}
	if _, err := store.AttachMemory("agent-1", entry.ID, AttachOptions{}); err != nil {
		t.Fatalf("attach memory: %v", err)
	}
	if _, err := store.PrepareAgentMemory("agent-1"); err != nil {
		t.Fatalf("prepare memory: %v", err)
	}

	explain, err := store.ExplainView("agent-1")
	if err != nil {
		t.Fatalf("explain view: %v", err)
	}
	if explain.MountMapPath == "" {
		t.Fatal("expected mount map path")
	}
	if _, statErr := os.Stat(explain.MountMapPath); statErr != nil {
		t.Fatalf("expected mount map file: %v", statErr)
	}

	raw, err := os.ReadFile(explain.MountMapPath)
	if err != nil {
		t.Fatalf("read mount map: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("mount map should be valid json: %v", err)
	}
	if parsed["agent_id"] != "agent-1" {
		t.Fatalf("unexpected agent_id in mountmap: %#v", parsed["agent_id"])
	}

	effectiveFile := filepath.Join(root, "views", "agent-1", "effective", "prompts", "system.md")
	if _, err := os.Stat(effectiveFile); err != nil {
		t.Fatalf("expected effective view file: %v", err)
	}
}
