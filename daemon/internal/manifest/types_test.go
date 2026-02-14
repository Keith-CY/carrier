package manifest

import "testing"

func TestValidateSuccess(t *testing.T) {
	m := Manifest{
		ID:      "openclaw",
		Name:    "OpenClaw",
		Version: "1.0.0",
		Runtime: RuntimeSpec{
			Type:    RuntimeTypeLocalBinary,
			Install: CommandSpec{Command: "./install.sh"},
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

func TestValidateRejectsInvalidRuntimeType(t *testing.T) {
	m := Manifest{
		ID:      "openclaw",
		Name:    "OpenClaw",
		Version: "1.0.0",
		Runtime: RuntimeSpec{
			Type:    RuntimeType("docker"),
			Install: CommandSpec{Command: "x"},
			Start:   CommandSpec{Command: "x"},
			Stop:    CommandSpec{Command: "x"},
		},
		Memory: MemorySpec{
			Supports:  []MemoryType{MemoryTypePerAgent},
			MountPath: "./memory",
		},
	}

	if err := m.Validate(); err == nil {
		t.Fatal("expected error for invalid runtime type")
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
