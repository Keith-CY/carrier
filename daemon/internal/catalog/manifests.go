package catalog

import (
	"carrier/daemon/internal/manifest"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const (
	defaultDaemonPort        = 9090
	defaultDaemonHealthzPath = "/healthz"
	defaultDaemonHealthURL   = "http://localhost:9090/healthz"

	// Official OpenClaw installer URLs
	installScriptURL = "https://openclaw.ai/install.sh"
	installPS1URL    = "https://openclaw.ai/install.ps1"

	// PicoClaw release URL pattern
	picoClawReleaseBaseURL = "https://github.com/sipeed/picoclaw/releases/latest/download"
)

// getInstallCommand returns the platform-appropriate install command.
// - Linux/macOS/WSL: curl | bash (official install.sh)
// - Windows: PowerShell irm | iex (official install.ps1)
// - Dev mode: creates a long-running placeholder script
func getInstallCommand() string {
	if os.Getenv("CARRIER_DEV_MODE") == "1" {
		return getDevInstallCommand()
	}

	switch runtime.GOOS {
	case "windows":
		return fmt.Sprintf(`powershell -NoProfile -Command "irm '%s' | iex"`, installPS1URL)
	default:
		// Linux, macOS, and anything else that has sh + curl
		return fmt.Sprintf(`sh -c 'curl -fsSL --proto "=https" --tlsv1.2 "%s" | bash'`, installScriptURL)
	}
}

// getDevInstallCommand creates a placeholder binary for development testing.
// The placeholder handles "gateway start" (long-running), "gateway stop", and version echo.
func getDevInstallCommand() string {
	// Base64-encoded placeholder script to avoid quoting issues inside sh -c:
	// #!/bin/sh
	// if [ "$1" = "gateway" ] && [ "$2" = "start" ]; then
	//   echo "OpenClaw dev placeholder running (pid $$)"
	//   trap "exit 0" TERM INT
	//   while true; do sleep 1; done
	// elif [ "$1" = "gateway" ] && [ "$2" = "stop" ]; then
	//   echo "OpenClaw dev stop"
	// else
	//   echo "OpenClaw dev placeholder"
	// fi
	return `sh -c '
mkdir -p "$HOME/.local/bin"
echo "IyEvYmluL3NoCmlmIFsgIiQxIiA9ICJnYXRld2F5IiBdICYmIFsgIiQyIiA9ICJzdGFydCIgXTsgdGhlbgogIGVjaG8gIk9wZW5DbGF3IGRldiBwbGFjZWhvbGRlciBydW5uaW5nIChwaWQgJCQpIgogIHRyYXAgImV4aXQgMCIgVEVSTSBJTlQKICB3aGlsZSB0cnVlOyBkbyBzbGVlcCAxOyBkb25lCmVsaWYgWyAiJDEiID0gImdhdGV3YXkiIF0gJiYgWyAiJDIiID0gInN0b3AiIF07IHRoZW4KICBlY2hvICJPcGVuQ2xhdyBkZXYgc3RvcCIKZWxzZQogIGVjaG8gIk9wZW5DbGF3IGRldiBwbGFjZWhvbGRlciIKZmkK" | base64 -d > "$HOME/.local/bin/openclaw"
chmod +x "$HOME/.local/bin/openclaw"
echo "Dev placeholder created at $HOME/.local/bin/openclaw" >&2
'`
}

// getStartCommand returns the platform-appropriate start command.
func getStartCommand() string {
	switch runtime.GOOS {
	case "windows":
		return "openclaw gateway start"
	default:
		// Use absolute path to avoid PATH shadowing on Unix
		home, err := os.UserHomeDir()
		if err != nil {
			home = os.Getenv("HOME")
		}
		return filepath.Join(home, ".local", "bin", "openclaw") + " gateway start"
	}
}

// getStopCommand returns the platform-appropriate stop command.
func getStopCommand() string {
	switch runtime.GOOS {
	case "windows":
		return "openclaw gateway stop"
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			home = os.Getenv("HOME")
		}
		return filepath.Join(home, ".local", "bin", "openclaw") + " gateway stop"
	}
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

// picoClawBinaryName returns the platform-specific binary name for PicoClaw releases.
func picoClawBinaryName() string {
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	if goos == "windows" {
		return fmt.Sprintf("picoclaw-%s-%s.exe", goos, goarch)
	}
	return fmt.Sprintf("picoclaw-%s-%s", goos, goarch)
}

// getPicoClawInstallCommand returns the install command for PicoClaw.
// It downloads the release binary and sha256sums.txt, verifies the checksum,
// and installs to ~/.local/bin/picoclaw.
func getPicoClawInstallCommand() string {
	if os.Getenv("CARRIER_DEV_MODE") == "1" {
		return getPicoClawDevInstallCommand()
	}

	binary := picoClawBinaryName()
	destName := "picoclaw"
	if runtime.GOOS == "windows" {
		destName = "picoclaw.exe"
	}

	switch runtime.GOOS {
	case "windows":
		return fmt.Sprintf(`powershell -NoProfile -Command "
$ErrorActionPreference = 'Stop'
$bin = '%s'; $dest = \"$env:USERPROFILE\.local\bin\%s\"
New-Item -ItemType Directory -Force -Path (Split-Path $dest) | Out-Null
Invoke-WebRequest -Uri '%s/%s' -OutFile $dest
Invoke-WebRequest -Uri '%s/sha256sums.txt' -OutFile \"$env:TEMP\sha256sums.txt\"
$expected = (Get-Content \"$env:TEMP\sha256sums.txt\" | Select-String $bin).ToString().Split(' ')[0]
$actual = (Get-FileHash $dest -Algorithm SHA256).Hash.ToLower()
if ($actual -ne $expected) { Remove-Item $dest; throw \"checksum mismatch\" }
"`, binary, destName, picoClawReleaseBaseURL, binary, picoClawReleaseBaseURL)
	default:
		return fmt.Sprintf(`sh -c '
set -e
BINARY="%s"
DEST="$HOME/.local/bin/%s"
mkdir -p "$HOME/.local/bin"
curl -fsSL -o "$DEST" "%s/%s"
curl -fsSL -o /tmp/picoclaw-sha256sums.txt "%s/sha256sums.txt"
EXPECTED=$(grep "$BINARY" /tmp/picoclaw-sha256sums.txt | cut -d" " -f1)
ACTUAL=$(sha256sum "$DEST" | cut -d" " -f1)
if [ "$EXPECTED" != "$ACTUAL" ]; then rm -f "$DEST"; echo "checksum mismatch" >&2; exit 1; fi
chmod +x "$DEST"
rm -f /tmp/picoclaw-sha256sums.txt
'`, binary, destName, picoClawReleaseBaseURL, binary, picoClawReleaseBaseURL)
	}
}

// getPicoClawDevInstallCommand creates a placeholder binary for development testing.
func getPicoClawDevInstallCommand() string {
	return `sh -c '
mkdir -p "$HOME/.local/bin"
cat > "$HOME/.local/bin/picoclaw" << '"'"'DEVEOF'"'"'
#!/bin/sh
if [ "$1" = "gateway" ]; then
  echo "PicoClaw dev placeholder running (pid $$)"
  trap "exit 0" TERM INT
  while true; do sleep 1; done
else
  echo "PicoClaw dev placeholder"
fi
DEVEOF
chmod +x "$HOME/.local/bin/picoclaw"
echo "Dev placeholder created at $HOME/.local/bin/picoclaw" >&2
'`
}

// getPicoClawStartCommand returns the start command for PicoClaw.
func getPicoClawStartCommand() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	switch runtime.GOOS {
	case "windows":
		return "picoclaw gateway"
	default:
		return filepath.Join(home, ".local", "bin", "picoclaw") + " gateway"
	}
}

