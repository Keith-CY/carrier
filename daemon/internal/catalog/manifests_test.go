package catalog

import (
	"carrier/daemon/internal/manifest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenClawManifestUsesDaemonHealthContract(t *testing.T) {
	m := OpenClawManifest()

	if len(m.Network.Ports) == 0 {
		t.Fatal("expected at least one network port in manifest")
	}
	if got := m.Network.Ports[0].Port; got != defaultDaemonPort {
		t.Fatalf("network port = %d, want %d", got, defaultDaemonPort)
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
	// Dev mode should not use cargo install
	if m.Runtime.Install.Command == "cargo install --git https://github.com/theonlyhennygod/zeroclaw.git --force" {
		t.Fatal("dev mode should not use production install command")
	}
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
	if len(fileManifest.Network.Ports) == 0 || len(generated.Network.Ports) == 0 {
		t.Fatal("expected network ports in both manifests")
	}
	if fileManifest.Network.Ports[0].Port != generated.Network.Ports[0].Port {
		t.Fatalf("catalog port = %d, generated = %d", fileManifest.Network.Ports[0].Port, generated.Network.Ports[0].Port)
	}
}

// ---------------------------------------------------------------------------
// Install-command shell-contract tests
// ---------------------------------------------------------------------------

func TestGetInstallCommand_Unix(t *testing.T) {
	// Ensure dev mode is off so we get the production command.
	t.Setenv("CARRIER_DEV_MODE", "")

	cmd := getInstallCommand()

	for _, want := range []string{
		"curl",
		"-fsSL",
		`--proto "=https"`,
		"--tlsv1.2",
		installScriptURL,
		"| bash",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("install command missing %q\ngot: %s", want, cmd)
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

	home, _ := os.UserHomeDir()
	wantPrefix := filepath.Join(home, ".local", "bin", "openclaw")

	if !strings.HasPrefix(cmd, wantPrefix) {
		t.Errorf("start command should use absolute path\ngot: %s\nwant prefix: %s", cmd, wantPrefix)
	}
	if !strings.HasSuffix(cmd, "gateway start") {
		t.Errorf("start command should end with 'gateway start'\ngot: %s", cmd)
	}
}

func TestGetStopCommand_Unix(t *testing.T) {
	cmd := getStopCommand()

	home, _ := os.UserHomeDir()
	wantPrefix := filepath.Join(home, ".local", "bin", "openclaw")

	if !strings.HasPrefix(cmd, wantPrefix) {
		t.Errorf("stop command should use absolute path\ngot: %s\nwant prefix: %s", cmd, wantPrefix)
	}
	if !strings.HasSuffix(cmd, "gateway stop") {
		t.Errorf("stop command should end with 'gateway stop'\ngot: %s", cmd)
	}
}

func TestGetNetworkSpec_ProductionIncludesPorts(t *testing.T) {
	t.Setenv("CARRIER_DEV_MODE", "")

	spec := getNetworkSpec()

	if len(spec.Ports) == 0 {
		t.Fatal("production network spec must declare ports")
	}
	if spec.Ports[0].Port != defaultDaemonPort {
		t.Fatalf("port = %d, want %d", spec.Ports[0].Port, defaultDaemonPort)
	}
}

func TestGetNetworkSpec_DevModeSkipsPorts(t *testing.T) {
	t.Setenv("CARRIER_DEV_MODE", "1")

	spec := getNetworkSpec()

	if len(spec.Ports) != 0 {
		t.Fatalf("dev mode should skip ports, got %d", len(spec.Ports))
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
