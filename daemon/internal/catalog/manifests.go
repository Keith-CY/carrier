package catalog

import (
	"carrier/daemon/internal/manifest"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func init() {
	// Validate that install command scripts do not contain their own heredoc
	// delimiters, which could cause early termination and shell injection.
	// See issue #619.
	validateHeredocSafety()
}

func validateHeredocSafety() {
	// Check ZeroClaw dev install for 'SCRIPT' delimiter collision
	zcDev := getZeroClawDevInstallCommand()
	// The heredoc uses 'SCRIPT' as a delimiter. The closing delimiter is on its
	// own line, matching "\nSCRIPT\n". We expect exactly one occurrence of this
	// closing delimiter. If more are found, the embedded script contains the delimiter.
	if strings.Count(zcDev, "\nSCRIPT\n") > 1 {
		panic("catalog: ZeroClaw dev install script contains heredoc delimiter 'SCRIPT'")
	}

	// Check PicoClaw dev install for 'DEVEOF' delimiter collision
	pcDev := getPicoClawDevInstallCommand()
	if strings.Count(pcDev, "DEVEOF") > 2 {
		panic("catalog: PicoClaw dev install script contains heredoc delimiter 'DEVEOF'")
	}
}

const (
	defaultDaemonPort        = 9090
	defaultDaemonHealthzPath = "/healthz"
	defaultDaemonHealthURL   = "http://localhost:9090/healthz"

	// Official OpenClaw installer URLs
	installScriptURL = "https://openclaw.ai/install.sh"
	openclawWSLHint  = "OpenClaw on Windows requires WSL2 with at least one Linux distro. Run: wsl --install -d Ubuntu"

	// PicoClaw release URL pattern
	picoClawReleaseBaseURL = "https://github.com/sipeed/picoclaw/releases/latest/download"
	picoClawReleaseAPIURL  = "https://api.github.com/repos/sipeed/picoclaw/releases/latest"

	// ZeroClaw release URL pattern
	zeroClawReleaseBaseURL = "https://github.com/zeroclaw-labs/zeroclaw/releases/latest/download"
)

func normalizeCatalogGOOS(goos string) string {
	normalized := strings.ToLower(strings.TrimSpace(goos))
	if normalized == "" {
		return runtime.GOOS
	}
	return normalized
}

// getInstallCommand returns the platform-appropriate install command.
// - Linux/macOS: download to a tmpfile, then execute with bash
// - Windows with PowerShell: download to a tmpfile, then execute script file
// - Windows cmd-only hosts: download to a tmpfile, then execute install.cmd
// - Dev mode: creates a long-running placeholder script
func getInstallCommand() string {
	return getInstallCommandForGOOS(runtime.GOOS)
}

func getInstallCommandForGOOS(goos string) string {
	if os.Getenv("CARRIER_DEV_MODE") == "1" {
		return getDevInstallCommand()
	}

	switch normalizeCatalogGOOS(goos) {
	case "windows":
		return resolveWindowsOpenClawInstallCommand(exec.LookPath)
	default:
		return fmt.Sprintf(`sh -c 'set -e; tmp="$(mktemp)"; trap "rm -f \"$tmp\"" EXIT; curl -fsSL --proto "=https" --tlsv1.2 %s -o "$tmp"; bash "$tmp" --no-onboard --no-prompt'`, installScriptURL)
	}
}

func quotePowerShellLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func wrapWindowsWSLBashCommand(shCommand string) string {
	return fmt.Sprintf(`powershell -NoProfile -Command "$ErrorActionPreference='Stop';$distro=(wsl -l -q | %% { $_.Replace([char]0,'').Trim() } | ? { $_ -ne '' } | Select-Object -First 1);if ([string]::IsNullOrWhiteSpace($distro)) { Write-Error %s; exit 1 }; wsl -d $distro -- bash -lc %s"`, quotePowerShellLiteral(openclawWSLHint), quotePowerShellLiteral(shCommand))
}

func resolveWindowsOpenClawInstallCommand(lookPath func(string) (string, error)) string {
	installInWSL := fmt.Sprintf(`set -e; tmp="$(mktemp)"; trap 'rm -f "$tmp"' EXIT; curl -fsSL --proto "=https" --tlsv1.2 %s -o "$tmp"; OPENCLAW_INSTALL_METHOD=npm OPENCLAW_NO_ONBOARD=1 OPENCLAW_NO_PROMPT=1 bash "$tmp" --no-onboard --no-prompt`, installScriptURL)
	if commandExistsOnHost(lookPath, "powershell") || commandExistsOnHost(lookPath, "powershell.exe") {
		return wrapWindowsWSLBashCommand(installInWSL)
	}
	return fmt.Sprintf(`echo %s && exit /b 1`, openclawWSLHint+" PowerShell is required.")
}

func commandExistsOnHost(lookPath func(string) (string, error), name string) bool {
	if lookPath == nil {
		return false
	}
	_, err := lookPath(name)
	return err == nil
}

// getDevInstallCommand creates a placeholder binary for development testing.
// The placeholder handles "gateway start" (long-running), "gateway stop", and version echo.
func getDevInstallCommand() string {
	return `sh -c '
mkdir -p "$HOME/.local/bin"
cat > "$HOME/.local/bin/openclaw" << '"'"'OCDEV'"'"'
#!/bin/sh
if [ "$1" = "gateway" ] && { [ $# -eq 1 ] || [ "$2" = "start" ]; }; then
  echo "OpenClaw dev placeholder running (pid $$)"
  trap "exit 0" TERM INT
  while true; do sleep 1; done
elif [ "$1" = "gateway" ] && [ "$2" = "stop" ]; then
  echo "OpenClaw dev stop"
else
  echo "OpenClaw dev placeholder"
fi
OCDEV
chmod +x "$HOME/.local/bin/openclaw"
echo "Dev placeholder created at $HOME/.local/bin/openclaw" >&2
'`
}

// getStartCommand returns the platform-appropriate start command.
func getStartCommand() string {
	return getStartCommandForGOOS(runtime.GOOS)
}

func getStartCommandForGOOS(goos string) string {
	switch normalizeCatalogGOOS(goos) {
	case "windows":
		startInWSL := `set -e; export PATH="$HOME/.local/bin:$HOME/.npm-global/bin:$PATH"; if command -v openclaw >/dev/null 2>&1; then exec "$(command -v openclaw)" gateway; fi; echo "openclaw executable not found inside WSL (checked PATH)" >&2; exit 127`
		return wrapWindowsWSLBashCommand(startInWSL)
	default:
		// Git installs create a ~/.local/bin wrapper, while npm installs may
		// place openclaw in npm's global bin dir. Resolve both without forcing
		// pre-flight to depend on a single hardcoded absolute path.
		return `sh -c 'if [ -x "$HOME/.local/bin/openclaw" ]; then exec "$HOME/.local/bin/openclaw" gateway; elif [ -x "$HOME/.npm-global/bin/openclaw" ]; then exec "$HOME/.npm-global/bin/openclaw" gateway; elif command -v openclaw >/dev/null 2>&1; then exec "$(command -v openclaw)" gateway; else echo "openclaw executable not found (checked $HOME/.local/bin/openclaw, $HOME/.npm-global/bin/openclaw, and PATH)" >&2; exit 127; fi'`
	}
}

// getStopCommand returns the platform-appropriate stop command.
func getStopCommand() string {
	return getStopCommandForGOOS(runtime.GOOS)
}

func getStopCommandForGOOS(goos string) string {
	switch normalizeCatalogGOOS(goos) {
	case "windows":
		stopInWSL := `set -e; export PATH="$HOME/.local/bin:$HOME/.npm-global/bin:$PATH"; if command -v openclaw >/dev/null 2>&1; then exec "$(command -v openclaw)" gateway stop; fi; echo "openclaw executable not found inside WSL (checked PATH)" >&2; exit 127`
		return wrapWindowsWSLBashCommand(stopInWSL)
	default:
		return `sh -c 'if [ -x "$HOME/.local/bin/openclaw" ]; then exec "$HOME/.local/bin/openclaw" gateway stop; elif [ -x "$HOME/.npm-global/bin/openclaw" ]; then exec "$HOME/.npm-global/bin/openclaw" gateway stop; elif command -v openclaw >/dev/null 2>&1; then exec "$(command -v openclaw)" gateway stop; else echo "openclaw executable not found (checked $HOME/.local/bin/openclaw, $HOME/.npm-global/bin/openclaw, and PATH)" >&2; exit 127; fi'`
	}
}

func getNetworkSpec() manifest.NetworkSpec {
	return manifest.NetworkSpec{
		Healthcheck: manifest.HealthcheckSpec{
			Type: "http",
			URL:  defaultDaemonHealthURL,
		},
	}
}

func OpenClawManifest() manifest.Manifest {
	installCmd := getInstallCommand()
	installByOS := map[string]string{
		manifest.CommandOSLinux:   getInstallCommandForGOOS(manifest.CommandOSLinux),
		manifest.CommandOSDarwin:  getInstallCommandForGOOS(manifest.CommandOSDarwin),
		manifest.CommandOSWindows: getInstallCommandForGOOS(manifest.CommandOSWindows),
	}
	startCmd := getStartCommand()
	startByOS := map[string]string{
		manifest.CommandOSLinux:   getStartCommandForGOOS(manifest.CommandOSLinux),
		manifest.CommandOSDarwin:  getStartCommandForGOOS(manifest.CommandOSDarwin),
		manifest.CommandOSWindows: getStartCommandForGOOS(manifest.CommandOSWindows),
	}
	stopCmd := getStopCommand()
	stopByOS := map[string]string{
		manifest.CommandOSLinux:   getStopCommandForGOOS(manifest.CommandOSLinux),
		manifest.CommandOSDarwin:  getStopCommandForGOOS(manifest.CommandOSDarwin),
		manifest.CommandOSWindows: getStopCommandForGOOS(manifest.CommandOSWindows),
	}

	return manifest.Manifest{
		ID:           "openclaw",
		Name:         "OpenClaw",
		Version:      "latest",
		Description:  "Full-featured AI assistant with memory support",
		Capabilities: []string{"chat", "code", "memory"},
		Runtime: manifest.RuntimeSpec{
			Type: manifest.RuntimeTypeLocalBinary,
			Install: manifest.CommandSpec{
				Command:     installCmd,
				CommandByOS: installByOS,
			},
			Upgrade: manifest.CommandSpec{
				Command:     installCmd,
				CommandByOS: installByOS,
			},
			Start: manifest.CommandSpec{
				Command:     startCmd,
				CommandByOS: startByOS,
			},
			Stop: manifest.CommandSpec{
				Command:     stopCmd,
				CommandByOS: stopByOS,
			},
		},
		Network: getNetworkSpec(),
		Env: manifest.EnvSpec{
			Required: []manifest.EnvVar{},
			Optional: []manifest.EnvVar{{Name: "LOG_LEVEL", Default: "info", Description: "Logging verbosity level"}},
		},
		Memory: manifest.MemorySpec{
			Supports:  []manifest.MemoryType{manifest.MemoryTypePerAgent, manifest.MemoryTypeShared, manifest.MemoryTypePublic},
			MountPath: "./memory",
		},
		Upgrade: manifest.UpgradeSpec{Channel: "stable", Strategy: "in_place_or_reinstall"},
		Health: manifest.HealthSpec{
			IntervalSeconds:   30,
			TimeoutSeconds:    5,
			Retries:           3,
			RestartLoopWindow: 300,
			RestartLoopMax:    5,
		},
		Diagnostics: manifest.Diagnostics{Include: []string{"runtime_logs", "process_state", "env_sanitized"}},
	}
}
