package catalog

import (
	"carrier/daemon/internal/manifest"
	"fmt"
	"strings"
)

type NpmAgentSpec struct {
	ID           string
	Name         string
	NpmPackage   string
	BinaryName   string
	Description  string
	Capabilities []string
	EnvVars      []manifest.EnvVar
}

var npmAgentSpecs = []NpmAgentSpec{
	{
		ID:           "codex",
		Name:         "Codex CLI",
		NpmPackage:   "@openai/codex",
		BinaryName:   "codex",
		Description:  "OpenAI Codex CLI agent for code generation and editing",
		Capabilities: []string{"code"},
		EnvVars: []manifest.EnvVar{
			{Name: "OPENAI_API_KEY", Secret: true, Description: "OpenAI API key for Codex"},
		},
	},
	{
		ID:           "opencode",
		Name:         "OpenCode",
		NpmPackage:   "opencode-ai",
		BinaryName:   "opencode",
		Description:  "OpenCode AI coding agent",
		Capabilities: []string{"code"},
		EnvVars: []manifest.EnvVar{
			{Name: "OPENAI_API_KEY", Secret: true, Description: "API key for OpenCode's LLM provider"},
		},
	},
}

func BuildNpmAgentManifest(spec NpmAgentSpec) manifest.Manifest {
	return manifest.Manifest{
		ID:           spec.ID,
		Name:         spec.Name,
		Version:      "latest",
		Description:  spec.Description,
		Capabilities: spec.Capabilities,
		Runtime: manifest.RuntimeSpec{
			Type:    manifest.RuntimeTypeNpmCLI,
			Install: buildNpmInstallCommand(spec),
			Upgrade: buildNpmInstallCommand(spec),
			Start:   buildNpmStartCommand(spec),
			Stop: manifest.CommandSpec{
				Command: "signal:term",
			},
		},
		Network: manifest.NetworkSpec{
			Healthcheck: manifest.HealthcheckSpec{
				Type:    "command",
				Command: buildNpmHealthCommand(spec),
			},
		},
		Env: manifest.EnvSpec{
			Optional: spec.EnvVars,
		},
		Memory: manifest.MemorySpec{
			Supports:  []manifest.MemoryType{manifest.MemoryTypePerAgent},
			MountPath: "./memory",
		},
		Upgrade: manifest.UpgradeSpec{
			Channel:  "stable",
			Strategy: manifest.UpgradeStrategyInPlaceOrReinstall,
		},
		Health: manifest.HealthSpec{
			IntervalSeconds:   30,
			TimeoutSeconds:    5,
			Retries:           3,
			RestartLoopWindow: 300,
			RestartLoopMax:    5,
		},
		Diagnostics: manifest.Diagnostics{
			Include: []string{"runtime_logs", "process_state"},
		},
	}
}

func buildNpmInstallCommand(spec NpmAgentSpec) manifest.CommandSpec {
	installScript := fmt.Sprintf(
		`sh -c 'if command -v bun >/dev/null 2>&1; then bun add -g %s; elif command -v npm >/dev/null 2>&1; then npm install -g %s; else echo "neither bun nor npm found" >&2; exit 127; fi; command -v %s >/dev/null 2>&1 || { echo "%s not found after install" >&2; exit 1; }'`,
		spec.NpmPackage, spec.NpmPackage, spec.BinaryName, spec.BinaryName,
	)
	return manifest.CommandSpec{
		Command: installScript,
		CommandByOS: map[string]string{
			manifest.CommandOSLinux:   installScript,
			manifest.CommandOSDarwin:  installScript,
			manifest.CommandOSWindows: buildWindowsWSLNpmInstall(spec),
		},
	}
}

func buildNpmStartCommand(spec NpmAgentSpec) manifest.CommandSpec {
	// Keep a managed process alive while preserving a quick binary validation.
	startScript := fmt.Sprintf(
		`sh -c 'command -v %s >/dev/null 2>&1 || { echo "%s not found in PATH" >&2; exit 127; }; trap "exit 0" TERM INT; while true; do sleep 3600; done'`,
		spec.BinaryName, spec.BinaryName,
	)
	return manifest.CommandSpec{
		Command: startScript,
		CommandByOS: map[string]string{
			manifest.CommandOSLinux:   startScript,
			manifest.CommandOSDarwin:  startScript,
			manifest.CommandOSWindows: startScript,
		},
	}
}

func buildNpmHealthCommand(spec NpmAgentSpec) string {
	return fmt.Sprintf("%s --version", spec.BinaryName)
}

func buildWindowsWSLNpmInstall(spec NpmAgentSpec) string {
	installInWSL := fmt.Sprintf(
		`set -e; if command -v bun >/dev/null 2>&1; then bun add -g %s; elif command -v npm >/dev/null 2>&1; then npm install -g %s; else echo "neither bun nor npm found" >&2; exit 127; fi; command -v %s >/dev/null 2>&1 || { echo "%s not found after install" >&2; exit 1; }`,
		spec.NpmPackage, spec.NpmPackage, spec.BinaryName, spec.BinaryName,
	)
	return wrapWindowsWSLBashCommand(installInWSL)
}

func NpmAgentManifests() []manifest.Manifest {
	manifests := make([]manifest.Manifest, 0, len(npmAgentSpecs))
	for _, spec := range npmAgentSpecs {
		manifests = append(manifests, BuildNpmAgentManifest(spec))
	}
	return manifests
}

func CodexManifest() manifest.Manifest {
	return npmManifestByID("codex")
}

func OpenCodeManifest() manifest.Manifest {
	return npmManifestByID("opencode")
}

func npmManifestByID(id string) manifest.Manifest {
	needle := strings.ToLower(strings.TrimSpace(id))
	for _, spec := range npmAgentSpecs {
		if strings.EqualFold(strings.TrimSpace(spec.ID), needle) {
			return BuildNpmAgentManifest(spec)
		}
	}
	panic("npm agent spec not found: " + id)
}
