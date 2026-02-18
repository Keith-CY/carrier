// Package runtime provides detection and auto-installation of the Bun runtime,
// which is required to run the Carrier gateway.
package runtime

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// DetectBun looks for bun in PATH and returns its path and version.
// Returns an error if bun is not found or version cannot be determined.
func DetectBun() (path string, version string, err error) {
	path, err = exec.LookPath("bun")
	if err != nil {
		return "", "", fmt.Errorf("bun not found in PATH: %w", err)
	}
	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		return path, "", fmt.Errorf("failed to get bun version: %w", err)
	}
	version = strings.TrimSpace(string(out))
	return path, version, nil
}

// InstallBun runs the official Bun installer for the current platform.
// On Linux/macOS it uses curl; on Windows it uses PowerShell.
func InstallBun() error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("powershell", "-c", "irm bun.sh/install.ps1 | iex")
	default: // linux, darwin
		cmd = exec.Command("bash", "-c", "curl -fsSL https://bun.sh/install | bash")
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("bun install failed: %w", err)
	}
	return nil
}

// EnsureBun detects bun and returns its path. If bun is not found, it returns
// an error with instructions. Use PromptAndInstallBun for interactive flows.
func EnsureBun() (string, error) {
	path, version, err := DetectBun()
	if err == nil {
		fmt.Printf("✓ Bun runtime found: %s (%s)\n", path, version)
		return path, nil
	}
	return "", fmt.Errorf("bun runtime not found: %w", err)
}

// PromptAndInstallBun interactively prompts the user to install Bun if it is
// not found. Returns the bun path on success.
func PromptAndInstallBun() (string, error) {
	path, _, err := DetectBun()
	if err == nil {
		return path, nil
	}

	fmt.Println("⚠️  Bun runtime not found. The Carrier gateway requires Bun to run.")
	fmt.Print("Install Bun automatically? [Y/n] ")

	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	if answer != "" && answer != "y" && answer != "yes" {
		return "", fmt.Errorf("bun installation declined by user")
	}

	fmt.Println("Installing Bun...")
	if err := InstallBun(); err != nil {
		return "", err
	}

	// Re-detect after install
	path, version, err := DetectBun()
	if err != nil {
		return "", fmt.Errorf("bun installed but not found in PATH — you may need to restart your shell: %w", err)
	}
	fmt.Printf("✓ Bun %s installed successfully at %s\n", version, path)
	return path, nil
}
