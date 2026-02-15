package catalog

import (
	"carrier/daemon/internal/manifest"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

//go:embed openclaw-installer.sh
var openclawInstallerScript string

const (
	openclawVersion          = "1.0.0"
	defaultDaemonPort        = 9090
	defaultDaemonHealthzPath = "/healthz"
	defaultDaemonHealthURL   = "http://localhost:9090/healthz"
)

// Pinned checksums for release artifacts, injected at build time via ldflags:
//
//	go build -ldflags "-X carrier/daemon/internal/catalog.openclawChecksumLinuxX86=abc123..."
//
// If not set (empty), the installer will refuse to run — fail-closed by design.
var (
	openclawChecksumLinuxX86  string
	openclawChecksumLinuxArm  string
	openclawChecksumDarwinX86 string
	openclawChecksumDarwinArm string
)

// getPinnedChecksum returns the build-time-injected checksum for the current platform.
// Returns empty string if not set — caller must handle this as a fatal condition.
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
// to a securely-created temporary file (via mktemp) and executes it with the
// pinned version and checksum. Checksum is anchored in the carrier binary (via
// ldflags at build time), not fetched from the download source.
func getInstallCommand() string {
	checksum := getPinnedChecksum()

	return fmt.Sprintf(`sh -c '
set -e
CHECKSUM="%s"
if [ -z "$CHECKSUM" ]; then
  if [ "${CARRIER_DEV_MODE:-0}" = "1" ]; then
    echo "WARNING: Dev mode enabled - skipping checksum validation" >&2
    echo "Creating placeholder installation for development..." >&2
    mkdir -p "$HOME/.local/bin"
    echo "IyEvYmluL3NoCmlmIFsgIiQxIiA9ICJnYXRld2F5IiBdICYmIFsgIiQyIiA9ICJzdGFydCIgXTsgdGhlbgogIGVjaG8gIk9wZW5DbGF3IGRldiBwbGFjZWhvbGRlciBydW5uaW5nIChwaWQgJCQpIgogIHRyYXAgImV4aXQgMCIgVEVSTSBJTlQKICB3aGlsZSB0cnVlOyBkbyBzbGVlcCAxOyBkb25lCmVsaWYgWyAiJDEiID0gImdhdGV3YXkiIF0gJiYgWyAiJDIiID0gInN0b3AiIF07IHRoZW4KICBlY2hvICJPcGVuQ2xhdyBkZXYgc3RvcCIKZWxzZQogIGVjaG8gIk9wZW5DbGF3IGRldiBwbGFjZWhvbGRlciIKZmkK" | base64 -d > "$HOME/.local/bin/openclaw"
    chmod +x "$HOME/.local/bin/openclaw"
    echo "Dev placeholder created at $HOME/.local/bin/openclaw" >&2
    exit 0
  fi
  echo "FATAL: no pinned checksum for this platform — binary was not built with release ldflags" >&2
  echo "Set CARRIER_DEV_MODE=1 to bypass checksum validation for development" >&2
  exit 1
fi
SCRIPT="$(mktemp /tmp/openclaw-installer.XXXXXX.sh)"
trap "rm -f \"$SCRIPT\"" EXIT
cat > "$SCRIPT" << '\''INSTALLER_EOF'\''
%s
INSTALLER_EOF
chmod 700 "$SCRIPT"
"$SCRIPT" "%s" "$CHECKSUM"
'`, checksum, openclawInstallerScript, openclawVersion)
}

func getNetworkSpec() manifest.NetworkSpec {
	spec := manifest.NetworkSpec{
		Healthcheck: manifest.HealthcheckSpec{
			Type: "http",
			URL:  defaultDaemonHealthURL,
		},
	}
	// Skip port declarations in dev mode to avoid port conflict with the daemon itself
	if os.Getenv("CARRIER_DEV_MODE") != "1" {
		spec.Ports = []manifest.PortSpec{{Name: "http", Port: defaultDaemonPort}}
	}
	return spec
}

func openclawBinPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, ".local", "bin", "openclaw")
}

func OpenClawManifest() manifest.Manifest {
	installCmd := getInstallCommand()
	bin := openclawBinPath()

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
			Start:   manifest.CommandSpec{Command: bin + " gateway start"},
			Stop:    manifest.CommandSpec{Command: bin + " gateway stop"},
		},
		Network: getNetworkSpec(),
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
