# Security Audit: Command Execution in daemon

**Date:** 2026-02-14
**Auditor:** dev01lay2
**Scope:** `daemon/internal/commandexec/runner.go` and all call sites

## Summary

All command execution in the daemon goes through `commandexec.ShellRunner.Run()`,
which passes commands to `/bin/sh -lc` (or `wsl.exe bash -lc` on Windows).

## Call Sites

All four call sites are in `daemon/internal/lifecycle/service.go`:

| Line | Operation | Command Source |
|------|-----------|---------------|
| 266  | Install   | `m.Runtime.Install.Command` |
| 333  | Start     | `m.Runtime.Start.Command` |
| 413  | Upgrade   | `m.Runtime.Upgrade.Command` |
| 460  | Stop      | `m.Runtime.Stop.Command` |

## Finding: Commands Come from Manifests Only

All commands originate from `manifest.Manifest` structs, which are:

1. **Hardcoded in source** (`catalog/manifests.go`) — the `OpenClawManifest()` function
   returns a manifest with fixed command strings.
2. **Loaded from YAML files** via `manifest.Load()` — these are operator-provided
   configuration files, not end-user input.

**No user-supplied input is ever interpolated into commands.** The command strings
are taken verbatim from the manifest and passed to the shell.

## Risk Assessment

| Risk | Level | Notes |
|------|-------|-------|
| Shell injection via user input | **None** | No user input reaches command execution |
| Malicious manifest | **Low** | Manifests are operator-controlled; same trust level as config files |
| Environment variable injection | **Low** | Shell inherits process env; no manifest-controlled env injection |

## Recommendations

1. ✅ Current design is safe — commands come from trusted manifests only
2. Add `ValidateCommand()` to reject obviously dangerous patterns (empty strings,
   null bytes) as defense-in-depth
3. Consider allowlisting manifest command prefixes in future if manifests become
   user-uploadable

## Input Validation Added

- `ValidateCommand()` function in `commandexec/runner.go` to reject empty commands
  and commands containing null bytes
- Tests for injection patterns to verify shell doesn't process them dangerously
