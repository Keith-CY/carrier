package manifest

import (
	"errors"
	"fmt"
	"runtime"
	"sort"
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

const (
	UpgradeStrategyInPlaceOrReinstall = "in_place_or_reinstall"
)

const (
	CommandOSDarwin  = "darwin"
	CommandOSLinux   = "linux"
	CommandOSWindows = "windows"
	CommandOSDefault = "default"
)

var supportedCommandOS = map[string]struct{}{
	CommandOSDarwin:  {},
	CommandOSLinux:   {},
	CommandOSWindows: {},
	CommandOSDefault: {},
}

// Manifest is the full agent manifest schema covering runtime, env, network,
// health, upgrade, memory, and diagnostics as required by the PRD.
type Manifest struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	Version      string      `json:"version"`
	Description  string      `json:"description,omitempty"`
	Capabilities []string    `json:"capabilities,omitempty"`
	Runtime      RuntimeSpec `json:"runtime"`
	Network      NetworkSpec `json:"network"`
	Env          EnvSpec     `json:"env"`
	Memory       MemorySpec  `json:"memory"`
	Upgrade      UpgradeSpec `json:"upgrade"`
	Health       HealthSpec  `json:"health"`
	Diagnostics  Diagnostics `json:"diagnostics"`
}

type RuntimeSpec struct {
	Type    RuntimeType `json:"type"`
	Install CommandSpec `json:"install"`
	Upgrade CommandSpec `json:"upgrade"`
	Start   CommandSpec `json:"start"`
	Stop    CommandSpec `json:"stop"`
}

type CommandSpec struct {
	Command     string            `json:"command,omitempty"`
	CommandByOS map[string]string `json:"command_by_os,omitempty"`
}

