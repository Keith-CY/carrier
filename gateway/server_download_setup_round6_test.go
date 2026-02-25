package gateway

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type failingReadCloser struct{}

func (failingReadCloser) Read(_ []byte) (int, error) {
	return 0, errors.New("read failed")
}

func (failingReadCloser) Close() error { return nil }

func buildDownloadMux(t *testing.T, artifactRoot string) (http.Handler, *DownloadStore, *httptest.Server) {
	t.Helper()
	srv := newMockDaemon(nil)
	dc := NewDaemonClient(srv.URL, "test-token", 5*time.Second)
	sessions := NewSessionStore("", 0, nil)
	t.Cleanup(sessions.Stop)
	downloads := NewDownloadStore(t.TempDir(), nil)
	rl := NewGatewayRateLimiter(100, 1000, time.Minute, nil)
	onboard := NewOnboardStore()
	setup := NewSetupStore()
	cfg := &GatewayConfig{
		APIToken:            "test-gateway-token",
		MaxCommandBodyBytes: 64 * 1024,
		ArtifactRoot:        artifactRoot,
	}
	mux := buildGatewayMux(cfg, dc, sessions, downloads, rl, onboard, setup)
	return mux, downloads, srv
}

func TestHandleDownload_FileMismatchBranch(t *testing.T) {
	tmp := t.TempDir()
	mux, downloads, srv := buildDownloadMux(t, tmp)
	defer srv.Close()

	fileRef := filepath.Join(tmp, "artifact.txt")
	if err := os.WriteFile(fileRef, []byte("payload"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	tok := downloads.Issue(fileRef, 5*time.Minute, false)

	wrongPath := fmt.Sprintf("/downloads/%s/%s", tok.Token, "wrong-name.txt")
	req := httptest.NewRequest(http.MethodGet, wrongPath, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"errorCode":"E_DOWNLOAD_FILE_MISMATCH"`) {
		t.Fatalf("expected E_DOWNLOAD_FILE_MISMATCH, got %s", w.Body.String())
	}
}

func TestHandleDownload_ArtifactRootOutsideBranch(t *testing.T) {
	root := t.TempDir()
	outsideDir := t.TempDir()
	mux, downloads, srv := buildDownloadMux(t, root)
	defer srv.Close()

	outsideFile := filepath.Join(outsideDir, "artifact.txt")
	if err := os.WriteFile(outsideFile, []byte("payload"), 0o644); err != nil {
		t.Fatalf("write outside artifact: %v", err)
	}
	tok := downloads.Issue(outsideFile, 5*time.Minute, false)

	req := httptest.NewRequest(http.MethodGet, downloads.ToDownloadURL(tok), nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"errorCode":"E_DOWNLOAD_NOT_FOUND"`) {
		t.Fatalf("expected E_DOWNLOAD_NOT_FOUND, got %s", w.Body.String())
	}
}

func TestHandleDownload_ReadFileErrorBranch(t *testing.T) {
	mux, downloads, srv := buildDownloadMux(t, "")
	defer srv.Close()

	missingFile := filepath.Join(t.TempDir(), "missing.txt")
	tok := downloads.Issue(missingFile, 5*time.Minute, false)

	req := httptest.NewRequest(http.MethodGet, downloads.ToDownloadURL(tok), nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"errorCode":"E_DOWNLOAD_NOT_FOUND"`) {
		t.Fatalf("expected E_DOWNLOAD_NOT_FOUND, got %s", w.Body.String())
	}
}

func TestHandleDownload_SingleUseFinalizeBranch(t *testing.T) {
	tmp := t.TempDir()
	mux, downloads, srv := buildDownloadMux(t, tmp)
	defer srv.Close()

	fileRef := filepath.Join(tmp, "single-use.txt")
	if err := os.WriteFile(fileRef, []byte("payload"), 0o644); err != nil {
		t.Fatalf("write single-use artifact: %v", err)
	}
	tok := downloads.Issue(fileRef, 5*time.Minute, true)
	url := downloads.ToDownloadURL(tok)

	firstReq := httptest.NewRequest(http.MethodGet, url, nil)
	first := httptest.NewRecorder()
	mux.ServeHTTP(first, firstReq)
	if first.Code != http.StatusOK {
		t.Fatalf("expected first download 200, got %d: %s", first.Code, first.Body.String())
	}

	secondReq := httptest.NewRequest(http.MethodGet, url, nil)
	second := httptest.NewRecorder()
	mux.ServeHTTP(second, secondReq)
	if second.Code != http.StatusNotFound {
		t.Fatalf("expected second download 404 after single-use consume, got %d: %s", second.Code, second.Body.String())
	}
	if !strings.Contains(second.Body.String(), `"errorCode":"E_DOWNLOAD_TOKEN_INVALID"`) {
		t.Fatalf("expected E_DOWNLOAD_TOKEN_INVALID, got %s", second.Body.String())
	}
}

func TestHandleSetupPost_ReadErrorAndInvalidJSONBranches(t *testing.T) {
	setup := NewSetupStore()

	readErrReq := httptest.NewRequest(http.MethodPost, "/api/v1/setup", nil)
	readErrReq.Body = failingReadCloser{}
	readErrRec := httptest.NewRecorder()
	handleSetupPost(readErrRec, readErrReq, "req-setup-read-error", setup)
	if readErrRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for read error, got %d: %s", readErrRec.Code, readErrRec.Body.String())
	}
	if !strings.Contains(readErrRec.Body.String(), `"errorCode":"E_BAD_REQUEST"`) {
		t.Fatalf("expected E_BAD_REQUEST, got %s", readErrRec.Body.String())
	}

	invalidJSONReq := httptest.NewRequest(http.MethodPost, "/api/v1/setup", strings.NewReader(`{"provider":`))
	invalidJSONRec := httptest.NewRecorder()
	handleSetupPost(invalidJSONRec, invalidJSONReq, "req-setup-invalid-json", setup)
	if invalidJSONRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid json, got %d: %s", invalidJSONRec.Code, invalidJSONRec.Body.String())
	}
	if !strings.Contains(invalidJSONRec.Body.String(), `"errorCode":"E_BAD_REQUEST"`) {
		t.Fatalf("expected E_BAD_REQUEST for invalid json, got %s", invalidJSONRec.Body.String())
	}
}
