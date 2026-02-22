package runtime

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Mock helpers
// ---------------------------------------------------------------------------

type mockRunner struct {
	lookPathFn       func(name string) (string, error)
	runFn            func(ctx context.Context, name string, args ...string) ([]byte, error)
	runInteractiveFn func(ctx context.Context, stdout, stderr io.Writer, name string, args ...string) error
}

func (m *mockRunner) LookPath(name string) (string, error) {
	if m.lookPathFn != nil {
		return m.lookPathFn(name)
	}
	return "/usr/bin/" + name, nil
}

func (m *mockRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if m.runFn != nil {
		return m.runFn(ctx, name, args...)
	}
	return []byte("1.0.0\n"), nil
}

func (m *mockRunner) RunInteractive(ctx context.Context, stdout, stderr io.Writer, name string, args ...string) error {
	if m.runInteractiveFn != nil {
		return m.runInteractiveFn(ctx, stdout, stderr, name, args...)
	}
	return nil
}

type mockStdinReader struct {
	line string
	err  error
}

func (m *mockStdinReader) ReadLine() (string, error) {
	return m.line, m.err
}

// ---------------------------------------------------------------------------
// detectBunWith tests
// ---------------------------------------------------------------------------

func TestDetectBunWith_Success(t *testing.T) {
	r := &mockRunner{
		lookPathFn: func(string) (string, error) { return "/usr/local/bin/bun", nil },
		runFn:      func(_ context.Context, _ string, _ ...string) ([]byte, error) { return []byte("1.2.3\n"), nil },
	}
	path, version, err := detectBunWith(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "/usr/local/bin/bun" {
		t.Errorf("path = %q, want /usr/local/bin/bun", path)
	}
	if version != "1.2.3" {
		t.Errorf("version = %q, want 1.2.3", version)
	}
}

func TestDetectBunWith_NotFound(t *testing.T) {
	r := &mockRunner{
		lookPathFn: func(string) (string, error) { return "", fmt.Errorf("not found") },
	}
	_, _, err := detectBunWith(r)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); !strings.Contains(got, "bun not found in PATH") {
		t.Errorf("error = %q, want to contain 'bun not found in PATH'", got)
	}
}

func TestDetectBunWith_VersionFails(t *testing.T) {
	r := &mockRunner{
		lookPathFn: func(string) (string, error) { return "/usr/bin/bun", nil },
		runFn:      func(_ context.Context, _ string, _ ...string) ([]byte, error) { return nil, fmt.Errorf("exec error") },
	}
	path, version, err := detectBunWith(r)
	if err == nil {
		t.Fatal("expected error")
	}
	if path != "/usr/bin/bun" {
		t.Errorf("path = %q, want /usr/bin/bun", path)
	}
	if version != "" {
		t.Errorf("version = %q, want empty", version)
	}
}

// ---------------------------------------------------------------------------
// installBunWith tests
// ---------------------------------------------------------------------------

