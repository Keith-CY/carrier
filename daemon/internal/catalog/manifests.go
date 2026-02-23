package catalog

import (
	"carrier/daemon/internal/manifest"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	installPS1URL    = "https://openclaw.ai/install.ps1"
	installCMDURL    = "https://openclaw.ai/install.cmd"

	// PicoClaw release URL pattern
	picoClawReleaseBaseURL = "https://github.com/sipeed/picoclaw/releases/latest/download"
	picoClawReleaseAPIURL  = "https://api.github.com/repos/sipeed/picoclaw/releases/latest"
)

// getInstallCommand returns the platform-appropriate install command.
// - Linux/macOS: download to a tmpfile, then execute with bash
// - Windows with PowerShell: download to a tmpfile, then execute script file
// - Windows cmd-only hosts: download to a tmpfile, then execute install.cmd
// - Dev mode: creates a long-running placeholder script
func getInstallCommand() string {
	if os.Getenv("CARRIER_DEV_MODE") == "1" {
		return getDevInstallCommand()
	}

	switch runtime.GOOS {
	case "windows":
		return resolveWindowsOpenClawInstallCommand(exec.LookPath)
	default:
		return fmt.Sprintf(`sh -c 'set -e; tmp="$(mktemp)"; trap "rm -f \"$tmp\"" EXIT; curl -fsSL %s -o "$tmp"; bash "$tmp" --no-onboard --no-prompt'`, installScriptURL)
	}
}

func resolveWindowsOpenClawInstallCommand(lookPath func(string) (string, error)) string {
	if commandExistsOnHost(lookPath, "powershell") || commandExistsOnHost(lookPath, "powershell.exe") {
		return fmt.Sprintf(`powershell -NoProfile -Command "$ErrorActionPreference='Stop';$env:OPENCLAW_INSTALL_METHOD='npm';$env:OPENCLAW_NO_ONBOARD='1';$env:OPENCLAW_NO_PROMPT='1';$tmp=Join-Path $env:TEMP ('openclaw-install-' + [guid]::NewGuid().ToString() + '.ps1');try { iwr -useb %s -OutFile $tmp; & $tmp } finally { Remove-Item $tmp -ErrorAction SilentlyContinue }"`, installPS1URL)
	}
	return fmt.Sprintf(`set "OPENCLAW_NO_ONBOARD=1" && set "OPENCLAW_NO_PROMPT=1" && set "TMPF=%%TEMP%%\openclaw-install-%%RANDOM%%%%RANDOM%%.cmd" && curl -fsSL %s -o "%%TMPF%%" && call "%%TMPF%%" --no-onboard && del "%%TMPF%%"`, installCMDURL)
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
		return "openclaw gateway"
	default:
		// Git installs create a ~/.local/bin wrapper, while npm installs may
		// place openclaw in npm's global bin dir. Resolve both without forcing
		// pre-flight to depend on a single hardcoded absolute path.
		return `sh -c 'if [ -x "$HOME/.local/bin/openclaw" ]; then exec "$HOME/.local/bin/openclaw" gateway; elif [ -x "$HOME/.npm-global/bin/openclaw" ]; then exec "$HOME/.npm-global/bin/openclaw" gateway; elif command -v openclaw >/dev/null 2>&1; then exec "$(command -v openclaw)" gateway; else echo "openclaw executable not found (checked $HOME/.local/bin/openclaw, $HOME/.npm-global/bin/openclaw, and PATH)" >&2; exit 127; fi'`
	}
}

