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

// getZeroClawInstallCommand returns the install command for ZeroClaw.
// ZeroClaw is installed via cargo (Rust toolchain required).
// In dev mode, a placeholder script is created instead.
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
	return getZeroClawGatewayCommandForGOOS(goos, "gateway start")
}

// getZeroClawStopCommand returns the stop command for ZeroClaw.
func getZeroClawStopCommand() string {
	return getZeroClawStopCommandForGOOS(runtime.GOOS)
}

func getZeroClawStopCommandForGOOS(goos string) string {
	return getZeroClawGatewayCommandForGOOS(goos, "gateway stop")
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
	return picoClawBinaryNameForGOOS(runtime.GOOS)
}

func picoClawBinaryNameForGOOS(goos string) string {
	if normalizeCatalogGOOS(goos) == "windows" {
		return "picoclaw.exe"
	}
	return "picoclaw"
}

func picoClawReleaseOSForGOOS(goos string) string {
	switch normalizeCatalogGOOS(goos) {
	case "darwin":
		return "Darwin"
	case "linux":
		return "Linux"
	case "freebsd":
		return "Freebsd"
	case "windows":
		return "Windows"
	default:
		normalized := normalizeCatalogGOOS(goos)
		if normalized == "" {
			return ""
		}
		return strings.ToUpper(normalized[:1]) + strings.ToLower(normalized[1:])
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
	return picoClawReleaseBundleNameForGOOS(runtime.GOOS)
}

func picoClawReleaseBundleNameForGOOS(goos string) string {
	ext := ".tar.gz"
	if normalizeCatalogGOOS(goos) == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("picoclaw_%s_%s%s", picoClawReleaseOSForGOOS(goos), picoClawReleaseArch(), ext)
}

// getPicoClawInstallCommand returns the install command for PicoClaw.
// It downloads the release archive, verifies checksum when available,
// and installs to ~/.local/bin/picoclaw.
func getPicoClawInstallCommand() string {
	return getPicoClawInstallCommandForGOOS(runtime.GOOS)
}

func getPicoClawInstallCommandForGOOS(goos string) string {
	if os.Getenv("CARRIER_DEV_MODE") == "1" {
		return getPicoClawDevInstallCommand()
	}

	targetGOOS := normalizeCatalogGOOS(goos)
	binary := picoClawBinaryNameForGOOS(targetGOOS)
	bundle := picoClawReleaseBundleNameForGOOS(targetGOOS)
	destName := "picoclaw"
	if targetGOOS == "windows" {
		destName = "picoclaw.exe"
	}

	switch targetGOOS {
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
	return getPicoClawStartCommandForGOOS(runtime.GOOS)
}

func getPicoClawStartCommandForGOOS(goos string) string {
	switch normalizeCatalogGOOS(goos) {
	case "windows":
		return "picoclaw gateway"
	default:
		return `sh -c 'if [ -x "$HOME/.local/bin/picoclaw" ]; then exec "$HOME/.local/bin/picoclaw" gateway; elif command -v picoclaw >/dev/null 2>&1; then exec "$(command -v picoclaw)" gateway; else echo "picoclaw executable not found (checked $HOME/.local/bin/picoclaw and PATH)" >&2; exit 127; fi'`
	}
}

// getPicoClawStopCommand returns "signal:term" to indicate SIGTERM-based stop.
// PicoClaw does not have a stop subcommand; the process is terminated via signal.
func getPicoClawStopCommand() string {
	return "signal:term"
}

// PicoClawManifest returns the manifest for the PicoClaw agent.
func PicoClawManifest() manifest.Manifest {
	installCmd := getPicoClawInstallCommand()
	installByOS := map[string]string{
		manifest.CommandOSLinux:   getPicoClawInstallCommandForGOOS(manifest.CommandOSLinux),
		manifest.CommandOSDarwin:  getPicoClawInstallCommandForGOOS(manifest.CommandOSDarwin),
		manifest.CommandOSWindows: getPicoClawInstallCommandForGOOS(manifest.CommandOSWindows),
	}
	startCmd := getPicoClawStartCommand()
	startByOS := map[string]string{
		manifest.CommandOSLinux:   getPicoClawStartCommandForGOOS(manifest.CommandOSLinux),
		manifest.CommandOSDarwin:  getPicoClawStartCommandForGOOS(manifest.CommandOSDarwin),
		manifest.CommandOSWindows: getPicoClawStartCommandForGOOS(manifest.CommandOSWindows),
	}

	return manifest.Manifest{
		ID:           "picoclaw",
		Name:         "PicoClaw",
		Version:      "latest",
		Description:  "Go-based ultra-lightweight AI assistant",
		Capabilities: []string{"chat", "code"},
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
			Stop: manifest.CommandSpec{Command: getPicoClawStopCommand()},
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

func getCodexInstallCommand() string {
	return `sh -c 'if command -v codex >/dev/null 2>&1; then exit 0; fi; if command -v npm >/dev/null 2>&1; then npm install -g @openai/codex; elif command -v bun >/dev/null 2>&1; then bun add -g @openai/codex; else echo "npm or bun is required to install codex" >&2; exit 127; fi'`
}

func getCodexStartCommand() string {
	return `sh -c 'if command -v codex >/dev/null 2>&1; then exec "$(command -v codex)"; else echo "codex executable not found in PATH" >&2; exit 127; fi'`
}

func CodexManifest() manifest.Manifest {
	installCmd := getCodexInstallCommand()
	return manifest.Manifest{
		ID:           "codex",
		Name:         "Codex CLI",
		Version:      "latest",
		Description:  "OpenAI Codex CLI coding agent",
		Capabilities: []string{"code"},
		Runtime: manifest.RuntimeSpec{
			Type: manifest.RuntimeTypeLocalBinary,
			Install: manifest.CommandSpec{
				Command: installCmd,
			},
			Upgrade: manifest.CommandSpec{
				Command: installCmd,
			},
			Start: manifest.CommandSpec{
				Command: getCodexStartCommand(),
			},
			Stop: manifest.CommandSpec{
				Command: "signal:term",
			},
		},
		Network: manifest.NetworkSpec{
			Healthcheck: manifest.HealthcheckSpec{
				Type: "process",
			},
		},
		Env: manifest.EnvSpec{
			Required: []manifest.EnvVar{},
			Optional: []manifest.EnvVar{},
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

func getOpenCodeInstallCommand() string {
	return `sh -c 'if command -v opencode >/dev/null 2>&1; then exit 0; fi; if command -v npm >/dev/null 2>&1; then npm install -g opencode-ai; elif command -v bun >/dev/null 2>&1; then bun add -g opencode-ai; else echo "npm or bun is required to install opencode" >&2; exit 127; fi'`
}

func getOpenCodeStartCommand() string {
	return `sh -c 'if command -v opencode >/dev/null 2>&1; then exec "$(command -v opencode)"; else echo "opencode executable not found in PATH" >&2; exit 127; fi'`
}

func OpenCodeManifest() manifest.Manifest {
	installCmd := getOpenCodeInstallCommand()
	return manifest.Manifest{
		ID:           "opencode",
		Name:         "OpenCode",
		Version:      "latest",
		Description:  "OpenCode CLI coding agent",
		Capabilities: []string{"code"},
		Runtime: manifest.RuntimeSpec{
			Type: manifest.RuntimeTypeLocalBinary,
			Install: manifest.CommandSpec{
				Command: installCmd,
			},
			Upgrade: manifest.CommandSpec{
				Command: installCmd,
			},
			Start: manifest.CommandSpec{
				Command: getOpenCodeStartCommand(),
			},
			Stop: manifest.CommandSpec{
				Command: "signal:term",
			},
		},
		Network: manifest.NetworkSpec{
			Healthcheck: manifest.HealthcheckSpec{
				Type: "process",
			},
		},
		Env: manifest.EnvSpec{
			Required: []manifest.EnvVar{},
			Optional: []manifest.EnvVar{},
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
