package manifest

import (
	"runtime"
	"strings"
	"testing"
)

func TestValidateSuccess(t *testing.T) {
	m := validManifestForTest()
	if err := m.Validate(); err != nil {
		t.Fatalf("expected valid manifest, got error: %v", err)
	}
}

func TestValidateRejectsMissingRequiredFields(t *testing.T) {
	testCases := []struct {
		name    string
		mutate  func(m *Manifest)
		wantErr string
	}{
		{
			name:    "missing id",
			mutate:  func(m *Manifest) { m.ID = " " },
			wantErr: "manifest.id is required",
		},
		{
			name:    "missing name",
			mutate:  func(m *Manifest) { m.Name = "" },
			wantErr: "manifest.name is required",
		},
		{
			name:    "missing version",
			mutate:  func(m *Manifest) { m.Version = "" },
			wantErr: "manifest.version is required",
		},
		{
			name:    "missing runtime type",
			mutate:  func(m *Manifest) { m.Runtime.Type = "" },
			wantErr: "runtime.type is required",
		},
		{
			name:    "missing runtime install command",
			mutate:  func(m *Manifest) { m.Runtime.Install.Command = " " },
			wantErr: "runtime.install.command is required",
		},
		{
			name:    "missing runtime start command",
			mutate:  func(m *Manifest) { m.Runtime.Start.Command = "" },
			wantErr: "runtime.start.command is required",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := validManifestForTest()
			tc.mutate(&m)
			err := m.Validate()
			if err == nil {
				t.Fatalf("expected error %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error to contain %q, got %q", tc.wantErr, err.Error())
			}
		})
	}
}

func TestValidateRejectsInvalidRuntimeType(t *testing.T) {
	m := validManifestForTest()
	m.Runtime.Type = RuntimeType("docker")
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error for invalid runtime type")
	}
	if !strings.Contains(err.Error(), "runtime.type") {
		t.Fatalf("expected runtime.type in error, got %q", err.Error())
	}
}

func TestValidateAcceptsValidUpgradeSpec(t *testing.T) {
	m := validManifestForTest()
	m.Upgrade = UpgradeSpec{Channel: "stable", Strategy: UpgradeStrategyInPlaceOrReinstall}
	if err := m.Validate(); err != nil {
		t.Fatalf("expected valid upgrade manifest, got error: %v", err)
	}
}

func TestValidateRejectsUnsupportedUpgradeStrategy(t *testing.T) {
	m := validManifestForTest()
	m.Upgrade = UpgradeSpec{Channel: "stable", Strategy: "unsupported-strategy"}
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error for unsupported upgrade strategy")
	}
	if !strings.Contains(err.Error(), "upgrade.strategy") {
		t.Fatalf("expected upgrade.strategy in error, got %q", err.Error())
	}
}

func TestValidateRejectsDuplicateMemoryType(t *testing.T) {
	m := validManifestForTest()
	m.Memory.Supports = []MemoryType{MemoryTypeShared, MemoryTypeShared}
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for duplicate memory types")
	}
}

func TestValidateNetworkPortRange(t *testing.T) {
	m := validManifestForTest()
	m.Network.Ports = []PortSpec{{Name: "bad", Port: -1}}
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error for port -1")
	}
	if !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("expected 'out of range' in error, got %q", err.Error())
	}
}

func TestValidateNetworkPortZeroIsValid(t *testing.T) {
	m := validManifestForTest()
	m.Network.Ports = []PortSpec{{Name: "dynamic", Port: 0}}
	if err := m.Validate(); err != nil {
		t.Fatalf("expected port 0 (dynamic) to be valid, got: %v", err)
	}
}

func TestValidateNetworkPortTooHigh(t *testing.T) {
	m := validManifestForTest()
	m.Network.Ports = []PortSpec{{Name: "bad", Port: 70000}}
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error for port 70000")
	}
}

func TestValidateNetworkDuplicatePort(t *testing.T) {
	m := validManifestForTest()
	m.Network.Ports = []PortSpec{{Name: "a", Port: 8080}, {Name: "b", Port: 8080}}
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error for duplicate port")
	}
	if !strings.Contains(err.Error(), "duplicate port") {
		t.Fatalf("expected 'duplicate port' in error, got %q", err.Error())
	}
}

func TestValidateNetworkMissingPortName(t *testing.T) {
	m := validManifestForTest()
	m.Network.Ports = []PortSpec{{Name: "", Port: 8080}}
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error for missing port name")
	}
}

func TestValidateHealthNegativeInterval(t *testing.T) {
	m := validManifestForTest()
	m.Health.IntervalSeconds = -1
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error for negative interval")
	}
}

func TestValidateHealthTimeoutExceedsInterval(t *testing.T) {
	m := validManifestForTest()
	m.Health.IntervalSeconds = 10
	m.Health.TimeoutSeconds = 15
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error when timeout >= interval")
	}
}

func TestValidateHealthValidConfig(t *testing.T) {
	m := validManifestForTest()
	m.Health = HealthSpec{IntervalSeconds: 30, TimeoutSeconds: 5, Retries: 3}
	if err := m.Validate(); err != nil {
		t.Fatalf("expected valid health config, got: %v", err)
	}
}

