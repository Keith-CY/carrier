package manifest

import (
	"strings"
	"testing"
)

func TestValidateSuccess(t *testing.T) {
	m := Manifest{
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
		Memory: MemorySpec{
			Supports:  []MemoryType{MemoryTypePerAgent, MemoryTypeShared, MemoryTypePublic},
			MountPath: "./memory",
		},
		Env: EnvSpec{
			Required: []EnvVar{{Name: "OPENAI_API_KEY", Secret: true}},
			Optional: []EnvVar{{Name: "LOG_LEVEL", Default: "info"}},
		},
	}

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
			name: "missing id",
			mutate: func(m *Manifest) {
				m.ID = " "
			},
			wantErr: "manifest.id is required",
		},
		{
			name: "missing name",
			mutate: func(m *Manifest) {
				m.Name = ""
			},
			wantErr: "manifest.name is required",
		},
		{
			name: "missing version",
			mutate: func(m *Manifest) {
				m.Version = ""
			},
			wantErr: "manifest.version is required",
		},
		{
			name: "missing runtime type",
			mutate: func(m *Manifest) {
				m.Runtime.Type = ""
			},
			wantErr: "runtime.type is required",
		},
		{
			name: "missing runtime install command",
			mutate: func(m *Manifest) {
				m.Runtime.Install.Command = " "
			},
			wantErr: "runtime.install.command is required",
		},
		{
			name: "missing runtime start command",
			mutate: func(m *Manifest) {
				m.Runtime.Start.Command = ""
			},
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

func TestValidateRejectsDuplicateMemoryType(t *testing.T) {
	m := Manifest{
		ID:      "openclaw",
		Name:    "OpenClaw",
		Version: "1.0.0",
		Runtime: RuntimeSpec{
			Type:    RuntimeTypeGoCLI,
			Install: CommandSpec{Command: "x"},
			Upgrade: CommandSpec{Command: "x"},
			Start:   CommandSpec{Command: "x"},
			Stop:    CommandSpec{Command: "x"},
		},
		Memory: MemorySpec{
			Supports:  []MemoryType{MemoryTypeShared, MemoryTypeShared},
			MountPath: "./memory",
		},
	}

	if err := m.Validate(); err == nil {
		t.Fatal("expected error for duplicate memory types")
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