// getPicoClawStopCommand returns "signal:term" to indicate SIGTERM-based stop.
// PicoClaw does not have a stop subcommand; the process is terminated via signal.
func getPicoClawStopCommand() string {
	return "signal:term"
}

// PicoClawManifest returns the manifest for the PicoClaw agent.
func PicoClawManifest() manifest.Manifest {
	return manifest.Manifest{
		ID:           "picoclaw",
		Name:         "PicoClaw",
		Version:      "latest",
		Description:  "Go-based ultra-lightweight AI assistant",
		Capabilities: []string{"chat", "code"},
		Runtime: manifest.RuntimeSpec{
			Type:    manifest.RuntimeTypeLocalBinary,
			Install: manifest.CommandSpec{Command: getPicoClawInstallCommand()},
			Upgrade: manifest.CommandSpec{Command: getPicoClawInstallCommand()},
			Start:   manifest.CommandSpec{Command: getPicoClawStartCommand()},
			Stop:    manifest.CommandSpec{Command: getPicoClawStopCommand()},
		},
		Network: manifest.NetworkSpec{
			Healthcheck: manifest.HealthcheckSpec{
				Type: "process",
			},
		},
		Env: manifest.EnvSpec{
			Required: []manifest.EnvVar{},
			Optional: []manifest.EnvVar{{Name: "LOG_LEVEL", Default: "info", Description: "Logging verbosity level"}},
		},
		Memory: manifest.MemorySpec{
			Supports:  []manifest.MemoryType{manifest.MemoryTypePerAgent},
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
		Diagnostics: manifest.Diagnostics{Include: []string{"runtime_logs", "process_state"}},
	}
}

func OpenClawManifest() manifest.Manifest {
	installCmd := getInstallCommand()

	return manifest.Manifest{
		ID:           "openclaw",
		Name:         "OpenClaw",
		Version:      "latest",
		Description:  "Full-featured AI assistant with memory support",
		Capabilities: []string{"chat", "code", "memory"},
		Runtime: manifest.RuntimeSpec{
			Type:    manifest.RuntimeTypeLocalBinary,
			Install: manifest.CommandSpec{Command: installCmd},
			Upgrade: manifest.CommandSpec{Command: installCmd},
			Start:   manifest.CommandSpec{Command: getStartCommand()},
			Stop:    manifest.CommandSpec{Command: getStopCommand()},
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
