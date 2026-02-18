// Package runtime provides detection and auto-installation of the Bun runtime,
// which is required to run the Carrier gateway.
package runtime

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// ---------------------------------------------------------------------------
// Abstractions for testability
// ---------------------------------------------------------------------------

// CommandRunner abstracts command execution so it can be mocked in tests.
type CommandRunner interface {
	// Run executes a command and returns its combined output.
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
	// RunInteractive executes a command with stdout/stderr wired to the
	// provided writers (typically os.Stdout / os.Stderr).
	RunInteractive(ctx context.Context, stdout, stderr io.Writer, name string, args ...string) error
	// LookPath searches for an executable in PATH.
	LookPath(name string) (string, error)
}

// StdinReader abstracts line-reading from stdin.
type StdinReader interface {
	ReadLine() (string, error)
}

// ---------------------------------------------------------------------------
// Default implementations
// ---------------------------------------------------------------------------

// ExecCommandRunner is the default CommandRunner backed by os/exec.
type ExecCommandRunner struct{}

func (r ExecCommandRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

func (r ExecCommandRunner) RunInteractive(_ context.Context, stdout, stderr io.Writer, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func (r ExecCommandRunner) LookPath(name string) (string, error) {
	return exec.LookPath(name)
}

// BufioStdinReader reads lines from os.Stdin.
type BufioStdinReader struct{}

func (r BufioStdinReader) ReadLine() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	return strings.TrimSpace(line), err
}

// Package-level defaults used by the public API functions.
var (
	defaultRunner      CommandRunner = ExecCommandRunner{}
	defaultStdinReader StdinReader   = BufioStdinReader{}
)

// ---------------------------------------------------------------------------
// Internal (testable) implementations
// ---------------------------------------------------------------------------

func detectBunWith(runner CommandRunner) (path string, version string, err error) {
	path, err = runner.LookPath("bun")
	if err != nil {
		return "", "", fmt.Errorf("bun not found in PATH: %w", err)
	}
	out, err := runner.Run(context.Background(), path, "--version")
	if err != nil {
		return path, "", fmt.Errorf("failed to get bun version: %w", err)
	}
	version = strings.TrimSpace(string(out))
	return path, version, nil
}

func installBunWith(runner CommandRunner) error {
	var name string
	var args []string
	switch runtime.GOOS {
	case "windows":
		name = "powershell"
		args = []string{"-c", "irm bun.sh/install.ps1 | iex"}
	default:
		name = "bash"
		args = []string{"-c", "curl -fsSL https://bun.sh/install | bash"}
	}
	if err := runner.RunInteractive(context.Background(), os.Stdout, os.Stderr, name, args...); err != nil {
		return fmt.Errorf("bun install failed: %w", err)
	}
	return nil
}

func ensureBunWith(runner CommandRunner) (string, error) {
	path, version, err := detectBunWith(runner)
	if err == nil {
		fmt.Printf("✓ Bun runtime found: %s (%s)\n", path, version)
		return path, nil
	}
	return "", fmt.Errorf("bun runtime not found: %w", err)
}

func promptAndInstallBunWith(runner CommandRunner, reader StdinReader) (string, error) {
	path, _, err := detectBunWith(runner)
	if err == nil {
		return path, nil
	}

	fmt.Println("⚠️  Bun runtime not found. The Carrier gateway requires Bun to run.")
	fmt.Print("Install Bun automatically? [Y/n] ")

	answer, _ := reader.ReadLine()
	answer = strings.ToLower(answer)

	if answer != "" && answer != "y" && answer != "yes" {
		return "", fmt.Errorf("bun installation declined by user")
	}

	fmt.Println("Installing Bun...")
	if err := installBunWith(runner); err != nil {
		return "", err
	}

	// Re-detect after install
	path, version, err := detectBunWith(runner)
	if err != nil {
		return "", fmt.Errorf("bun installed but not found in PATH — you may need to restart your shell: %w", err)
	}
	fmt.Printf("✓ Bun %s installed successfully at %s\n", version, path)
	return path, nil
}

// ---------------------------------------------------------------------------
// Public API (signatures unchanged for backward compatibility)
// ---------------------------------------------------------------------------

// DetectBun looks for bun in PATH and returns its path and version.
// Returns an error if bun is not found or version cannot be determined.
func DetectBun() (path string, version string, err error) {
	return detectBunWith(defaultRunner)
}

// InstallBun runs the official Bun installer for the current platform.
// On Linux/macOS it uses curl; on Windows it uses PowerShell.
func InstallBun() error {
	return installBunWith(defaultRunner)
}

// EnsureBun detects bun and returns its path. If bun is not found, it returns
// an error with instructions. Use PromptAndInstallBun for interactive flows.
func EnsureBun() (string, error) {
	return ensureBunWith(defaultRunner)
}

// PromptAndInstallBun interactively prompts the user to install Bun if it is
// not found. Returns the bun path on success.
func PromptAndInstallBun() (string, error) {
	return promptAndInstallBunWith(defaultRunner, defaultStdinReader)
}
