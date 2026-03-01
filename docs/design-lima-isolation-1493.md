# Design: Secure Lima Isolation for macOS (Issue #1493)

**Author:** Security engineering review  
**Status:** DRAFT — design only, not yet implemented  
**Date:** 2026-03-01  

---

## 1. Problem Statement

Lima's **default template** mounts the user's entire home directory (`~`) read-only into the VM. When Carrier uses `limactl create -y --name <id>` (the current code path in `isolation_backend.go`), the isolated agent gains read access to:

- `~/.ssh/` — SSH private keys  
- `~/.aws/` — AWS credentials  
- `~/.config/` — app tokens and configs  
- `~/.gnupg/` — GPG keys  
- `~/.carrier/` — Carrier's own state, including other agents' data  

Read access to secrets is a security breach for a feature whose entire purpose is isolation. Additionally, the current implementation uses a **single shared Lima instance** (defaulting to `"default"` or `CARRIER_ISOLATION_LIMA_INSTANCE`), which means all isolated agents share the same VM — defeating per-agent isolation.

### Previous Failed Approach (PR #1497)

The closed PR had these blocking issues:
1. **YAML injection** — agent workspace path was interpolated directly into a YAML template string without escaping
2. **Path traversal in cleanup** — instance names derived from user input were used in `os.RemoveAll()` without validation
3. **State regression** — the `Isolated` flag was set on install but never cleared on uninstall
4. **Global instance reuse** — all agents shared one Lima instance

This design addresses all four from scratch.

---

## 2. Architecture Overview

### 2.1 Per-Agent Lima Instances

Each agent that requests isolation gets its **own dedicated Lima VM instance** with a deterministic, validated name:

```
carrier-<agentID>-<random4hex>
```

Example: `carrier-openclaw-a3f2`

The instance name is generated at install time, stored in agent state, and reused for start/stop/uninstall operations. This eliminates global instance reuse.

### 2.2 Custom Lima Template Generation

Instead of relying on the default template, Carrier generates a minimal YAML template per instance that:

