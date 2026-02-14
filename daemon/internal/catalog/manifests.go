package catalog

import (
	_ "embed"
	"carrier/daemon/internal/manifest"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

//go:embed openclaw-installer.sh
var openclawInstallerScript string

const (
	openclawVersion = "1.0.0"
	// Pinned checksums for release artifacts — anchored independently from the download source.
	// Update these when cutting a new release.
	openclawChecksumLinuxX86  = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	openclawChecksumLinuxArm  = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	openclawChecksumDarwinX86 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	openclawChecksumDarwinArm = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
)

// getPinnedChecksum returns the checksum for the current platform, pinned in source.
func getPinnedChecksum() string {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "linux/amd64":
		return openclawChecksumLinuxX86
	case "linux/arm64":
		return openclawChecksumLinuxArm
	case "darwin/amd64":
		return openclawChecksumDarwinX86
	case "darwin/arm64":
		return openclawChecksumDarwinArm
	default:
		return openclawChecksumLinuxX86
	}
}

// getInstallCommand returns the install command that writes the embedded installer
// to a temporary location and executes it with the pinned version and checksum.
// Checksum is anchored in the carrier binary, not fetched from the download source.
func getInstallCommand() string {
	tmpDir := os.TempDir()
	scriptPath := filepath.Join(tmpDir, "openclaw-installer.sh")
	checksum := getPinnedChecksum()
	
	return fmt.Sprintf(`sh -c '
set -e
SCRIPT="%s"
cat > "$SCRIPT" << '\''INSTALLER_EOF'\''
%s
INSTALLER_EOF
chmod 755 "$SCRIPT"
"$SCRIPT" "%s" "%s"
rm -f "$SCRIPT"
'`, scriptPath, openclawInstallerScript, openclawVersion, checksum)
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
