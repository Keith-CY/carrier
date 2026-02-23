package catalog

import (
	"carrier/daemon/internal/manifest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestOpenClawManifestUsesDaemonHealthContract(t *testing.T) {
	m := OpenClawManifest()

	// OpenClaw runs alongside carrier daemon; declaring daemon's own fixed port
	// in agent pre-flight causes false-positive conflict failures.
	if got := len(m.Network.Ports); got != 0 {
		t.Fatalf("expected no declared network ports, got %d", got)
	}
	if got := m.Network.Healthcheck.URL; got != defaultDaemonHealthURL {
		t.Fatalf("healthcheck url = %q, want %q", got, defaultDaemonHealthURL)
	}
}

func TestZeroClawManifestBasic(t *testing.T) {
	m := ZeroClawManifest()

	if m.ID != "zeroclaw" {
		t.Fatalf("expected id zeroclaw, got %s", m.ID)
	}
	if m.Network.Healthcheck.Type != "process" {
		t.Fatalf("expected healthcheck type process, got %s", m.Network.Healthcheck.Type)
	}
	if m.Runtime.Start.Command == "" {
		t.Fatal("expected start command to be set")
	}
	if m.Runtime.Stop.Command == "" {
		t.Fatal("expected stop command to be set")
	}
	if m.Runtime.Install.Command == "" {
		t.Fatal("expected install command to be set")
	}
}

func TestZeroClawManifestDevMode(t *testing.T) {
	t.Setenv("CARRIER_DEV_MODE", "1")
	m := ZeroClawManifest()
	if m.Runtime.Install.Command == "" {
		t.Fatal("expected dev install command to be set")
	}
	// Dev mode should not use cargo install.
	if m.Runtime.Install.Command == "cargo install --git https://github.com/theonlyhennygod/zeroclaw.git --force" {
		t.Fatal("dev mode should not use production install command")
	}
}

func TestPicoClawManifestValid(t *testing.T) {
	m := PicoClawManifest()

	if err := m.Validate(); err != nil {
		t.Fatalf("PicoClawManifest().Validate() = %v", err)
	}

	if m.ID != "picoclaw" {
		t.Fatalf("id = %q, want %q", m.ID, "picoclaw")
	}
	if m.Network.Healthcheck.Type != "process" {
		t.Fatalf("healthcheck type = %q, want %q", m.Network.Healthcheck.Type, "process")
	}
	if m.Runtime.Stop.Command != "signal:term" {
		t.Fatalf("stop command = %q, want %q", m.Runtime.Stop.Command, "signal:term")
	}
}

func TestPicoClawBinaryName(t *testing.T) {
	name := picoClawBinaryName()
	if name == "" {
		t.Fatal("picoClawBinaryName() returned empty string")
	}
	// Should contain picoclaw and current GOOS/GOARCH
	if !contains(name, "picoclaw") {
		t.Fatalf("binary name %q does not contain 'picoclaw'", name)
	}
}

func TestPicoClawReleaseBundleName(t *testing.T) {
	name := picoClawReleaseBundleName()
	if !strings.HasPrefix(name, "picoclaw_") {
		t.Fatalf("bundle name %q should start with picoclaw_", name)
	}
	if runtime.GOOS == "windows" {
		if !strings.HasSuffix(name, ".zip") {
			t.Fatalf("windows bundle should end with .zip, got %q", name)
		}
	} else {
		if !strings.HasSuffix(name, ".tar.gz") {
			t.Fatalf("unix bundle should end with .tar.gz, got %q", name)
		}
	}
}

