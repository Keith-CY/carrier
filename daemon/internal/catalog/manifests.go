package catalog

import (
	_ "embed"
	"carrier/daemon/internal/manifest"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed openclaw-installer.sh
var openclawInstallerScript string

const (
	openclawVersion = "1.0.0"
)

// getInstallCommand returns the install command that writes the embedded installer
// to a temporary location and executes it with the pinned version.
// This avoids pipe-to-shell and dynamic shell pipelines.
func getInstallCommand() string {
	tmpDir := os.TempDir()
	scriptPath := filepath.Join(tmpDir, "openclaw-installer.sh")
	
	// Write installer script to temp, execute it with pinned version, then clean up
	// No pipe-to-shell: script is embedded in carrier binary, not fetched remotely
	return fmt.Sprintf(`sh -c '
set -e
SCRIPT="%s"
cat > "$SCRIPT" << '\''INSTALLER_EOF'\''
%s
INSTALLER_EOF
chmod 755 "$SCRIPT"
"$SCRIPT" "%s"
rm -f "$SCRIPT"
'`, scriptPath, openclawInstallerScript, openclawVersion)
}

func OpenClawManifest() manifest.Manifest {
	installCmd := getInstallCommand()
	
	return manifest.Manifest{
		ID:           "openclaw",
		Name:         "OpenClaw",
		Version:      openclawVersion,
		Description:  "Full-featured AI assistant with memory support",
		Capabilities: []string{"chat", "code", "memory"},
		Runtime: manifest.RuntimeSpec{
			Type:    manifest.RuntimeTypeLocalBinary,
			Install: manifest.CommandSpec{Command: installCmd},
			Upgrade: manifest.CommandSpec{Command: installCmd},
			Start:   manifest.CommandSpec{Command: "openclaw gateway start"},
			Stop:    manifest.CommandSpec{Command: "openclaw gateway stop"},
		},
		Network: manifest.NetworkSpec{
			Ports: []manifest.PortSpec{{Name: "http", Port: 8080}},
			Healthcheck: manifest.HealthcheckSpec{
				Type: "http",
				URL:  "http://localhost:8080/health",
			},
		},
		Env: manifest.EnvSpec{
			Required: []manifest.EnvVar{{Name: "OPENAI_API_KEY", Secret: true, Description: "OpenAI API key for LLM access"}},
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
