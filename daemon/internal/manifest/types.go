package manifest

import (
	"errors"
	"fmt"
	"strings"
)

type RuntimeType string

const (
	RuntimeTypeLocalBinary RuntimeType = "local_binary"
	RuntimeTypeNpmCLI      RuntimeType = "npm_cli"
	RuntimeTypeGoCLI       RuntimeType = "go_cli"
)

type MemoryType string

const (
	MemoryTypePerAgent MemoryType = "per_agent"
	MemoryTypeShared   MemoryType = "shared"
	MemoryTypePublic   MemoryType = "public"
)

type Manifest struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Version     string        `json:"version"`
	Runtime     RuntimeSpec   `json:"runtime"`
	Network     NetworkSpec   `json:"network"`
	Env         EnvSpec       `json:"env"`
	Memory      MemorySpec    `json:"memory"`
	Upgrade     UpgradeSpec   `json:"upgrade"`
	Diagnostics Diagnostics   `json:"diagnostics"`
}

type RuntimeSpec struct {
	Type    RuntimeType `json:"type"`
	Install CommandSpec `json:"install"`
	Upgrade CommandSpec `json:"upgrade"`
	Start   CommandSpec `json:"start"`
	Stop    CommandSpec `json:"stop"`
}

type CommandSpec struct {
	Command string `json:"command"`
}

type NetworkSpec struct {
	Ports       []PortSpec       `json:"ports"`
	Healthcheck HealthcheckSpec  `json:"healthcheck"`
}

type PortSpec struct {
	Name string `json:"name"`
	Port int    `json:"port"`
}

type HealthcheckSpec struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type EnvSpec struct {
	Required []EnvVar `json:"required"`
	Optional []EnvVar `json:"optional"`
}

type EnvVar struct {
	Name    string `json:"name"`
	Secret  bool   `json:"secret,omitempty"`
	Default string `json:"default,omitempty"`
}

type MemorySpec struct {
	Supports  []MemoryType `json:"supports"`
	MountPath string       `json:"mount_path"`
}

type UpgradeSpec struct {
	Channel  string `json:"channel"`
	Strategy string `json:"strategy"`
}

type Diagnostics struct {
	Include []string `json:"include"`
}

func (m Manifest) Validate() error {
	if err := validateRequired("manifest.id", m.ID); err != nil {
		return err
	}
	if err := validateRequired("manifest.name", m.Name); err != nil {
		return err
	}
	if err := validateRequired("manifest.version", m.Version); err != nil {
		return err
	}
	if err := validateRuntime(m.Runtime); err != nil {
		return err
	}
	if err := validateMemory(m.Memory); err != nil {
		return err
	}
	if err := validateEnv(m.Env); err != nil {
		return err
	}

	return nil
}

func validateRuntime(r RuntimeSpec) error {
	if err := validateRequired("runtime.type", string(r.Type)); err != nil {
		return err
	}

	switch r.Type {
	case RuntimeTypeLocalBinary, RuntimeTypeNpmCLI, RuntimeTypeGoCLI:
		// valid
	default:
		return fmt.Errorf(
			"runtime.type %q is invalid; expected one of %q, %q, %q",
			r.Type,
			RuntimeTypeLocalBinary,
			RuntimeTypeNpmCLI,
			RuntimeTypeGoCLI,
		)
	}

	if err := validateRequired("runtime.install.command", r.Install.Command); err != nil {
		return err
	}
	// runtime.upgrade.command is optional - not all agents need upgrade support
	if err := validateRequired("runtime.start.command", r.Start.Command); err != nil {
		return err
	}
	if err := validateRequired("runtime.stop.command", r.Stop.Command); err != nil {
		return err
	}

	return nil
}

func validateRequired(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", field)
	}

	return nil
}

func validateMemory(m MemorySpec) error {
	if m.MountPath == "" {
		return errors.New("memory.mount_path is required")
	}
	if len(m.Supports) == 0 {
		return errors.New("memory.supports must not be empty")
	}

	seen := map[MemoryType]struct{}{}
	for _, t := range m.Supports {
		switch t {
		case MemoryTypePerAgent, MemoryTypeShared, MemoryTypePublic:
			// valid
		default:
			return fmt.Errorf("memory.supports contains invalid type: %q", t)
		}
		if _, ok := seen[t]; ok {
			return fmt.Errorf("memory.supports contains duplicate type: %q", t)
		}
		seen[t] = struct{}{}
	}

	return nil
}

func validateEnv(e EnvSpec) error {
	seen := map[string]struct{}{}
	for _, key := range append(e.Required, e.Optional...) {
		if key.Name == "" {
			return errors.New("env variable name must not be empty")
		}
		if _, ok := seen[key.Name]; ok {
			return fmt.Errorf("duplicate env variable declaration: %q", key.Name)
		}
		seen[key.Name] = struct{}{}
	}

	return nil
}