func TestPicoClawInstallCommand_UnixUsesArchiveFlow(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix install command test")
	}
	t.Setenv("CARRIER_DEV_MODE", "")

	cmd := getPicoClawInstallCommand()
	for _, want := range []string{
		picoClawReleaseAPIURL,
		"checksums",
		"shasum -a 256",
		"tar -xzf",
		"find \"$TMPDIR\" -type f -name \"$BINARY\"",
		"install -m 0755",
		picoClawReleaseBundleName(),
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("install command missing %q\ngot: %s", want, cmd)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestCatalogJSONHealthcheckMatchesGeneratedManifest(t *testing.T) {
	path := filepath.Join("..", "..", "..", "catalog", "openclaw.manifest.json")
	fileManifest, err := manifest.LoadFile(path)
	if err != nil {
		t.Fatalf("load catalog manifest: %v", err)
	}

	generated := OpenClawManifest()
	if fileManifest.Network.Healthcheck.URL != generated.Network.Healthcheck.URL {
		t.Fatalf("catalog healthcheck url = %q, generated = %q", fileManifest.Network.Healthcheck.URL, generated.Network.Healthcheck.URL)
	}
	if len(fileManifest.Network.Ports) != len(generated.Network.Ports) {
		t.Fatalf("catalog port count = %d, generated = %d", len(fileManifest.Network.Ports), len(generated.Network.Ports))
	}
}

// ---------------------------------------------------------------------------
// Install-command shell-contract tests
// ---------------------------------------------------------------------------

func TestGetInstallCommand_Default(t *testing.T) {
	// Ensure dev mode is off so we get the production command.
	t.Setenv("CARRIER_DEV_MODE", "")

	cmd := getInstallCommand()

	switch runtime.GOOS {
	case "windows":
		if !strings.Contains(cmd, installPS1URL) && !strings.Contains(cmd, installCMDURL) {
			t.Fatalf("windows install command should use official script URL, got: %s", cmd)
		}
	default:
		for _, want := range []string{
			"curl -fsSL",
			installScriptURL,
			"| bash -s --",
			"--no-onboard",
			"--no-prompt",
		} {
			if !strings.Contains(cmd, want) {
				t.Errorf("unix default install command missing %q\ngot: %s", want, cmd)
			}
		}
		if strings.Contains(cmd, "--install-method") || strings.Contains(cmd, " npm ") || strings.Contains(cmd, "||") {
			t.Fatalf("unix command should not include installer fallback/method switching, got: %s", cmd)
		}
	}
}

func TestResolveWindowsOpenClawInstallCommand_PowerShellPreferred(t *testing.T) {
	cmd := resolveWindowsOpenClawInstallCommand(func(name string) (string, error) {
		if strings.EqualFold(name, "powershell") || strings.EqualFold(name, "powershell.exe") {
			return `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`, nil
		}
		return "", os.ErrNotExist
	})

	if !strings.Contains(cmd, "powershell -NoProfile -Command") {
		t.Fatalf("expected powershell install command, got: %s", cmd)
	}
	if !strings.Contains(cmd, installPS1URL) {
		t.Fatalf("expected powershell install URL, got: %s", cmd)
	}
	if !strings.Contains(cmd, "iwr -useb") || !strings.Contains(cmd, "| iex") {
		t.Fatalf("expected powershell command to use iwr|iex flow, got: %s", cmd)
	}
	if !strings.Contains(cmd, "$env:OPENCLAW_NO_ONBOARD='1'") {
		t.Fatalf("expected powershell command to disable onboarding, got: %s", cmd)
	}
	if !strings.Contains(cmd, "$env:OPENCLAW_INSTALL_METHOD='npm'") {
		t.Fatalf("expected powershell command to pin install method, got: %s", cmd)
	}
}

func TestResolveWindowsOpenClawInstallCommand_CmdFallback(t *testing.T) {
	cmd := resolveWindowsOpenClawInstallCommand(func(string) (string, error) {
		return "", os.ErrNotExist
	})

	if strings.Contains(cmd, "powershell -NoProfile -Command") {
		t.Fatalf("did not expect powershell command in cmd fallback path: %s", cmd)
	}
	for _, want := range []string{
		"curl -fsSL",
		installCMDURL,
		"install.cmd",
		"--no-onboard",
		"del install.cmd",
	} {
		if !strings.Contains(cmd, want) {
			t.Fatalf("cmd fallback command missing %q: %s", want, cmd)
		}
	}
}

func TestGetInstallCommand_DevMode(t *testing.T) {
	t.Setenv("CARRIER_DEV_MODE", "1")

	cmd := getInstallCommand()

	for _, want := range []string{
		"mkdir -p",
		".local/bin",
		"base64 -d",
		"chmod +x",
		"openclaw",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("dev install command missing %q\ngot: %s", want, cmd)
		}
	}

	// Must NOT contain the production URL.
	if strings.Contains(cmd, installScriptURL) {
		t.Error("dev install command should not reference the production install URL")
	}
}

