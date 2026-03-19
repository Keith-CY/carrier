package catalog

import (
	"carrier/daemon/internal/manifest"
	"fmt"
	"os"
	"runtime"
	"strings"
)

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
			Type:    manifest.RuntimeTypeLocalBinary,
			Install: manifest.CommandSpec{Command: installCmd, CommandByOS: installByOS},
			Upgrade: manifest.CommandSpec{Command: installCmd, CommandByOS: installByOS},
			Start:   manifest.CommandSpec{Command: startCmd, CommandByOS: startByOS},
			Stop:    manifest.CommandSpec{Command: getPicoClawStopCommand()},
		},
		Network: manifest.NetworkSpec{Healthcheck: manifest.HealthcheckSpec{Type: "process"}},
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