// getStopCommand returns the platform-appropriate stop command.
func getStopCommand() string {
	switch runtime.GOOS {
	case "windows":
		return "openclaw gateway stop"
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

// getZeroClawInstallCommand returns the install command for ZeroClaw.
// ZeroClaw is installed via cargo (Rust toolchain required).
// In dev mode, a placeholder script is created instead.
func getZeroClawInstallCommand() string {
	if os.Getenv("CARRIER_DEV_MODE") == "1" {
		return getZeroClawDevInstallCommand()
	}
	return "cargo install --git https://github.com/theonlyhennygod/zeroclaw.git --force"
}

// getZeroClawDevInstallCommand creates a placeholder binary for development testing.
func getZeroClawDevInstallCommand() string {
	return `sh -c '
mkdir -p "$HOME/.cargo/bin"
cat > "$HOME/.cargo/bin/zeroclaw" << 'SCRIPT'
#!/bin/sh
if [ "$1" = "gateway" ] && [ "$2" = "start" ]; then
  echo "ZeroClaw dev placeholder running (pid $$)"
  trap "exit 0" TERM INT
  while true; do sleep 1; done
elif [ "$1" = "gateway" ] && [ "$2" = "stop" ]; then
  echo "ZeroClaw dev stop"
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

// getZeroClawStartCommand returns the start command for ZeroClaw.
func getZeroClawStartCommand() string {
	switch runtime.GOOS {
	case "windows":
		return "zeroclaw gateway start"
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			home = os.Getenv("HOME")
		}
		return filepath.Join(home, ".cargo", "bin", "zeroclaw") + " gateway start"
	}
}

// getZeroClawStopCommand returns the stop command for ZeroClaw.
func getZeroClawStopCommand() string {
	switch runtime.GOOS {
	case "windows":
		return "zeroclaw gateway stop"
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			home = os.Getenv("HOME")
		}
		return filepath.Join(home, ".cargo", "bin", "zeroclaw") + " gateway stop"
	}
}

// ZeroClawManifest returns the manifest for the ZeroClaw agent.
func ZeroClawManifest() manifest.Manifest {
	installCmd := getZeroClawInstallCommand()

	return manifest.Manifest{
		ID:           "zeroclaw",
		Name:         "ZeroClaw",
		Version:      "latest",
		Description:  "Rust-based AI assistant with chat and code capabilities",
		Capabilities: []string{"chat", "code"},
		Runtime: manifest.RuntimeSpec{
			Type:    manifest.RuntimeTypeLocalBinary,
			Install: manifest.CommandSpec{Command: installCmd},
			Upgrade: manifest.CommandSpec{Command: installCmd},
			Start:   manifest.CommandSpec{Command: getZeroClawStartCommand()},
			Stop:    manifest.CommandSpec{Command: getZeroClawStopCommand()},
		},
		Network: manifest.NetworkSpec{
			Healthcheck: manifest.HealthcheckSpec{
				Type: "process",
			},
		},
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

// picoClawBinaryName returns the installed executable name.
func picoClawBinaryName() string {
	if runtime.GOOS == "windows" {
		return "picoclaw.exe"
	}
	return "picoclaw"
}

func picoClawReleaseOS() string {
	switch runtime.GOOS {
	case "darwin":
		return "Darwin"
	case "linux":
		return "Linux"
	case "freebsd":
		return "Freebsd"
	case "windows":
		return "Windows"
	default:
		return strings.Title(runtime.GOOS)
	}
}

func picoClawReleaseArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "arm64"
	case "arm":
		return "armv6"
	default:
		return runtime.GOARCH
	}
}

// picoClawReleaseBundleName returns the release archive name in GitHub assets.
func picoClawReleaseBundleName() string {
	ext := ".tar.gz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("picoclaw_%s_%s%s", picoClawReleaseOS(), picoClawReleaseArch(), ext)
}

// getPicoClawInstallCommand returns the install command for PicoClaw.
// It downloads the release archive, verifies checksum when available,
// and installs to ~/.local/bin/picoclaw.
func getPicoClawInstallCommand() string {
	if os.Getenv("CARRIER_DEV_MODE") == "1" {
		return getPicoClawDevInstallCommand()
	}

	binary := picoClawBinaryName()
	bundle := picoClawReleaseBundleName()
	destName := "picoclaw"
	if runtime.GOOS == "windows" {
		destName = "picoclaw.exe"
	}

	switch runtime.GOOS {
	case "windows":
		return fmt.Sprintf(`powershell -NoProfile -Command "
$ErrorActionPreference = 'Stop'
$bundle = '%s'; $bin = '%s'; $dest = \"$env:USERPROFILE\.local\bin\%s\"
New-Item -ItemType Directory -Force -Path (Split-Path $dest) | Out-Null
$release = Invoke-RestMethod -Uri '%s'
$asset = ($release.assets | Where-Object { $_.name -eq $bundle } | Select-Object -First 1)
$url = if ($asset) { $asset.browser_download_url } else { '%s/' + $bundle }
$archive = Join-Path $env:TEMP $bundle
$tmp = Join-Path $env:TEMP ('picoclaw_extract_' + [guid]::NewGuid().ToString())
New-Item -ItemType Directory -Force -Path $tmp | Out-Null
Invoke-WebRequest -Uri $url -OutFile $archive
$checksumAsset = ($release.assets | Where-Object { $_.name -match 'checksums.*\.txt$' } | Select-Object -First 1)
if ($checksumAsset) {
  $sumsPath = Join-Path $env:TEMP 'picoclaw_checksums.txt'
  Invoke-WebRequest -Uri $checksumAsset.browser_download_url -OutFile $sumsPath
  $line = Get-Content $sumsPath | Select-String $bundle | Select-Object -First 1
  if ($line) {
    $expected = ($line.ToString().Trim() -split '\s+')[0].ToLower()
    $actual = (Get-FileHash $archive -Algorithm SHA256).Hash.ToLower()
    if ($actual -ne $expected) { Remove-Item $archive -ErrorAction SilentlyContinue; throw \"checksum mismatch\" }
  }
}
Expand-Archive -Path $archive -DestinationPath $tmp -Force
$extracted = Get-ChildItem -Path $tmp -Recurse -File | Where-Object { $_.Name -eq $bin } | Select-Object -First 1
if (-not $extracted) { throw \"extracted binary not found: $bin\" }
Copy-Item -Path $extracted.FullName -Destination $dest -Force
"`, bundle, binary, destName, picoClawReleaseAPIURL, picoClawReleaseBaseURL)
	default:
		return fmt.Sprintf(`sh -c '
set -e
BUNDLE="%s"
BINARY="%s"
DEST="$HOME/.local/bin/%s"
RELEASE_API_URL="%s"
mkdir -p "$HOME/.local/bin"
TMPDIR=$(mktemp -d)
cleanup() { rm -rf "$TMPDIR"; }
trap cleanup EXIT
download() {
  url="$1"
  out="$2"
  if ! curl -fsSL -o "$out" "$url"; then
    echo "download failed: $url" >&2
    exit 1
  fi
}

RELEASE_JSON="$TMPDIR/release.json"
download "$RELEASE_API_URL" "$RELEASE_JSON"

ASSET_URL=$(grep -Eo "\"browser_download_url\":[[:space:]]*\"[^\"]*\"" "$RELEASE_JSON" | sed -E "s/.*\"([^\"]+)\"/\\1/" | grep "/$BUNDLE$" | head -n1)
if [ -z "$ASSET_URL" ]; then ASSET_URL="%s/$BUNDLE"; fi

CHECKSUM_URL=$(grep -Eo "\"browser_download_url\":[[:space:]]*\"[^\"]*checksums[^\"]*\.txt\"" "$RELEASE_JSON" | sed -E "s/.*\"([^\"]+)\"/\\1/" | head -n1)

ARCHIVE="$TMPDIR/$BUNDLE"
download "$ASSET_URL" "$ARCHIVE"

if [ -n "$CHECKSUM_URL" ]; then
  SUMS="$TMPDIR/checksums.txt"
  download "$CHECKSUM_URL" "$SUMS"
  EXPECTED=$(awk -v f="$BUNDLE" "\$2==f {print \$1; exit}" "$SUMS")
  if [ -z "$EXPECTED" ]; then EXPECTED=$(grep -E "[ *]$BUNDLE$" "$SUMS" | head -n1 | awk "{print \$1}"); fi
  if [ -n "$EXPECTED" ]; then
    if command -v sha256sum >/dev/null 2>&1; then
      ACTUAL=$(sha256sum "$ARCHIVE" | awk "{print \$1}")
    elif command -v shasum >/dev/null 2>&1; then
      ACTUAL=$(shasum -a 256 "$ARCHIVE" | awk "{print \$1}")
    elif command -v openssl >/dev/null 2>&1; then
      ACTUAL=$(openssl dgst -sha256 "$ARCHIVE" | awk "{print \$NF}")
    else
      echo "no sha256 tool available (need sha256sum, shasum, or openssl)" >&2
      exit 1
    fi
    if [ "$EXPECTED" != "$ACTUAL" ]; then echo "checksum mismatch" >&2; exit 1; fi
  fi
fi

tar -xzf "$ARCHIVE" -C "$TMPDIR"
EXTRACTED=$(find "$TMPDIR" -type f -name "$BINARY" | head -n1)
if [ -z "$EXTRACTED" ]; then echo "extracted binary not found: $BINARY" >&2; exit 1; fi
install -m 0755 "$EXTRACTED" "$DEST"
'`, bundle, binary, destName, picoClawReleaseAPIURL, picoClawReleaseBaseURL)
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