func TestInstallBunWith_Success(t *testing.T) {
	r := &mockRunner{}
	if err := installBunWith(r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInstallBunWith_Failure(t *testing.T) {
	r := &mockRunner{
		runInteractiveFn: func(_ context.Context, _, _ io.Writer, _ string, _ ...string) error {
			return fmt.Errorf("curl failed")
		},
	}
	err := installBunWith(r)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "bun install failed") {
		t.Errorf("error = %q, want to contain 'bun install failed'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// ensureBunWith tests
// ---------------------------------------------------------------------------

func TestEnsureBunWith_Found(t *testing.T) {
	r := &mockRunner{}
	path, err := ensureBunWith(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty path")
	}
}

func TestEnsureBunWith_NotFound(t *testing.T) {
	r := &mockRunner{
		lookPathFn: func(string) (string, error) { return "", fmt.Errorf("missing") },
	}
	_, err := ensureBunWith(r)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "bun runtime not found") {
		t.Errorf("error = %q, want to contain 'bun runtime not found'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// promptAndInstallBunWith tests
// ---------------------------------------------------------------------------

func TestPromptAndInstallBunWith_AlreadyInstalled(t *testing.T) {
	r := &mockRunner{}
	reader := &mockStdinReader{line: ""}
	path, err := promptAndInstallBunWith(r, reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty path")
	}
}

func TestPromptAndInstallBunWith_UserAcceptsY(t *testing.T) {
	callCount := 0
	r := &mockRunner{
		lookPathFn: func(string) (string, error) {
			callCount++
			if callCount == 1 {
				return "", fmt.Errorf("not found")
			}
			return "/home/user/.bun/bin/bun", nil
		},
		runFn: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return []byte("1.0.0\n"), nil
		},
	}
	reader := &mockStdinReader{line: "y"}
	path, err := promptAndInstallBunWith(r, reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "/home/user/.bun/bin/bun" {
		t.Errorf("path = %q", path)
	}
}

func TestPromptAndInstallBunWith_UserAcceptsEmpty(t *testing.T) {
	callCount := 0
	r := &mockRunner{
		lookPathFn: func(string) (string, error) {
			callCount++
			if callCount == 1 {
				return "", fmt.Errorf("not found")
			}
			return "/usr/bin/bun", nil
		},
	}
	reader := &mockStdinReader{line: ""}
	path, err := promptAndInstallBunWith(r, reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty path")
	}
}

func TestPromptAndInstallBunWith_UserAcceptsYes(t *testing.T) {
	callCount := 0
	r := &mockRunner{
		lookPathFn: func(string) (string, error) {
			callCount++
			if callCount == 1 {
				return "", fmt.Errorf("not found")
			}
			return "/usr/bin/bun", nil
		},
	}
	reader := &mockStdinReader{line: "yes"}
	_, err := promptAndInstallBunWith(r, reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPromptAndInstallBunWith_UserDeclines(t *testing.T) {
	r := &mockRunner{
		lookPathFn: func(string) (string, error) { return "", fmt.Errorf("not found") },
	}
	reader := &mockStdinReader{line: "n"}
	_, err := promptAndInstallBunWith(r, reader)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "declined") {
		t.Errorf("error = %q, want to contain 'declined'", err.Error())
	}
}

func TestPromptAndInstallBunWith_UserDeclinesOther(t *testing.T) {
	r := &mockRunner{
		lookPathFn: func(string) (string, error) { return "", fmt.Errorf("not found") },
	}
	reader := &mockStdinReader{line: "no"}
	_, err := promptAndInstallBunWith(r, reader)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPromptAndInstallBunWith_ReadLineError(t *testing.T) {
	r := &mockRunner{
		lookPathFn: func(string) (string, error) { return "", fmt.Errorf("not found") },
	}
	reader := &mockStdinReader{line: "", err: fmt.Errorf("EOF")}
	_, err := promptAndInstallBunWith(r, reader)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "failed to read input") {
		t.Errorf("error = %q, want to contain 'failed to read input'", err.Error())
	}
}

func TestPromptAndInstallBunWith_InstallFails(t *testing.T) {
	r := &mockRunner{
		lookPathFn: func(string) (string, error) { return "", fmt.Errorf("not found") },
		runInteractiveFn: func(_ context.Context, _, _ io.Writer, _ string, _ ...string) error {
			return fmt.Errorf("network error")
		},
	}
	reader := &mockStdinReader{line: "y"}
	_, err := promptAndInstallBunWith(r, reader)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "bun install failed") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestPromptAndInstallBunWith_InstallSuccessButDetectFails(t *testing.T) {
	r := &mockRunner{
		lookPathFn: func(string) (string, error) { return "", fmt.Errorf("not found") },
	}
	reader := &mockStdinReader{line: "y"}
	_, err := promptAndInstallBunWith(r, reader)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "restart your shell") {
		t.Errorf("error = %q, want to contain 'restart your shell'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// Public API integration tests (using real exec, skipped if bun missing)
// ---------------------------------------------------------------------------

func TestDetectBun_ReturnsErrorWhenMissing(t *testing.T) {
	t.Setenv("PATH", "/nonexistent")
	_, _, err := DetectBun()
	if err == nil {
		t.Fatal("expected error when bun is not in PATH")
	}
}

func TestDetectBun_FindsBunIfPresent(t *testing.T) {
	if _, err := exec.LookPath("bun"); err != nil {
		t.Skip("bun not installed, skipping")
	}
	path, version, err := DetectBun()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty path")
	}
	if version == "" {
		t.Error("expected non-empty version")
	}
}

func TestEnsureBun_ReturnsErrorWhenMissing(t *testing.T) {
	t.Setenv("PATH", "/nonexistent")
	_, err := EnsureBun()
	if err == nil {
		t.Fatal("expected error when bun is not in PATH")
	}
}

// ---------------------------------------------------------------------------
// Public wrapper tests (swap defaultRunner/defaultStdinReader)
// ---------------------------------------------------------------------------

func TestInstallBun_WithMock(t *testing.T) {
	old := defaultRunner
	defer func() { defaultRunner = old }()
	defaultRunner = &mockRunner{}

	if err := InstallBun(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPromptAndInstallBun_WithMock(t *testing.T) {
	oldR := defaultRunner
	oldS := defaultStdinReader
	defer func() { defaultRunner = oldR; defaultStdinReader = oldS }()

	defaultRunner = &mockRunner{}
	defaultStdinReader = &mockStdinReader{line: ""}

	path, err := PromptAndInstallBun()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty path")
	}
}