1. Mounts **only** the agent's workspace directory (writable)
2. Explicitly sets **no other mounts** (overriding Lima's default `~` mount)
3. Provisions bubblewrap (`bwrap`) inside the guest
4. Optionally configures network policy

The template is written to `~/.carrier/lima/templates/<instance-name>.yaml`.

### 2.3 Data Flow

```
Install with Isolation:
  1. Validate agent ID and workspace path
  2. Generate unique instance name → validate against allowlist pattern
  3. Generate YAML template (using Go encoding/yaml, NOT string interpolation)
  4. Write template to ~/.carrier/lima/templates/<name>.yaml
  5. limactl create --name=<name> <template-path>
  6. limactl start <name>
  7. Provision bwrap inside guest
  8. Run install command inside guest
  9. Store instance name in agent state

Start with Isolation:
  1. Read instance name from agent state
  2. Verify instance exists (limactl list)
  3. limactl start <name> (if not running)
  4. Execute start command wrapped in lima shell + bwrap

Uninstall:
  1. Stop agent
  2. limactl stop <name>
  3. limactl delete <name>
  4. Remove template file
  5. Clear isolation state
```

---

## 3. Security Considerations

### 3.1 YAML Injection Prevention

**Root cause in PR #1497:** Template was generated via `fmt.Sprintf()` or string interpolation, allowing specially-crafted workspace paths to inject arbitrary YAML.

**Solution:** Use Go's `encoding/yaml` (specifically `gopkg.in/yaml.v3` or equivalent) to produce the template programmatically. The template is built as a Go struct, marshaled to YAML, and written to disk. No string interpolation of user-controlled values into YAML.

```go
// LimaTemplate is the Go struct representation, marshaled with encoding/yaml
type LimaTemplate struct {
    Mounts    []LimaMount    `yaml:"mounts"`
    Provision []LimaProvision `yaml:"provision,omitempty"`
    // ... other fields
}

type LimaMount struct {
    Location string `yaml:"location"`
    Writable bool   `yaml:"writable"`
}
```

The workspace path is set as a Go string field value, and `yaml.Marshal` handles all escaping. Even a path like `/tmp/evil\ninjection: true` will be properly quoted in the YAML output.

### 3.2 Path Traversal Protection

Three levels of defense:

#### 3.2.1 Instance Name Validation

Instance names are generated internally with a strict pattern and validated before any filesystem or shell operation:

```go
var validLimaInstanceName = regexp.MustCompile(`^carrier-[a-zA-Z0-9][a-zA-Z0-9._-]*-[0-9a-f]{4,16}$`)
```

This prevents:
- Names like `../../etc/passwd`
- Empty names
- Names with shell metacharacters

The validation function is a single chokepoint:

```go
func validateLimaInstanceName(name string) error {
    if !validLimaInstanceName.MatchString(name) {
        return fmt.Errorf("%w: invalid lima instance name %q", ErrIsolationUnavailable, name)
    }
    return nil
}
```

Every function that uses an instance name calls this first, including `PrepareCommands()`, `WrapCommand()`, cleanup, and template path resolution.

#### 3.2.2 Workspace Path Validation

The workspace path must:
- Be an absolute path
- Exist and be a directory
- Be within `~/.carrier/instances/` (or `CARRIER_INSTANCE_STORE` parent)
- Not contain `..` segments after `filepath.Clean()`
- Not be a symlink to outside the allowed prefix (use `filepath.EvalSymlinks()`)

```go
func validateWorkspacePath(workspacePath string) error {
    cleaned := filepath.Clean(workspacePath)
    if !filepath.IsAbs(cleaned) {
        return fmt.Errorf("workspace path must be absolute: %q", workspacePath)
    }
    
    resolved, err := filepath.EvalSymlinks(cleaned)
    if err != nil {
        return fmt.Errorf("resolve workspace path: %w", err)
    }
    
    allowedPrefix := resolveAllowedWorkspacePrefix() // ~/.carrier/instances/
    if !strings.HasPrefix(resolved+"/", allowedPrefix+"/") {
        return fmt.Errorf("workspace path %q is outside allowed prefix %q", resolved, allowedPrefix)
    }
    
    info, err := os.Stat(resolved)
    if err != nil {
        return fmt.Errorf("stat workspace path: %w", err)
    }
    if !info.IsDir() {
        return fmt.Errorf("workspace path is not a directory: %q", resolved)
    }
    
    return nil
}
```

#### 3.2.3 Template Path Confinement

Template files are written to a fixed directory (`~/.carrier/lima/templates/`), and the filename is derived only from the validated instance name:

```go
func templatePath(instanceName string) (string, error) {
    if err := validateLimaInstanceName(instanceName); err != nil {
        return "", err
    }
    root := limaTemplateDir() // ~/.carrier/lima/templates/
    path := filepath.Join(root, instanceName+".yaml")
    // Double-check the result is still under root (defense in depth)
    if !strings.HasPrefix(filepath.Clean(path)+string(os.PathSeparator), filepath.Clean(root)+string(os.PathSeparator)) {
        return "", fmt.Errorf("template path escapes root: %q", path)
    }
    return path, nil
}
```

### 3.3 State Lifecycle Correctness

**Problem in PR #1497:** The `Isolated` flag was set during install but never cleared during uninstall, causing stale state.

**Solution:** Track isolation state as part of `AgentState` with explicit transitions:

```go
// In types.go — add to AgentState:
type AgentState struct {
    // ... existing fields ...
    Isolated         bool   `json:"isolated"`
    LimaInstanceName string `json:"limaInstanceName,omitempty"`
}
```

State transitions:

| Operation      | `Isolated` | `LimaInstanceName` |
|---------------|------------|---------------------|
| Install (no isolation) | `false` | `""` |
| Install (with isolation) | `true` | `"carrier-<id>-<hex>"` |
| Start (with isolation) | `true` (preserved) | preserved |
| Uninstall | `false` | `""` |
| Re-install (no isolation) | `false` | `""` |

The **uninstall path** must:
1. Read `LimaInstanceName` from state
2. If non-empty: stop + delete the Lima instance, remove the template file
3. Clear both `Isolated` and `LimaInstanceName`
4. Continue with normal uninstall logic

The **install path** must:
1. If existing state has `LimaInstanceName` set, clean up the old instance first
2. Generate new instance name and set both fields atomically

This is also persisted to `PersistedAgentState`:

```go
type PersistedAgentState struct {
    // ... existing fields ...
    Isolated         bool   `json:"isolated,omitempty"`
    LimaInstanceName string `json:"lima_instance_name,omitempty"`
}
```

### 3.4 Shell Command Safety

The existing `shellSingleQuote()` function is already used for quoting. The design continues to use it for all values interpolated into shell commands (instance names, paths, etc.). Instance names and paths are validated before quoting as an additional layer.

### 3.5 VM Cleanup on Uninstall

The cleanup must be robust against partial failures:

```go
func (b *perAgentLimaBackend) Cleanup() error {
    if err := validateLimaInstanceName(b.instanceName); err != nil {
        return err
    }
    
    var errs []error
    
    // 1. Stop the instance (ignore "not running" errors)
    if err := runCommand(b.limactlPath, "stop", b.instanceName); err != nil {
        if !isInstanceNotRunningError(err) {
            errs = append(errs, fmt.Errorf("stop instance: %w", err))
        }
    }
    
    // 2. Delete the instance (ignore "not found" errors)
    if err := runCommand(b.limactlPath, "delete", b.instanceName); err != nil {
        if !isInstanceNotFoundError(err) {
            errs = append(errs, fmt.Errorf("delete instance: %w", err))
        }
    }
    
    // 3. Remove the template file
    tmplPath, err := templatePath(b.instanceName)
    if err == nil {
        if removeErr := os.Remove(tmplPath); removeErr != nil && !os.IsNotExist(removeErr) {
            errs = append(errs, fmt.Errorf("remove template: %w", removeErr))
        }
    }
    
    return errors.Join(errs...)
}
```

### 3.6 No Global Instance Reuse

The current code defaults to `"default"` instance. The new design:

1. **Removes** `defaultLimaInstanceName` and `CARRIER_ISOLATION_LIMA_INSTANCE` env var support
2. Each `limaIsolationBackend` is initialized with a unique per-agent instance name
3. Instance name is stored in agent state and not shared

### 3.7 Race Condition Protection

Two agents installing simultaneously could attempt to create instances with the same name if the random suffix collides. Mitigations:
- Use 8 bytes (16 hex chars) of randomness instead of 4 for the suffix
- The `limactl create` command itself will fail if the instance already exists
- The retry-with-new-name pattern handles collisions gracefully

### 3.8 Network Isolation (Optional, Phase 2)

Lima supports network configuration in the template. For future hardening:

```yaml
networks: []  # No shared networks — equivalent to --isolation-network=none
```

This is out of scope for the initial implementation but the template struct should include placeholder support.

---

## 4. Detailed File Changes

### 4.1 New File: `daemon/internal/lifecycle/lima_template.go`

Responsible for:
- Go struct types mirroring Lima YAML schema (subset)
- `generateLimaTemplate(instanceName, workspacePath string) ([]byte, error)` — returns YAML bytes
- `writeLimaTemplate(instanceName, workspacePath string) (templatePath string, err error)`
- `removeLimaTemplate(instanceName string) error`
- `validateLimaInstanceName(name string) error`
- `validateWorkspacePath(path string) error`
- `limaTemplateDir() string`

Key design: **All YAML generation uses `encoding/yaml` marshal, never string interpolation.**

### 4.2 Modified File: `daemon/internal/lifecycle/isolation_backend.go`

Replace `limaIsolationBackend` with `perAgentLimaIsolationBackend`:

```go
type perAgentLimaIsolationBackend struct {
    limactlPath   string
    instanceName  string   // validated, per-agent
    workspacePath string   // validated, absolute
}
```

Changes to methods:

- **`PrepareCommands()`**: 
  - Generate template if not exists
  - `limactl create --name=<name> <template-path>` (instead of default)
  - `limactl start <name>`
  - Guest bwrap provisioning (same as before)

- **`WrapCommand()`**: Same structure but uses validated `instanceName`

- **`Cleanup()`**: New method — stop, delete, remove template

- **Remove**: `defaultLimaInstanceName`, `CARRIER_ISOLATION_LIMA_INSTANCE` constant/env lookup

### 4.3 Modified File: `daemon/internal/lifecycle/types.go`

Add to `AgentState`:
```go
Isolated         bool   `json:"isolated"`
LimaInstanceName string `json:"limaInstanceName,omitempty"`
```

### 4.4 Modified File: `daemon/internal/lifecycle/state_file.go`

Add to `PersistedAgentState`:
```go
Isolated         bool   `json:"isolated,omitempty"`
LimaInstanceName string `json:"lima_instance_name,omitempty"`
```

Update `applyPersistedState()` and `saveState()` to handle these fields.

### 4.5 Modified File: `daemon/internal/lifecycle/install.go`

In `InstallWithOptions()`:
- When `opts.Isolation` is true:
  1. Generate unique instance name: `carrier-<agentID>-<random16hex>`
  2. Validate instance name
  3. Resolve workspace path from managed instances or default
  4. Validate workspace path
  5. Create `perAgentLimaIsolationBackend` with instance name and workspace path
  6. After successful install, store `Isolated=true` and `LimaInstanceName` in state

- If re-installing with isolation over an existing isolated install:
  1. Clean up old Lima instance first
  2. Proceed with fresh instance

### 4.6 Modified File: `daemon/internal/lifecycle/start_stop.go`

In `StartWithOptions()`:
- When `opts.Isolation` is true:
  1. Read `LimaInstanceName` from agent state (must be non-empty)
  2. Validate the stored instance name
  3. Create `perAgentLimaIsolationBackend` using stored instance name
  4. Existing start flow continues

### 4.7 Modified File: `daemon/internal/lifecycle/uninstall.go`

In `Uninstall()`:
- Before resetting state:
  1. Read `LimaInstanceName` from state
  2. If non-empty, call cleanup (stop + delete instance, remove template)
  3. Log cleanup success/failure
- Clear `Isolated` and `LimaInstanceName` in the state reset block

### 4.8 Modified File: `daemon/internal/lifecycle/service.go`

In `resolveIsolationBackend()`:
- Change Darwin case to accept workspace path and instance name parameters
- Factory method: `resolvePerAgentLimaBackend(instanceName, workspacePath string) (isolationBackend, error)`

Alternatively, extend the `isolationBackend` interface:

```go
type isolationBackend interface {
    CommandGOOS() string
    WrapCommand(command string) (string, error)
    WrapStartCommand(startCommand string) (string, error)
    PrepareCommands() ([]string, error)
    Cleanup() error  // NEW
}
```

### 4.9 Modified File: `go.mod`

Add `gopkg.in/yaml.v3` dependency (if not already present) for safe YAML generation.

### 4.10 Modified Files: `daemon/internal/api/server.go`, `gateway/daemonclient.go`

The `daemonAgent` response struct should expose `isolated` and `limaInstanceName` fields so the gateway and web UI can show isolation status. No changes to the install/start request format (the `isolation` boolean already exists).

---

## 5. Lima Template Specification

The generated template for an agent with workspace at `~/.carrier/instances/openclaw-a3f2/workspace`:

```yaml
# Managed by Carrier — do not edit manually
images:
  - location: "https://cloud-images.ubuntu.com/releases/24.04/release/ubuntu-24.04-server-cloudimg-amd64.img"
    arch: "x86_64"
  - location: "https://cloud-images.ubuntu.com/releases/24.04/release/ubuntu-24.04-server-cloudimg-arm64.img"
    arch: "aarch64"

mounts:
  - location: "/Users/username/.carrier/instances/openclaw-a3f2/workspace"
    writable: true

mountType: "reverse-sshfs"

provision:
  - mode: system
    script: |
      #!/bin/bash
      set -eu
      apt-get update -qq
      apt-get install -y -qq bubblewrap git curl tar bash

cpus: 2
memory: "2GiB"
disk: "10GiB"
```

Critical: The `mounts` array contains **only** the agent workspace. Lima's default `~` mount is not present, so it is not applied.

---

## 6. Instance Name Generation

```go
func generateLimaInstanceName(agentID string) (string, error) {
    cleanID := strings.ToLower(strings.TrimSpace(agentID))
    if cleanID == "" {
        return "", fmt.Errorf("agent ID is required for instance name generation")
    }
    // Sanitize: keep only alphanumeric and hyphens
    sanitized := sanitizeForInstanceName(cleanID)
    if sanitized == "" {
        return "", fmt.Errorf("agent ID %q produces empty sanitized name", agentID)
    }
    
    buf := make([]byte, 8)
    if _, err := io.ReadFull(rand.Reader, buf); err != nil {
        return "", fmt.Errorf("generate random suffix: %w", err)
    }
    
    name := fmt.Sprintf("carrier-%s-%s", sanitized, hex.EncodeToString(buf))
    if err := validateLimaInstanceName(name); err != nil {
        return "", err
    }
    return name, nil
}

func sanitizeForInstanceName(s string) string {
    var b strings.Builder
    for _, r := range s {
        if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
            b.WriteRune(r)
        }
    }
    return strings.Trim(b.String(), "-")
}
```

---

## 7. Rollback / Error Handling

### 7.1 Template Write Failure
If template write fails, installation aborts before creating any Lima instance. No cleanup needed.

### 7.2 `limactl create` Failure
Template file exists but instance does not. Cleanup: remove template file, report error.

### 7.3 `limactl start` Failure
Instance exists but is not running. Cleanup: `limactl delete`, remove template, report error.

### 7.4 Guest Provisioning Failure
Instance is running but bwrap is not installed. Cleanup: `limactl stop`, `limactl delete`, remove template.

### 7.5 Install Command Failure (inside guest)
Full cleanup of Lima instance, or keep for retry (configurable). The existing retry loop already handles this; the instance stays alive during retries and is cleaned up on final failure.

### 7.6 Daemon Restart
On restart, `loadPersistedState()` restores `LimaInstanceName`. The instance may or may not still be running in Lima. The start flow verifies instance status via `limactl list` and starts if needed.

---

## 8. Test Strategy

### 8.1 Unit Tests — `lima_template_test.go`

| Test | What it verifies |
|------|-----------------|
| `TestGenerateLimaTemplateProducesValidYAML` | Marshal/unmarshal roundtrip; only workspace mount present |
| `TestGenerateLimaTemplateNoHomeMount` | Explicitly verify `~` is not in mounts |
| `TestGenerateLimaTemplateEscapesSpecialPaths` | Workspace with quotes, newlines, unicode |
| `TestValidateLimaInstanceNameAcceptsValid` | Normal names pass |
| `TestValidateLimaInstanceNameRejectsTraversal` | `../../etc`, empty, metacharacters |
| `TestValidateWorkspacePathRejectsOutsidePrefix` | `/etc/passwd`, `/tmp/evil`, symlinks |
| `TestValidateWorkspacePathAcceptsValid` | Normal workspace paths |
| `TestTemplatePathConfinement` | Result is always under template dir |
| `TestGenerateLimaInstanceNameUniqueness` | 1000 calls produce unique names |
| `TestSanitizeForInstanceName` | Various agent IDs sanitize correctly |

### 8.2 Unit Tests — `isolation_backend_test.go` (extend existing)

| Test | What it verifies |
|------|-----------------|
| `TestPerAgentLimaPrepareCommandsUsesCustomTemplate` | `limactl create` uses template path, not default |
| `TestPerAgentLimaWrapCommandUsesInstanceName` | Shell command references correct instance |
| `TestPerAgentLimaCleanupStopsAndDeletes` | Cleanup calls stop + delete + remove template |
| `TestPerAgentLimaCleanupIgnoresNotFound` | Non-existent instance doesn't error |

### 8.3 Integration Tests — `install_isolation_pipeline_test.go` (extend existing)

| Test | What it verifies |
|------|-----------------|
| `TestInstallWithIsolationCreatesPerAgentInstance` | Instance name stored in state |
| `TestInstallWithIsolationGeneratesTemplate` | Template file exists with correct content |
| `TestUninstallCleansUpLimaInstance` | Instance name cleared, cleanup commands issued |
| `TestReinstallCleansUpOldInstance` | Old instance deleted before new one created |
| `TestInstallIsolationStatePersistedAcrossRestart` | State file round-trip preserves instance name |

### 8.4 State Lifecycle Tests

| Test | What it verifies |
|------|-----------------|
| `TestIsolatedFlagSetOnInstall` | `state.Isolated == true` after isolated install |
| `TestIsolatedFlagClearedOnUninstall` | `state.Isolated == false` after uninstall |
| `TestIsolatedFlagClearedOnNonIsolatedReinstall` | Reinstall without isolation clears flag |
| `TestLimaInstanceNamePersistedAndRestored` | Full persist/load cycle |

### 8.5 Security-Focused Tests

| Test | What it verifies |
|------|-----------------|
| `TestYAMLInjectionViaMaliciousWorkspacePath` | Paths with YAML metacharacters don't inject |
| `TestPathTraversalInInstanceNameRejected` | `../../` in instance name is rejected |
| `TestPathTraversalInCleanupRejected` | Cleanup with tampered instance name fails safely |
| `TestSymlinkEscapeInWorkspaceRejected` | Symlink pointing outside prefix is rejected |
| `TestTemplateContainsNoHomeMount` | Grep generated YAML for absence of `~` mount |

### 8.6 Fuzz Tests (optional, recommended)

```go
func FuzzValidateLimaInstanceName(f *testing.F) {
    f.Add("carrier-openclaw-a3f2b1c4")
    f.Add("../../etc/passwd")
    f.Add("")
    f.Add("carrier-x-\n")
    f.Fuzz(func(t *testing.T, name string) {
        err := validateLimaInstanceName(name)
        if err == nil {
            // If accepted, must match the pattern
            if !validLimaInstanceName.MatchString(name) {
                t.Errorf("accepted invalid name: %q", name)
            }
        }
    })
}

func FuzzGenerateLimaTemplate(f *testing.F) {
    f.Add("carrier-test-abcd1234", "/home/user/.carrier/instances/test/workspace")
    f.Fuzz(func(t *testing.T, instanceName, workspacePath string) {
        data, err := generateLimaTemplate(instanceName, workspacePath)
        if err != nil {
            return // validation rejected — OK
        }
        // Verify output is valid YAML
        var parsed map[string]interface{}
        if yamlErr := yaml.Unmarshal(data, &parsed); yamlErr != nil {
            t.Errorf("generated invalid YAML for inputs (%q, %q): %v", instanceName, workspacePath, yamlErr)
        }
    })
}
```

---

## 9. Migration / Backwards Compatibility

### 9.1 Existing Installations

Agents installed without isolation are unaffected (no state change). Agents installed with the old global Lima instance (`"default"`) will not have `LimaInstanceName` in state. On next install with isolation, a new per-agent instance is created.

### 9.2 Environment Variable Deprecation

`CARRIER_ISOLATION_LIMA_INSTANCE` is removed. If set, log a deprecation warning and ignore it. Each agent gets its own instance name regardless.

### 9.3 State Schema Version

Consider adding a `schema_version` field to the persisted state file. The new fields (`Isolated`, `LimaInstanceName`) are added as `omitempty`, so old state files load without error (they'll just have zero values, which is correct for non-isolated agents).

---

## 10. Open Questions / Deferred Work

1. **Resource limits:** Should we enforce CPU/memory limits per agent VM? (Currently hardcoded to 2 CPU / 2 GiB RAM in the template — should this be configurable?)

2. **Network isolation:** The issue mentions `--isolation-network=none|host|restricted`. This can be a Phase 2 addition. The template struct should have a `Networks` field ready.

3. **Shared base image caching:** Multiple per-agent instances will each download the Ubuntu cloud image. Lima caches these at `~/.lima/_caches/`, so this should be efficient, but worth monitoring disk usage.

4. **Instance lifecycle on daemon restart:** If the daemon crashes, Lima instances continue running. On daemon restart, should we:
   - Resume monitoring existing instances? (Preferred)
   - Stop and recreate them?
   
   Answer: Resume. The instance name is in persisted state; `limactl list` confirms it exists; we reconnect.

5. **Concurrent install protection:** If two `InstallWithOptions` calls race for the same agent, the mutex in `Service` already serializes them. But if different agents generate the same random suffix (astronomically unlikely with 8 bytes), `limactl create` will fail and the retry can generate a new name.

---

## 11. Summary of Security Properties

| Threat | Mitigation |
|--------|-----------|
| Read `~/.ssh/`, `~/.aws/`, etc. from VM | Custom template mounts only agent workspace |
| YAML injection via workspace path | `encoding/yaml` marshal (no string interpolation) |
| Path traversal in instance name | Regex validation + `filepath.Clean` + prefix check |
| Path traversal in cleanup (`os.RemoveAll`) | Instance name validation before any FS operation |
| Stale isolation state after uninstall | Explicit state transitions; `Isolated`+`LimaInstanceName` cleared in `Uninstall()` |
| Cross-agent data access via shared VM | Per-agent instances; no global instance reuse |
| Template file tampering | Template written with `0600` perms; re-generated on install |
| Symlink escape in workspace path | `filepath.EvalSymlinks()` before prefix check |