// IsEmpty reports whether neither command nor command_by_os is defined.
func (c CommandSpec) IsEmpty() bool {
	if strings.TrimSpace(c.Command) != "" {
		return false
	}
	for _, value := range c.CommandByOS {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}

// ResolveForCurrentOS resolves the runtime command for the active GOOS.
func (c CommandSpec) ResolveForCurrentOS() (string, error) {
	return c.ResolveForGOOS(runtime.GOOS)
}

// ResolveForGOOS resolves the runtime command for a specific GOOS value.
// Resolution order:
// 1. command_by_os[goos]
// 2. command_by_os["default"]
// 3. command
func (c CommandSpec) ResolveForGOOS(goos string) (string, error) {
	goos = strings.ToLower(strings.TrimSpace(goos))
	if goos != "" {
		if cmd := strings.TrimSpace(c.CommandByOS[goos]); cmd != "" {
			return cmd, nil
		}
	}
	if cmd := strings.TrimSpace(c.CommandByOS["default"]); cmd != "" {
		return cmd, nil
	}
	if cmd := strings.TrimSpace(c.Command); cmd != "" {
		return cmd, nil
	}
	if goos == "" {
		return "", errors.New("command is required")
	}
	return "", fmt.Errorf("command is not defined for os %q", goos)
}

type NetworkSpec struct {
	Ports       []PortSpec      `json:"ports"`
	Healthcheck HealthcheckSpec `json:"healthcheck"`
}

type PortSpec struct {
	Name     string `json:"name"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol,omitempty"`
}

type HealthcheckSpec struct {
	Type    string `json:"type"`
	URL     string `json:"url,omitempty"`
	Command string `json:"command,omitempty"`
}

// HealthSpec defines the top-level health configuration for the agent,
// covering probe intervals, timeouts, and restart-loop detection.
type HealthSpec struct {
	IntervalSeconds   int `json:"interval_seconds,omitempty"`
	TimeoutSeconds    int `json:"timeout_seconds,omitempty"`
	Retries           int `json:"retries,omitempty"`
	RestartLoopWindow int `json:"restart_loop_window,omitempty"`
	RestartLoopMax    int `json:"restart_loop_max,omitempty"`
}

type EnvSpec struct {
	Required []EnvVar `json:"required"`
	Optional []EnvVar `json:"optional"`
}

type EnvVar struct {
	Name        string `json:"name"`
	Secret      bool   `json:"secret,omitempty"`
	Default     string `json:"default,omitempty"`
	Description string `json:"description,omitempty"`
}

type MemorySpec struct {
	Supports    []MemoryType      `json:"supports"`
	MountPath   string            `json:"mount_path"`
	Permissions MemoryPermissions `json:"permissions,omitempty"`
}

type MemoryPermissions struct {
	ReadScopes            []string `json:"read_scopes,omitempty"`
	WriteScopes           []string `json:"write_scopes,omitempty"`
	RequestedSharedScopes []string `json:"requested_shared_scopes,omitempty"`
	Retention             string   `json:"retention,omitempty"`
	Capture               []string `json:"capture,omitempty"`
	Redaction             []string `json:"redaction,omitempty"`
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
	if err := validateUpgrade(m.Upgrade); err != nil {
		return err
	}
	if err := validateNetwork(m.Network); err != nil {
		return err
	}
	if err := validateHealth(m.Health); err != nil {
		return err
	}
	if err := validateCapabilities(m.Capabilities); err != nil {
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

	if err := validateCommandSpec("runtime.install", r.Install, true); err != nil {
		return err
	}
	// runtime.upgrade.command is optional - not all agents need upgrade support
	if err := validateCommandSpec("runtime.upgrade", r.Upgrade, false); err != nil {
		return err
	}
	if err := validateCommandSpec("runtime.start", r.Start, true); err != nil {
		return err
	}
	if err := validateCommandSpec("runtime.stop", r.Stop, true); err != nil {
		return err
	}

	return nil
}

func validateCommandSpec(field string, command CommandSpec, required bool) error {
	trimmedDefault := strings.TrimSpace(command.Command)
	if trimmedDefault != "" {
		if err := validateCommandByOS(field, command.CommandByOS); err != nil {
			return err
		}
		return nil
	}

	if len(command.CommandByOS) > 0 {
		if err := validateCommandByOS(field, command.CommandByOS); err != nil {
			return err
		}
		// command_by_os is present and valid, so this command spec is satisfied.
		return nil
	}

	if required {
		return fmt.Errorf("%s.command is required", field)
	}
	return nil
}

func validateCommandByOS(field string, byOS map[string]string) error {
	for osName, raw := range byOS {
		normalized := strings.ToLower(strings.TrimSpace(osName))
		if normalized == "" {
			return fmt.Errorf("%s.command_by_os key must not be empty", field)
		}
		if normalized != osName {
			return fmt.Errorf("%s.command_by_os key %q must be lowercase and trimmed", field, osName)
		}
		if _, ok := supportedCommandOS[normalized]; !ok {
			keys := make([]string, 0, len(supportedCommandOS))
			for key := range supportedCommandOS {
				keys = append(keys, fmt.Sprintf("%q", key))
			}
			sort.Strings(keys)
			return fmt.Errorf("%s.command_by_os has unsupported key %q; supported: %s", field, osName, strings.Join(keys, ", "))
		}
		if strings.TrimSpace(raw) == "" {
			return fmt.Errorf("%s.command_by_os.%s is required", field, osName)
		}
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
	if err := validateMemoryPermissions(m.Permissions); err != nil {
		return err
	}

	return nil
}

func validateMemoryPermissions(p MemoryPermissions) error {
	validateScopes := func(field string, values []string) error {
		seen := map[string]struct{}{}
		for _, v := range values {
			scope := strings.TrimSpace(v)
			if scope == "" {
				return fmt.Errorf("%s contains empty scope", field)
			}
			if _, ok := seen[scope]; ok {
				return fmt.Errorf("%s contains duplicate scope: %q", field, scope)
			}
			seen[scope] = struct{}{}
		}
		return nil
	}
	if err := validateScopes("memory.permissions.read_scopes", p.ReadScopes); err != nil {
		return err
	}
	if err := validateScopes("memory.permissions.write_scopes", p.WriteScopes); err != nil {
		return err
	}
	if err := validateScopes("memory.permissions.requested_shared_scopes", p.RequestedSharedScopes); err != nil {
		return err
	}
	return nil
}

func validateUpgrade(u UpgradeSpec) error {
	if u.Channel == "" && u.Strategy == "" {
		return nil
	}

	if strings.TrimSpace(u.Channel) == "" {
		return errors.New("upgrade.channel is required when upgrade strategy is provided")
	}
	if strings.TrimSpace(u.Strategy) == "" {
		return errors.New("upgrade.strategy is required when upgrade channel is provided")
	}
	if u.Strategy != UpgradeStrategyInPlaceOrReinstall {
		return fmt.Errorf("upgrade.strategy %q is unsupported; supported: %q", u.Strategy, UpgradeStrategyInPlaceOrReinstall)
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

func validateNetwork(n NetworkSpec) error {
	seen := map[int]struct{}{}
	for _, p := range n.Ports {
		if p.Port < 0 || p.Port > 65535 {
			return fmt.Errorf("network.ports: port %d out of range (0-65535)", p.Port)
		}
		if p.Name == "" {
			return errors.New("network.ports: port name is required")
		}
		if _, ok := seen[p.Port]; ok {
			return fmt.Errorf("network.ports: duplicate port %d", p.Port)
		}
		seen[p.Port] = struct{}{}
	}
	return nil
}

func validateHealth(h HealthSpec) error {
	if h.IntervalSeconds < 0 {
		return errors.New("health.interval_seconds must not be negative")
	}
	if h.TimeoutSeconds < 0 {
		return errors.New("health.timeout_seconds must not be negative")
	}
	if h.Retries < 0 {
		return errors.New("health.retries must not be negative")
	}
	if h.TimeoutSeconds > 0 && h.IntervalSeconds > 0 && h.TimeoutSeconds >= h.IntervalSeconds {
		return errors.New("health.timeout_seconds must be less than health.interval_seconds")
	}
	return nil
}

func validateCapabilities(caps []string) error {
	seen := map[string]struct{}{}
	for _, c := range caps {
		if strings.TrimSpace(c) == "" {
			return errors.New("capability must not be empty")
		}
		if _, ok := seen[c]; ok {
			return fmt.Errorf("duplicate capability: %q", c)
		}
		seen[c] = struct{}{}
	}
	return nil
}