func TestValidateCapabilitiesDuplicate(t *testing.T) {
	m := validManifestForTest()
	m.Capabilities = []string{"chat", "chat"}
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error for duplicate capability")
	}
}

func TestValidateCapabilitiesEmpty(t *testing.T) {
	m := validManifestForTest()
	m.Capabilities = []string{"chat", ""}
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error for empty capability")
	}
}

func TestValidateCapabilitiesValid(t *testing.T) {
	m := validManifestForTest()
	m.Capabilities = []string{"chat", "code", "memory"}
	if err := m.Validate(); err != nil {
		t.Fatalf("expected valid capabilities, got: %v", err)
	}
}

func TestValidateDescriptionOptional(t *testing.T) {
	m := validManifestForTest()
	m.Description = "A test agent"
	if err := m.Validate(); err != nil {
		t.Fatalf("expected valid with description, got: %v", err)
	}
}

func TestValidateAcceptsCommandByOSWhenCommandEmpty(t *testing.T) {
	m := validManifestForTest()
	m.Runtime.Install = CommandSpec{
		CommandByOS: map[string]string{
			"darwin":  "curl -fsSL https://openclaw.ai/install.sh | bash",
			"linux":   "curl -fsSL https://openclaw.ai/install.sh | bash",
			"windows": "powershell -NoProfile -Command \"iwr -useb https://openclaw.ai/install.ps1 | iex\"",
		},
	}
	m.Runtime.Start = CommandSpec{
		CommandByOS: map[string]string{
			"default": "openclaw gateway",
		},
	}
	m.Runtime.Stop = CommandSpec{
		CommandByOS: map[string]string{
			"default": "openclaw gateway stop",
		},
	}

	if err := m.Validate(); err != nil {
		t.Fatalf("expected command_by_os manifest to validate, got: %v", err)
	}
}

func TestValidateRejectsUppercaseCommandByOSKey(t *testing.T) {
	m := validManifestForTest()
	m.Runtime.Install = CommandSpec{
		CommandByOS: map[string]string{
			"Linux": "curl -fsSL https://openclaw.ai/install.sh | bash",
		},
	}
	err := m.Validate()
	if err == nil {
		t.Fatal("expected validation error for uppercase command_by_os key")
	}
	if !strings.Contains(err.Error(), "must be lowercase and trimmed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRejectsUnsupportedCommandByOSKey(t *testing.T) {
	m := validManifestForTest()
	m.Runtime.Install = CommandSpec{
		CommandByOS: map[string]string{
			"plan9": "curl -fsSL https://openclaw.ai/install.sh | bash",
		},
	}
	err := m.Validate()
	if err == nil {
		t.Fatal("expected validation error for unsupported command_by_os key")
	}
	if !strings.Contains(err.Error(), `unsupported key "plan9"`) {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), `"darwin"`) || !strings.Contains(err.Error(), `"linux"`) || !strings.Contains(err.Error(), `"windows"`) || !strings.Contains(err.Error(), `"default"`) {
		t.Fatalf("error should include supported keys, got: %v", err)
	}
}

func TestCommandSpecResolveForGOOS(t *testing.T) {
	spec := CommandSpec{
		Command: "openclaw gateway",
		CommandByOS: map[string]string{
			"windows": "powershell -NoProfile -Command \"openclaw gateway\"",
			"default": "sh -lc 'openclaw gateway'",
		},
	}

	got, err := spec.ResolveForGOOS("windows")
	if err != nil {
		t.Fatalf("ResolveForGOOS(windows) error: %v", err)
	}
	if got != `powershell -NoProfile -Command "openclaw gateway"` {
		t.Fatalf("windows command = %q", got)
	}

	got, err = spec.ResolveForGOOS("linux")
	if err != nil {
		t.Fatalf("ResolveForGOOS(linux) error: %v", err)
	}
	if got != `sh -lc 'openclaw gateway'` {
		t.Fatalf("linux command = %q", got)
	}

	got, err = spec.ResolveForCurrentOS()
	if err != nil {
		t.Fatalf("ResolveForCurrentOS(%s) error: %v", runtime.GOOS, err)
	}
	if strings.TrimSpace(got) == "" {
		t.Fatal("ResolveForCurrentOS returned empty command")
	}
}

func validManifestForTest() Manifest {
	return Manifest{
		ID:      "openclaw",
		Name:    "OpenClaw",
		Version: "1.0.0",
		Runtime: RuntimeSpec{
			Type:    RuntimeTypeLocalBinary,
			Install: CommandSpec{Command: "./install.sh"},
			Upgrade: CommandSpec{Command: "./install.sh --upgrade"},
			Start:   CommandSpec{Command: "./openclaw start"},
			Stop:    CommandSpec{Command: "./openclaw stop"},
		},
		Network: NetworkSpec{
			Ports:       []PortSpec{{Name: "http", Port: 8080}},
			Healthcheck: HealthcheckSpec{Type: "http", URL: "http://localhost:8080/health"},
		},
		Memory: MemorySpec{
			Supports:  []MemoryType{MemoryTypePerAgent, MemoryTypeShared, MemoryTypePublic},
			MountPath: "./memory",
		},
		Env: EnvSpec{
			Required: []EnvVar{{Name: "OPENAI_API_KEY", Secret: true}},
			Optional: []EnvVar{{Name: "LOG_LEVEL", Default: "info"}},
		},
	}
}
