package gateway

import (
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestRemoteKeyDirPathFallbackAndResolve(t *testing.T) {
	t.Setenv("CARRIER_REMOTE_KEY_DIR", "")
	path, err := remoteKeyDirPath()
	if err != nil {
		t.Fatalf("remoteKeyDirPath error: %v", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir error: %v", err)
	}
	expected := filepath.Join(home, ".carrier", "keys")
	if path != expected {
		t.Fatalf("expected fallback remote key dir %q, got %q", expected, path)
	}

	if got, err := resolveRemoteKeyPath("bad ref"); err == nil {
		t.Fatalf("expected invalid key ref to fail, got %q", got)
	}
	if path, err = resolveRemoteKeyPath("  abcdefgh12 "); err != nil {
		t.Fatalf("resolveRemoteKeyPath error: %v", err)
	} else if filepath.Base(path) != "abcdefgh12.pem" {
		t.Fatalf("unexpected resolved path: %q", path)
	}
}

func TestSaveUploadedRemoteKeyFilenameFallback(t *testing.T) {
	rawPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte{1, 2, 3}})
	t.Setenv("CARRIER_REMOTE_KEY_DIR", t.TempDir())

	uploaded, err := saveUploadedRemoteKey(" ", rawPEM)
	if err != nil {
		t.Fatalf("saveUploadedRemoteKey fallback filename error: %v", err)
	}
	if uploaded == nil {
		t.Fatalf("expected uploaded key payload")
	}
	if uploaded.Name != "uploaded.pem" {
		t.Fatalf("expected fallback filename uploaded.pem, got %q", uploaded.Name)
	}
	if uploaded.KeyRef == "" || uploaded.KeyRef != uploaded.ID {
		t.Fatalf("expected keyRef and id match: %#v", uploaded)
	}
}
