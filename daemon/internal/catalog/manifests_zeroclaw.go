package catalog

import (
	"carrier/daemon/internal/manifest"
	"fmt"
	"os"
	"runtime"
)

// getZeroClawInstallCommand returns the install command for ZeroClaw.
func getZeroClawInstallCommand() string {
	return getZeroClawInstallCommandForGOOS(runtime.GOOS)
}

func getZeroClawInstallCommandForGOOS(goos string) string {
	if os.Getenv("CARRIER_DEV_MODE") == "1" {
		return getZeroClawDevInstallCommand()
	}
	switch normalizeCatalogGOOS(goos) {
	case "linux":
		return `sh -c '
set -e
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) target="x86_64-unknown-linux-gnu" ;;
  aarch64|arm64) target="aarch64-unknown-linux-gnu" ;;
  armv7l|armv6l) target="armv7-unknown-linux-gnueabihf" ;;
  *) echo "unsupported arch: $arch" >&2; exit 2 ;;
esac
tmp="$(mktemp -d)"
cleanup() { rm -rf "$tmp"; }
trap cleanup EXIT
asset="zeroclaw-${target}.tar.gz"
url="` + zeroClawReleaseBaseURL + `/${asset}"
curl -fsSL "$url" -o "$tmp/zeroclaw.tar.gz"
tar -xzf "$tmp/zeroclaw.tar.gz" -C "$tmp"
bin="$(find "$tmp" -type f -name "zeroclaw*" -perm -u+x | head -n 1)"
[ -n "$bin" ] || { echo "zeroclaw binary not found in release archive" >&2; exit 3; }
mkdir -p "$HOME/.local/bin" "$HOME/.zeroclaw"
install -m 0755 "$bin" "$HOME/.local/bin/zeroclaw"
"$HOME/.local/bin/zeroclaw" --version >/dev/null 2>&1 || true
'`
	default:
		return "cargo install --git https://github.com/theonlyhennygod/zeroclaw.git --force"
	}
}

func getZeroClawDevInstallCommand() string {
	return `sh -c '
mkdir -p "$HOME/.cargo/bin"
cat > "$HOME/.cargo/bin/zeroclaw" << 'SCRIPT'
#!/bin/sh
if [ "$1" = "gateway" ] && [ "$2" = "start" ]; then
  echo "ZeroClaw dev placeholder running (pid $$)"
  trap "exit 0" TERM INT
  while true; do sleep 1; done
elif [ "$1" = "gateway" ] && [ $# -eq 1 ]; then
  echo "ZeroClaw dev placeholder running (pid $$)"
  trap "exit 0" TERM INT
  while true; do sleep 1; done
elif [ "$1" = "status" ]; then
  echo "ZeroClaw dev placeholder: ok"
else
  echo "ZeroClaw dev placeholder"
fi
SCRIPT
chmod +x "$HOME/.cargo/bin/zeroclaw"
echo "Dev placeholder created at $HOME/.cargo/bin/zeroclaw" >&2
'`
}

func getZeroClawStartCommand() string {
	return getZeroClawStartCommandForGOOS(runtime.GOOS)
}

func getZeroClawGatewayCommandForGOOS(goos, gatewayCommand string) string {
	switch normalizeCatalogGOOS(goos) {
	case "windows":
		return "zeroclaw " + gatewayCommand
	default:
		return fmt.Sprintf(`sh -c 'if [ -x "$HOME/.local/bin/zeroclaw" ]; then exec "$HOME/.local/bin/zeroclaw" %s; elif [ -x "$HOME/.cargo/bin/zeroclaw" ]; then exec "$HOME/.cargo/bin/zeroclaw" %s; elif command -v zeroclaw >/dev/null 2>&1; then exec "$(command -v zeroclaw)" %s; else echo "zeroclaw executable not found (checked $HOME/.local/bin/zeroclaw, $HOME/.cargo/bin/zeroclaw, and PATH)" >&2; exit 127; fi'`, gatewayCommand, gatewayCommand, gatewayCommand)
	}
}

func getZeroClawStartCommandForGOOS(goos string) string {
	return getZeroClawGatewayCommandForGOOS(goos, "gateway")
}

func getZeroClawStopCommand() string {
	return getZeroClawStopCommandForGOOS(runtime.GOOS)
}

func getZeroClawStopCommandForGOOS(goos string) string {
	return "signal:term"
}

// ZeroClawManifest returns the manifest for the ZeroClaw agent.
func ZeroClawManifest() manifest.Manifest {
	installCmd := getZeroClawInstallCommand()
	installByOS := map[string]string{
		manifest.CommandOSLinux:   getZeroClawInstallCommandForGOOS(manifest.CommandOSLinux),
		manifest.CommandOSDarwin:  getZeroClawInstallCommandForGOOS(manifest.CommandOSDarwin),
		manifest.CommandOSWindows: getZeroClawInstallCommandForGOOS(manifest.CommandOSWindows),
	}
	startCmd := getZeroClawStartCommand()
	stopCmd := getZeroClawStopCommand()
	startByOS := map[string]string{
		manifest.CommandOSLinux:   getZeroClawStartCommandForGOOS(manifest.CommandOSLinux),
		manifest.CommandOSDarwin:  getZeroClawStartCommandForGOOS(manifest.CommandOSDarwin),
		manifest.CommandOSWindows: getZeroClawStartCommandForGOOS(manifest.CommandOSWindows),
	}
	stopByOS := map[string]string{
		manifest.CommandOSLinux:   getZeroClawStopCommandForGOOS(manifest.CommandOSLinux),
		manifest.CommandOSDarwin:  getZeroClawStopCommandForGOOS(manifest.CommandOSDarwin),
		manifest.CommandOSWindows: getZeroClawStopCommandForGOOS(manifest.CommandOSWindows),
	}

	return manifest.Manifest{
		ID:           "zeroclaw",
		Name:         "ZeroClaw",
		Version:      "latest",
		Description:  "Rust-based AI assistant with chat and code capabilities",
		Capabilities: []string{"chat", "code"},
		Runtime: manifest.RuntimeSpec{
			Type:    manifest.RuntimeTypeLocalBinary,
			Install: manifest.CommandSpec{Command: installCmd, CommandByOS: installByOS},
			Upgrade: manifest.CommandSpec{Command: installCmd, CommandByOS: installByOS},
			Start:   manifest.CommandSpec{Command: startCmd, CommandByOS: startByOS},
			Stop:    manifest.CommandSpec{Command: stopCmd, CommandByOS: stopByOS},
		},
		Network: manifest.NetworkSpec{Healthcheck: manifest.HealthcheckSpec{Type: "process"}},
		Env: manifest.EnvSpec{
			Optional: []manifest.EnvVar{
				{Name: "ZEROCLAW_API_KEY", Secret: true, Description: "API key for LLM provider"},
				{Name: "ZEROCLAW_PROVIDER", Default: "openrouter", Description: "LLM provider name"},
			},
		},
		Upgrade: manifest.UpgradeSpec{Channel: "stable", Strategy: "in_place_or_reinstall"},
		Health: manifest.HealthSpec{
			IntervalSeconds:   30,
			TimeoutSeconds:    5,
			Retries:           3,
			RestartLoopWindow: 300,
			RestartLoopMax:    5,
		},
		Memory: manifest.MemorySpec{
			Supports:  []manifest.MemoryType{manifest.MemoryTypePerAgent},
			MountPath: "./memory",
		},
		Diagnostics: manifest.Diagnostics{Include: []string{"runtime_logs", "process_state", "env_sanitized"}},
	}
}
