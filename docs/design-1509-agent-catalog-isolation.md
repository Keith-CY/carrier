# Design Document: Issue #1509 — Agent Catalog, Auto-Install Bwrap, OpenAI-Compatible Auth

**Status:** Draft  
**Date:** 2026-03-01  
**Author:** Design subagent  
**Issue:** https://github.com/Keith-CY/carrier/issues/1509

---

## Table of Contents

1. [Overview](#overview)
2. [Part 1: Auto-Install Bubblewrap](#part-1-auto-install-bubblewrap)
3. [Part 2: Codex/OpenCode Catalog Entries](#part-2-codexopencode-catalog-entries)
4. [Part 3: OpenAI-Compatible Provider Auth](#part-3-openai-compatible-provider-auth)
5. [Part 4: Channel-Optional Agents](#part-4-channel-optional-agents)
6. [Part 5: Config Schema Changes](#part-5-config-schema-changes)
7. [Part 6: Test Strategy](#part-6-test-strategy)
8. [Appendix: File Change Map](#appendix-file-change-map)

---

## Overview

Three independent but related improvements, designed for maintainability and extensibility:

| Improvement | Scope | Key packages |
|---|---|---|
| Auto-install bwrap | Isolation pipeline | `daemon/internal/lifecycle` |
| Codex/OpenCode catalog | Agent manifests + onboarding | `daemon/internal/catalog`, `shared/catalog`, `gateway` |
| OpenAI-compatible auth | Provider onboarding | `shared/catalog`, `gateway` |

### Design Principles

1. **No post-hoc patching** — channel-optional logic belongs in `shared/catalog`, not in gateway `managed_onboard.go` after the fact
2. **Table-driven tests** — avoid near-duplicate test functions for codex vs opencode
3. **Error capture for both sudo and non-sudo paths** — auto-install must capture stderr from whichever execution path runs
4. **Extensibility** — adding a new agent or provider should require adding a single struct, not touching 5 files

---

## Part 1: Auto-Install Bubblewrap

### Current State

`daemon/internal/lifecycle/isolation_backend.go` already has `resolveIsolationBackend()` which returns `ErrIsolationUnavailable` when bwrap is missing. The `buildHostEnsureLinuxIsolationDepsCommand()` and `buildGuestEnsureBwrapCommand()` functions already contain shell scripts that attempt package-manager-based installation with `sudo -n`. These are used in the host-prepare and guest-prepare pipeline steps.

**The actual gap:** When `resolveIsolationBackend()` is called as part of `InstallWithOptions(Isolation: true)`, it currently fails immediately if bwrap is not in PATH. The ensure-deps scripts exist but are only run as part of the Lima/WSL guest preparation pipeline — they are **not** called for direct Linux hosts before resolving the backend.

### Proposed Flow

```
InstallWithOptions(isolation=true)
  ├── resolveIsolationBackend()  → try to find bwrap
  │     └── if found → proceed
  │     └── if NOT found → continue to auto-install
  ├── attemptAutoInstallBwrap()  ← NEW
  │     ├── detectPackageManager() → apt-get | dnf | yum | pacman | apk | zypper
  │     ├── detectSudoAvailability()
  │     │     ├── command -v sudo → exists?
  │     │     └── sudo -n true → passwordless available?
  │     ├── runPackageInstall(pkgMgr, "bubblewrap")
  │     │     ├── try: sudo -n <pkg-mgr> install bubblewrap
  │     │     │     └── capture stdout+stderr
  │     │     ├── if sudo fails, try: <pkg-mgr> install bubblewrap (for root/container)
  │     │     │     └── capture stdout+stderr
  │     │     └── if both fail → return combined error with both outputs
  │     └── verifyInstallation()
  │           └── exec.LookPath("bwrap") → found?
  ├── resolveIsolationBackend()  → retry (should now find bwrap)
  └── continue normal isolation pipeline
```

### New Go Types and Functions

**File: `daemon/internal/lifecycle/isolation_autoinstall.go`** (new)

```go
package lifecycle

// PackageManager represents a detected system package manager.
type PackageManager struct {
    Name       string   // e.g. "apt-get", "dnf", "pacman"
    InstallCmd []string // e.g. ["apt-get", "install", "-y"]
    UpdateCmd  []string // e.g. ["apt-get", "update"] (nil if none)
}

// BwrapAutoInstallResult captures the outcome of an auto-install attempt.
type BwrapAutoInstallResult struct {
    Installed      bool
    PackageManager string
    UsedSudo       bool
    SudoOutput     string // stderr from sudo attempt
    DirectOutput   string // stderr from non-sudo attempt
    VerifyPath     string // path from LookPath after install
}

// ErrAutoInstallFailed is returned when bwrap could not be auto-installed.
var ErrAutoInstallFailed = errors.New("bwrap auto-install failed")

// attemptAutoInstallBwrap tries to install bubblewrap via the system
// package manager. It attempts sudo first, then falls back to direct
// execution (for root/container environments).
func attemptAutoInstallBwrap() (*BwrapAutoInstallResult, error)

// detectPackageManager probes the system for a known package manager.
func detectPackageManager() (*PackageManager, error)

// detectSudoAvailability checks if sudo exists and can run passwordless.
func detectSudoAvailability() (hasSudo bool, passwordless bool)
```

### Key Design Decisions

**1. Two-attempt strategy (sudo → direct):**
```
sudo -n apt-get install -y bubblewrap 2>&1
  → success? done
  → failure? capture output, try without sudo:
apt-get install -y bubblewrap 2>&1
  → success? done  
  → failure? return error with BOTH outputs
```

This handles:
- Normal user with passwordless sudo (most common)
- Root user in containers (no sudo needed)
- User without sudo → clear error with both failure messages

**2. Package manager detection order:**
```
apt-get → dnf → yum → pacman → apk → zypper
```
This matches the existing order in `buildHostEnsureLinuxIsolationDepsCommand()`. Each is checked via `exec.LookPath`.

**3. Error reporting:** The `BwrapAutoInstallResult` captures output from BOTH the sudo and non-sudo attempts. If both fail, the error message includes:
```
bwrap auto-install failed:
  sudo attempt: <stderr from sudo>
  direct attempt: <stderr from direct>
Hint: install bubblewrap manually: sudo apt-get install -y bubblewrap
```

**4. Verification:** After install, call `exec.LookPath("bwrap")` to confirm. This catches cases where install "succeeds" but the binary isn't actually available.

**5. No interactive sudo:** Only `sudo -n` (non-interactive). If sudo requires a password, we fall through to the non-sudo attempt and ultimately to a clear error message.

**6. Testability:** All external calls go through injectable function variables (same pattern as existing `isolationBackendLookup`, `isolationEnvLookup`):

```go
var (
    autoInstallLookPath = exec.LookPath
    autoInstallExecCmd  = func(name string, args ...string) *exec.Cmd {
        return exec.Command(name, args...)
    }
)
```

### Integration Point

In `daemon/internal/lifecycle/install.go`, in the isolation installation path:

```go
// Current: resolveIsolationBackend() → fail if bwrap missing
// New:
backend, err := resolveIsolationBackend()
if err != nil && errors.Is(err, ErrIsolationUnavailable) && isolationRuntimeGOOS == "linux" {
    result, installErr := attemptAutoInstallBwrap()
    if installErr != nil {
        // Log both sudo and direct outputs for diagnostics
        return fmt.Errorf("isolation requires bubblewrap: %w (auto-install: %v)", err, installErr)
    }
    // Retry backend resolution
    backend, err = resolveIsolationBackend()
}
if err != nil {
    return err
}
```

### Refactoring: Deduplicate Shell Scripts

The existing `buildHostEnsureLinuxIsolationDepsCommand()` and `buildGuestEnsureBwrapCommand()` contain nearly identical shell scripts. Extract the common logic:

```go
// buildEnsureIsolationDepsScript generates the shell script for ensuring
// isolation dependencies are installed. The 'context' parameter distinguishes
// host vs guest error messages.
func buildEnsureIsolationDepsScript(context string) string
```

Then:
```go
func buildHostEnsureLinuxIsolationDepsCommand() string {
    return buildEnsureIsolationDepsScript("host")
}
func buildGuestEnsureBwrapCommand() string {
    return buildEnsureIsolationDepsScript("guest")
}
```

---

## Part 2: Codex/OpenCode Catalog Entries

### Current State

- `daemon/internal/catalog/catalog.go` has `DefaultEntries()` returning built-in agent entries (openclaw, zeroclaw, picoclaw, etc.)
- `daemon/internal/catalog/manifests.go` has `OpenClawManifest()`, `ZeroClawManifest()`, `PicoClawManifest()` returning full `manifest.Manifest` structs
- `manifest.RuntimeType` already defines `RuntimeTypeNpmCLI` — perfect for codex/opencode
- `codeagent/adapters/codex/` and `codeagent/adapters/opencode/` already have adapter code with install/health/version
- `gateway/remote_codeagent.go` has `buildRemoteCodeAgentInstallPlan()` with npm/bun install logic

**The gap:** Codex and OpenCode exist as remote code-agent backends but have no catalog entries or lifecycle manifests, so they can't be onboarded via `carrier add` or `/onboard`.

### Proposed Catalog Entries

**File: `daemon/internal/catalog/catalog.go`** — add to `DefaultEntries()`:

```go
{
    ID:           "codex",
    Name:         "Codex CLI",
    Version:      "latest",
    Status:       StatusActive,
    Capabilities: []string{"code"},
    Description:  "OpenAI Codex CLI agent for code generation and editing",
},
{
    ID:           "opencode",
    Name:         "OpenCode",
    Version:      "latest",
    Status:       StatusActive,
    Capabilities: []string{"code"},
    Description:  "OpenCode AI coding agent",
},
```

### Manifest Structure

**File: `daemon/internal/catalog/manifests_npm.go`** (new file, separating npm-based agents)

The key insight: codex and opencode share the same installation pattern (npm/bun global install) and the same lifecycle (foreground process, health via `--version`, no gateway port). Extract a **shared builder**:

```go
package catalog

import (
    "carrier/daemon/internal/manifest"
    "fmt"
    "runtime"
    "strings"
)

// NpmAgentSpec defines an npm-based agent's identity for manifest generation.
type NpmAgentSpec struct {
    ID          string
    Name        string
    NpmPackage  string // e.g. "@openai/codex"
    BinaryName  string // e.g. "codex"
    Description string
    Capabilities []string
    EnvVars     []manifest.EnvVar // agent-specific env vars
    StartArgs   string            // args after binary name, e.g. "" or "--interactive"
    HealthArgs  string            // args for health check, e.g. "--version"
}

// npmAgentSpecs is the registry of npm-based agents.
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

// BuildNpmAgentManifest generates a Manifest from an NpmAgentSpec.
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
            Upgrade: buildNpmInstallCommand(spec), // same command re-installs
            Start:   buildNpmStartCommand(spec),
            Stop:    manifest.CommandSpec{Command: "signal:term"},
        },
        Network: manifest.NetworkSpec{
            Healthcheck: manifest.HealthcheckSpec{
                Type:    "command",
                Command: buildNpmHealthCommand(spec), // e.g. "codex --version"
            },
        },
        Env: manifest.EnvSpec{
            Required: []manifest.EnvVar{},
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
    // Prefer bun if available, fall back to npm
    installScript := fmt.Sprintf(
        `sh -c 'if command -v bun >/dev/null 2>&1; then bun add -g %s; elif command -v npm >/dev/null 2>&1; then npm install -g %s; else echo "neither bun nor npm found" >&2; exit 127; fi; command -v %s >/dev/null 2>&1 || { echo "%s not found after install" >&2; exit 1; }'`,
        spec.NpmPackage, spec.NpmPackage, spec.BinaryName, spec.BinaryName,
    )
    return manifest.CommandSpec{
        Command: installScript,
        CommandByOS: map[string]string{
            manifest.CommandOSLinux:  installScript,
            manifest.CommandOSDarwin: installScript,
            // Windows: run inside WSL
            manifest.CommandOSWindows: buildWindowsWSLNpmInstall(spec),
        },
    }
}

func buildNpmStartCommand(spec NpmAgentSpec) manifest.CommandSpec {
    // Code agents are foreground processes, not gateway daemons.
    // They are invoked per-task, not kept running. Start is a no-op placeholder
    // that returns immediately — the actual invocation happens through the
    // codeagent adapter's Run() method.
    startScript := fmt.Sprintf(
        `sh -c 'command -v %s >/dev/null 2>&1 || { echo "%s not found in PATH" >&2; exit 127; }; echo "%s ready"'`,
        spec.BinaryName, spec.BinaryName, spec.Name,
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
```

### Manifest Registration

In `daemon/internal/catalog/manifests.go`, add a registration function:

```go
// NpmAgentManifests returns manifests for all registered npm-based agents.
func NpmAgentManifests() []manifest.Manifest {
    manifests := make([]manifest.Manifest, 0, len(npmAgentSpecs))
    for _, spec := range npmAgentSpecs {
        manifests = append(manifests, BuildNpmAgentManifest(spec))
    }
    return manifests
}

// CodexManifest returns the manifest for Codex CLI. Convenience accessor.
func CodexManifest() manifest.Manifest {
    for _, spec := range npmAgentSpecs {
        if spec.ID == "codex" {
            return BuildNpmAgentManifest(spec)
        }
    }
    panic("codex spec not found in npmAgentSpecs")
}

// OpenCodeManifest returns the manifest for OpenCode. Convenience accessor.
func OpenCodeManifest() manifest.Manifest {
    for _, spec := range npmAgentSpecs {
        if spec.ID == "opencode" {
            return BuildNpmAgentManifest(spec)
        }
    }
    panic("opencode spec not found in npmAgentSpecs")
}
```

### Why a New File?

Separating npm-based agent manifests into `manifests_npm.go`:
- `manifests.go` is already 500+ lines with OpenClaw/ZeroClaw/PicoClaw (binary-based agents)
- npm agents share install/upgrade/start patterns — a single builder avoids duplication
- Adding future npm agents (e.g. `aider`, `cursor-cli`) means adding one `NpmAgentSpec` entry

---

## Part 3: OpenAI-Compatible Provider Auth

### Current State

`shared/catalog/catalog.go` already defines an `openai-compatible` provider:
```go
{
    ID:       "openai-compatible",
    AuthMode: AuthModeNone,       // ← THIS IS THE PROBLEM
    EnvVar:   "OPENAI_COMPATIBLE_API_KEY",
    Category: "generic",
}
```

It's `AuthModeNone` even though OpenRouter requires an API key and Ollama needs a base URL. The single "openai-compatible" entry can't represent the diversity of OpenAI v1 compatible providers.

### Proposed Design: Expand Provider Catalog

Instead of a single generic entry, add specific well-known compatible providers while keeping the generic fallback:

```go
// In shared/catalog/catalog.go, replace the single openai-compatible entry with:

// Well-known OpenAI-compatible providers
{
    ID:           "openrouter",
    Name:         "OpenRouter",
    AuthMode:     AuthModeAPIKey,
    EnvVar:       "OPENROUTER_API_KEY",
    ExampleModel: "openrouter/anthropic/claude-sonnet-4-6",
    Category:     "compatible",  // NEW category
    Description:  "OpenRouter multi-model proxy (OpenAI-compatible)",
    Setup:        "API key from openrouter.ai",
    BaseURLEnv:   "OPENROUTER_BASE_URL",             // NEW field
    DefaultBase:  "https://openrouter.ai/api/v1",    // NEW field
},
{
    ID:           "ollama",
    Name:         "Ollama (local)",
    AuthMode:     AuthModeNone,
    EnvVar:       "",
    ExampleModel: "ollama/llama3",
    Category:     "compatible",
    Description:  "Local Ollama instance (OpenAI-compatible)",
    BaseURLEnv:   "OLLAMA_BASE_URL",
    DefaultBase:  "http://localhost:11434/v1",
},
{
    ID:           "openai-compatible",
    Name:         "OpenAI-Compatible (custom)",
    AuthMode:     AuthModeAPIKey,  // Changed from None to APIKey
    EnvVar:       "OPENAI_COMPATIBLE_API_KEY",
    ExampleModel: "openai-compatible/your-model-id",
    Category:     "compatible",
    Description:  "Custom OpenAI v1-compatible endpoint",
    BaseURLEnv:   "OPENAI_COMPATIBLE_BASE_URL",       // NEW field
    DefaultBase:  "",                                  // user must provide
},
```

### New Fields on ProviderSpec

```go
type ProviderSpec struct {
    ID           string   `json:"id"`
    Name         string   `json:"name"`
    AuthMode     AuthMode `json:"auth_mode"`
    EnvVar       string   `json:"env_var,omitempty"`
    ExampleModel string   `json:"example_model,omitempty"`
    Category     string   `json:"category"`
    Description  string   `json:"description,omitempty"`
    Setup        string   `json:"-"`
    
    // New fields for OpenAI-compatible providers
    BaseURLEnv   string   `json:"base_url_env,omitempty"`   // env var for base URL override
    DefaultBase  string   `json:"default_base,omitempty"`   // default base URL (empty = user must provide)
    
    // Channel requirement policy
    ChannelRequired bool  `json:"channel_required,omitempty"` // false = agent can work without a channel
}
```

### Auth Flow Changes

**`gateway/provider_auth.go`** — extend `BuildProviderAuthPrompt` and `HandleProviderAuthInput`:

For `AuthModeAPIKey` providers with `BaseURLEnv`:

```go
// When provider has BaseURLEnv and no DefaultBase:
"Please paste your API key for **OpenAI-Compatible (custom)** (env: `OPENAI_COMPATIBLE_API_KEY`).

Then provide the base URL (env: `OPENAI_COMPATIBLE_BASE_URL`), e.g.:
  https://my-server.example.com/v1

Reply with: <api-key> or KEY=<api-key> URL=<base-url>"

// When provider has BaseURLEnv WITH DefaultBase:
"Please paste your API key for **OpenRouter** (env: `OPENROUTER_API_KEY`).
Default base URL: https://openrouter.ai/api/v1
To override, reply: KEY=<api-key> URL=<custom-url>"
```

For `AuthModeNone` with `BaseURLEnv` (Ollama):
```
"**Ollama (local)** requires no API key.
Default endpoint: http://localhost:11434/v1
To use a custom endpoint, reply: URL=<your-url>
Or reply `/onboard confirm` to use the default."
```

### ProviderEnvVarsToSet Enhancement

```go
func ProviderEnvVarsToSet(p *LLMProvider, value string, baseURL string) map[string]string {
    out := map[string]string{}
    
    // API key
    if p.EnvVar != "" && strings.TrimSpace(value) != "" {
        out[p.EnvVar] = strings.TrimSpace(value)
    }
    
    // Base URL
    if p.BaseURLEnv != "" {
        url := strings.TrimSpace(baseURL)
        if url == "" {
            url = p.DefaultBase
        }
        if url != "" {
            out[p.BaseURLEnv] = url
        }
    }
    
    // Compatibility aliases
    if strings.EqualFold(p.ID, "openai-codex") {
        out["OPENAI_API_KEY"] = strings.TrimSpace(value)
    }
    
    return out
}
```

### Category Update

Add "compatible" to the category order in `buildProviderListResponse`:

```go
categoryOrder := []struct{ key, label string }{
    {"builtin",    "☁️  Built-in (API key)"},
    {"custom",     "🔐 Custom / OAuth"},
    {"compatible", "🔌 OpenAI-Compatible"},   // NEW
    {"generic",    "🖥️  Generic"},
}
```

### Provider Mapping for Managed Agents

Extend `MapToManagedProvider`:

```go
func MapToManagedProvider(providerID string) string {
    normalized := strings.ToLower(strings.TrimSpace(providerID))
    switch normalized {
    case "openai-codex", "openai-compatible":
        return "openai"
    case "openrouter":
        return "openrouter"  // keep distinct — managed configs need base_url
    case "ollama":
        return "ollama"      // keep distinct
    default:
        return normalized
    }
}
```

---

## Part 4: Channel-Optional Agents

### Problem

The current onboarding flow (`onboard.go`) requires channel selection for all managed agents:
```go
if isManagedAgent(agentID) {
    sess.Step = OnboardChannelSelect  // forced
}
```

Codex and OpenCode don't need Telegram/Discord — they're invoked per-task via the codeagent adapter, not through chat channels.

### Proposed Design

**Add a `ChannelPolicy` field to `shared/catalog`:**

```go
// ChannelPolicy describes an agent's channel requirements.
type ChannelPolicy string

const (
    ChannelRequired ChannelPolicy = "required"  // must configure a channel (picoclaw, openclaw, zeroclaw)
    ChannelOptional ChannelPolicy = "optional"  // can configure but not required (future agents)
    ChannelNone     ChannelPolicy = "none"      // no channel support (codex, opencode)
)
```

**Add to agent spec in `shared/catalog` (new `AgentSpec` type):**

Rather than scattering agent metadata across `daemon/internal/catalog/catalog.go` (Entry), `gateway/managed_onboard.go` (managedAgentConfig), and `gateway/managed_onboard.go` (channels list), consolidate into a single shared spec:

```go
// In shared/catalog/agents.go (new file)

// AgentSpec is the full agent specification shared across packages.
type AgentSpec struct {
    ID             string        `json:"id"`
    Name           string        `json:"name"`
    Version        string        `json:"version"`
    Status         string        `json:"status"`       // "active" | "candidate"
    Capabilities   []string      `json:"capabilities"`
    Description    string        `json:"description"`
    ChannelPolicy  ChannelPolicy `json:"channel_policy"`
    ConfigDir      string        `json:"-"`             // e.g. ".codex"
    ConfigFile     string        `json:"-"`             // e.g. "config.json"
    RequiredEnvKey string        `json:"-"`             // e.g. "OPENAI_API_KEY"
    Channels       []string      `json:"-"`             // supported channel IDs, empty = none
}
```

This consolidation means `gateway/managed_onboard.go` can look up agent properties from `shared/catalog` instead of maintaining its own parallel `managedAgents` map and `managedAgentChannels()` function.

### Onboarding Flow Change

```go
func onboardSelectAgent(...) {
    // ...existing agent lookup...
    
    agentSpec := catalog.GetAgentSpec(agentID)
    
    switch agentSpec.ChannelPolicy {
    case catalog.ChannelNone:
        // Skip channel selection entirely
        sess.Step = OnboardAgentSelected
        // No SelectedChannel needed
        
    case catalog.ChannelOptional:
        // Ask but allow skip
        sess.Step = OnboardChannelSelect
        // Show: "Choose a channel, or reply /onboard skip to proceed without one"
        
    case catalog.ChannelRequired:
        // Current behavior
        sess.Step = OnboardChannelSelect
    }
}
```

### Managed Config Generation

For channel-none agents, `prepareManagedOnboard` skips channel validation:

```go
func prepareManagedOnboard(agentID string, sess *OnboardSession, actor string) (*managedOnboardResult, error) {
    agentSpec := catalog.GetAgentSpec(agentID)
    
    // Channel validation — respect policy
    if agentSpec.ChannelPolicy == catalog.ChannelRequired {
        if strings.TrimSpace(sess.SelectedChannel) == "" {
            return nil, fmt.Errorf("%s requires a channel", agentID)
        }
        // ...existing channel token validation...
    }
    // ChannelNone and ChannelOptional: proceed without channel
    
    // ...rest of config generation...
}
```

For codex/opencode, the generated config is simpler — just provider/model/env, no channel block:

```go
func buildManagedCodeAgentJSONConfig(
    provider *LLMProvider,
    providerKey, providerToken, modelID, workspacePath string,
) map[string]interface{} {
    return map[string]interface{}{
        "provider":       providerKey,
        "model":          modelID,
        "workspace_root": workspacePath,
    }
}
```

---

## Part 5: Config Schema Changes

### configv2 Changes

**`configv2/config.go`** — the `Channel` slice becomes truly optional:

```go
type Config struct {
    ConfigVersion int           `json:"config_version"`
    Channels      []Channel     `json:"channels,omitempty"` // omitempty for channel-none agents
    ModelList     []Model       `json:"model_list"`
    DefaultModel  string        `json:"default_model"`
    BaseAgent     BaseAgentSpec `json:"base_agent"`
    ConfiguredAt  string        `json:"configured_at"`
}
```

**Model entry gains `base_url`:**

```go
type Model struct {
    ModelName     string `json:"model_name"`
    Model         string `json:"model"`
    ProviderID    string `json:"provider_id"`
    AuthMode      string `json:"auth_mode,omitempty"`
    EnvVar        string `json:"env_var,omitempty"`
    CredentialRef string `json:"credential_ref,omitempty"`
    BaseURL       string `json:"base_url,omitempty"`  // NEW: for OpenAI-compatible providers
}
```

**`ApplyGatewayEnvironment`** — set base URL env vars:

```go
for _, m := range cfg.ModelList {
    // ...existing credential resolution...
    
    // Base URL for compatible providers
    if m.BaseURL != "" {
        provider := catalog.GetProvider(m.ProviderID)
        if provider != nil && provider.BaseURLEnv != "" {
            if err := setEnvIfUnset(provider.BaseURLEnv, m.BaseURL); err != nil {
                return err
            }
        }
    }
}
```

### Validation Relaxation

`validateConfig` currently requires all channels to be in the supported list. This remains unchanged — but we add validation that compatible providers have valid base URLs:

```go
func validateConfig(cfg *Config) error {
    // ...existing validation...
    
    for _, model := range cfg.ModelList {
        if model.BaseURL != "" {
            if !strings.HasPrefix(model.BaseURL, "http://") && !strings.HasPrefix(model.BaseURL, "https://") {
                return fmt.Errorf("model %q base_url must start with http:// or https://", model.ModelName)
            }
        }
    }
    return nil
}
```

---

## Part 6: Test Strategy

### Principle: Table-Driven, No Duplication

The previous PR #1491 had review comments about near-identical test functions for codex vs opencode. The solution:

### 1. Npm Agent Manifest Tests (table-driven)

**File: `daemon/internal/catalog/manifests_npm_test.go`**

```go
func TestNpmAgentManifests(t *testing.T) {
    for _, spec := range npmAgentSpecs {
        t.Run(spec.ID, func(t *testing.T) {
            m := BuildNpmAgentManifest(spec)
            
            // Common assertions for all npm agents
            if err := m.Validate(); err != nil {
                t.Fatalf("manifest validation failed: %v", err)
            }
            if m.Runtime.Type != manifest.RuntimeTypeNpmCLI {
                t.Errorf("runtime.type = %q, want %q", m.Runtime.Type, manifest.RuntimeTypeNpmCLI)
            }
            if m.ID != spec.ID {
                t.Errorf("manifest.ID = %q, want %q", m.ID, spec.ID)
            }
            
            // Install command should reference the npm package
            installCmd, err := m.Runtime.Install.ResolveForGOOS("linux")
            if err != nil {
                t.Fatalf("resolve linux install: %v", err)
            }
            if !strings.Contains(installCmd, spec.NpmPackage) {
                t.Errorf("install command should reference %q, got: %s", spec.NpmPackage, installCmd)
            }
            
            // Install command should include post-install verification
            if !strings.Contains(installCmd, "command -v "+spec.BinaryName) {
                t.Errorf("install command should verify %q is available after install", spec.BinaryName)
            }
            
            // Stop should be signal:term
            stopCmd, err := m.Runtime.Stop.ResolveForCurrentOS()
            if err != nil {
                t.Fatalf("resolve stop: %v", err)
            }
            if stopCmd != "signal:term" {
                t.Errorf("stop = %q, want signal:term", stopCmd)
            }
        })
    }
}

func TestNpmAgentManifestPlatforms(t *testing.T) {
    platforms := []string{"linux", "darwin", "windows"}
    for _, spec := range npmAgentSpecs {
        for _, os := range platforms {
            t.Run(spec.ID+"/"+os, func(t *testing.T) {
                m := BuildNpmAgentManifest(spec)
                cmd, err := m.Runtime.Install.ResolveForGOOS(os)
                if err != nil {
                    t.Fatalf("resolve install for %s: %v", os, err)
                }
                if cmd == "" {
                    t.Error("install command should not be empty")
                }
            })
        }
    }
}
```

### 2. Auto-Install Bwrap Tests (inject dependencies)

```go
func TestAttemptAutoInstallBwrap(t *testing.T) {
    cases := []struct {
        name           string
        pkgManager     string // which package manager is "installed"
        sudoAvailable  bool
        sudoSucceeds   bool
        directSucceeds bool
        bwrapAfter     bool   // is bwrap available after install?
        wantErr        bool
        wantSudo       bool
    }{
        {
            name:          "apt-get with sudo",
            pkgManager:    "apt-get",
            sudoAvailable: true,
            sudoSucceeds:  true,
            bwrapAfter:    true,
        },
        {
            name:           "dnf without sudo (root)",
            pkgManager:     "dnf",
            sudoAvailable:  false,
            directSucceeds: true,
            bwrapAfter:     true,
        },
        {
            name:           "sudo fails, direct succeeds",
            pkgManager:     "apt-get",
            sudoAvailable:  true,
            sudoSucceeds:   false,
            directSucceeds: true,
            bwrapAfter:     true,
        },
        {
            name:           "both fail",
            pkgManager:     "yum",
            sudoAvailable:  true,
            sudoSucceeds:   false,
            directSucceeds: false,
            wantErr:        true,
        },
        {
            name:       "no package manager",
            pkgManager: "",
            wantErr:    true,
        },
        {
            name:          "install succeeds but bwrap not in PATH",
            pkgManager:    "apt-get",
            sudoAvailable: true,
            sudoSucceeds:  true,
            bwrapAfter:    false, // install "succeeded" but binary missing
            wantErr:       true,
        },
    }
    
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            // Inject mock lookPath/exec functions
            // ...test body...
        })
    }
}
```

### 3. Provider Auth Tests (table-driven for compatible providers)

```go
func TestCompatibleProviderAuthFlow(t *testing.T) {
    compatibleProviders := []string{"openrouter", "ollama", "openai-compatible"}
    
    for _, providerID := range compatibleProviders {
        t.Run(providerID, func(t *testing.T) {
            p := GetLLMProvider(providerID)
            if p == nil {
                t.Fatalf("provider %q not found in catalog", providerID)
            }
            
            // Test prompt generation
            prompt := BuildProviderAuthPrompt(p)
            if prompt == "" {
                t.Error("prompt should not be empty")
            }
            
            // Test auth handling
            switch p.AuthMode {
            case AuthModeAPIKey:
                result, err := HandleProviderAuthInput(p, "sk-test-key-123")
                if err != nil {
                    t.Fatalf("handle auth input: %v", err)
                }
                if !result.Done {
                    t.Error("API key input should complete auth")
                }
                if result.EnvVar != p.EnvVar {
                    t.Errorf("env var = %q, want %q", result.EnvVar, p.EnvVar)
                }
            case AuthModeNone:
                result, err := HandleProviderAuthInput(p, "confirm")
                if err != nil {
                    t.Fatalf("handle auth input: %v", err)
                }
                if !result.Done {
                    t.Error("none-auth should auto-complete")
                }
            }
        })
    }
}
```

### 4. Channel-Optional Onboarding Tests

```go
func TestOnboardChannelPolicies(t *testing.T) {
    cases := []struct {
        agentID        string
        channelPolicy  catalog.ChannelPolicy
        expectChannel  bool // should flow ask for channel?
    }{
        {"openclaw",  catalog.ChannelRequired, true},
        {"picoclaw",  catalog.ChannelRequired, true},
        {"zeroclaw",  catalog.ChannelRequired, true},
        {"codex",     catalog.ChannelNone,     false},
        {"opencode",  catalog.ChannelNone,     false},
    }
    
    for _, tc := range cases {
        t.Run(tc.agentID, func(t *testing.T) {
            // Mock onboard flow, verify channel step is skipped/included
        })
    }
}
```

### 5. Integration: Shared Test Fixtures

Create `daemon/internal/catalog/testutil_test.go`:

```go
func sampleNpmManifest(id string) manifest.Manifest {
    for _, spec := range npmAgentSpecs {
        if spec.ID == id {
            return BuildNpmAgentManifest(spec)
        }
    }
    // Return a generic test manifest
    return BuildNpmAgentManifest(NpmAgentSpec{
        ID:          id,
        Name:        "Test Agent",
        NpmPackage:  "test-agent-pkg",
        BinaryName:  "testagent",
        Description: "Test agent for unit tests",
    })
}
```

---

## Appendix: File Change Map

### New Files

| File | Purpose |
|---|---|
| `daemon/internal/lifecycle/isolation_autoinstall.go` | Auto-install bwrap logic |
| `daemon/internal/lifecycle/isolation_autoinstall_test.go` | Tests for auto-install |
| `daemon/internal/catalog/manifests_npm.go` | Shared npm agent manifest builder + codex/opencode specs |
| `daemon/internal/catalog/manifests_npm_test.go` | Table-driven tests for npm manifests |
| `shared/catalog/agents.go` | Consolidated `AgentSpec` with `ChannelPolicy` |
| `shared/catalog/agents_test.go` | Agent spec tests |
| `catalog/codex.manifest.json` | JSON manifest for Codex (mirrors Go code, for external tooling) |
| `catalog/opencode.manifest.json` | JSON manifest for OpenCode |

### Modified Files

| File | Changes |
|---|---|
| `shared/catalog/catalog.go` | Add `BaseURLEnv`, `DefaultBase` to `ProviderSpec`; add openrouter/ollama providers; add "compatible" category; add `ChannelPolicy` |
| `daemon/internal/catalog/catalog.go` | Add codex/opencode to `DefaultEntries()` |
| `daemon/internal/lifecycle/isolation_backend.go` | Extract common ensure-deps script; integrate auto-install before failing |
| `daemon/internal/lifecycle/install.go` | Call `attemptAutoInstallBwrap()` on Linux when bwrap missing |
| `gateway/managed_onboard.go` | Add codex/opencode to `managedAgents`; respect `ChannelPolicy`; generate simpler configs for channel-none agents |
| `gateway/onboard.go` | Skip channel step when `ChannelPolicy == ChannelNone` |
| `gateway/provider_auth.go` | Handle base URL prompting for compatible providers |
| `gateway/llm_providers.go` | No structural changes (delegates to shared/catalog) |
| `configv2/config.go` | Add `BaseURL` field to `Model`; `omitempty` on Channels; apply base URL env vars |

### Lines of Code Estimate

| Component | New | Modified | Tests |
|---|---|---|---|
| Auto-install bwrap | ~150 | ~30 | ~200 |
| Npm agent manifests | ~120 | ~20 | ~150 |
| Provider auth (compatible) | ~40 | ~60 | ~100 |
| Channel-optional | ~50 | ~80 | ~80 |
| Config schema | ~10 | ~25 | ~40 |
| **Total** | **~370** | **~215** | **~570** |

### Migration / Backwards Compatibility

1. **Config v2 schema** — adding optional fields (`base_url`, `omitempty` on channels) is backwards compatible
2. **Provider catalog** — adding new providers doesn't break existing configs; renaming `openai-compatible` category from "generic" to "compatible" needs a migration in `ProvidersByCategory()` callers
3. **Manifest validation** — `RuntimeTypeNpmCLI` is already defined but unused; enabling it is safe
4. **Auto-install** — purely additive; only activates when bwrap is missing and Linux is detected

---

## Open Questions

1. **Should auto-install prompt the user before running `sudo`?** Current design uses `sudo -n` (non-interactive only) which is safe but might silently fail. Alternative: detect if interactive, ask user to confirm.

2. **Should codex/opencode manifests have a health check type of "command" instead of "process"?** These are per-invocation tools, not long-running daemons. A `codex --version` health check is more meaningful than process-state monitoring. This may require extending `HealthcheckSpec.Type` to support `"command"` with a command string.

3. **Ollama base URL discovery**: Should we auto-detect running Ollama instances (check localhost:11434) during onboarding? This would improve UX but adds a network probe during setup.

4. **npm prefix isolation**: Should npm-installed agents use `--prefix ~/.carrier/npm` to avoid polluting the global npm space? This would require PATH manipulation in start commands.