func TestGetStartCommand_Unix(t *testing.T) {
	cmd := getStartCommand()

	t.Run("uses shell resolver wrapper", func(t *testing.T) {
		if !strings.HasPrefix(cmd, "sh -c") {
			t.Errorf("start command should use shell resolver wrapper\ngot: %s", cmd)
		}
	})

	for _, tc := range []struct {
		name string
		want string
	}{
		{"home bin path", `$HOME/.local/bin/openclaw`},
		{"npm-global path", `$HOME/.npm-global/bin/openclaw`},
		{"path lookup", "command -v openclaw"},
		{"subcommand", "gateway"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(cmd, tc.want) {
				t.Errorf("start command missing %q\ngot: %s", tc.want, cmd)
			}
		})
	}
}

func TestGetStopCommand_Unix(t *testing.T) {
	cmd := getStopCommand()

	t.Run("uses shell resolver wrapper", func(t *testing.T) {
		if !strings.HasPrefix(cmd, "sh -c") {
			t.Errorf("stop command should use shell resolver wrapper\ngot: %s", cmd)
		}
	})

	for _, tc := range []struct {
		name string
		want string
	}{
		{"home bin path", `$HOME/.local/bin/openclaw`},
		{"npm-global path", `$HOME/.npm-global/bin/openclaw`},
		{"path lookup", "command -v openclaw"},
		{"subcommand", "gateway stop"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(cmd, tc.want) {
				t.Errorf("stop command missing %q\ngot: %s", tc.want, cmd)
			}
		})
	}
}

func TestGetNetworkSpec_ProductionOmitsPorts(t *testing.T) {
	t.Setenv("CARRIER_DEV_MODE", "")

	spec := getNetworkSpec()

	if len(spec.Ports) != 0 {
		t.Fatalf("production network spec should not declare fixed ports, got %d", len(spec.Ports))
	}
}

func TestGetNetworkSpec_DevModeAlsoOmitsPorts(t *testing.T) {
	t.Setenv("CARRIER_DEV_MODE", "1")

	spec := getNetworkSpec()

	if len(spec.Ports) != 0 {
		t.Fatalf("dev mode should not declare ports, got %d", len(spec.Ports))
	}
}

func TestOpenClawManifest_InstallAndUpgradeMatch(t *testing.T) {
	t.Setenv("CARRIER_DEV_MODE", "")

	m := OpenClawManifest()

	if m.Runtime.Install.Command != m.Runtime.Upgrade.Command {
		t.Errorf("install and upgrade commands should be identical\ninstall: %s\nupgrade: %s",
			m.Runtime.Install.Command, m.Runtime.Upgrade.Command)
	}
}

// ---------------------------------------------------------------------------
// Drift-detection tests (issue #640)
// ---------------------------------------------------------------------------

// TestNoStaleInstallerScripts ensures catalog/scripts/ does not contain
// unreferenced installer scripts. See issue #640.
func TestNoStaleInstallerScripts(t *testing.T) {
	staleDir := filepath.Join("..", "..", "..", "catalog", "scripts")
	entries, err := os.ReadDir(staleDir)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		t.Fatalf("reading catalog/scripts: %v", err)
	}
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Name()), "install") {
			t.Errorf("stale installer script found: catalog/scripts/%s", e.Name())
		}
	}
}

// TestInstallCommandUsesOfficialURL verifies the install command uses the official URL.
func TestInstallCommandUsesOfficialURL(t *testing.T) {
	m := OpenClawManifest()
	if !strings.Contains(m.Runtime.Install.Command, "https://openclaw.ai/install") {
		t.Fatalf("install command should reference official installer URL, got: %s", m.Runtime.Install.Command)
	}
}
