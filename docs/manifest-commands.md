# Manifest Commands Reference

## Overview

The OpenClaw manifest (`catalog/openclaw.manifest.json`) defines runtime commands for the full agent lifecycle: install, upgrade, start, and stop.

## Platform-Specific Commands

Each command supports optional platform overrides via the `platforms` field:

```json
{
  "command": "default-command",
  "platforms": {
    "linux": "linux-specific-command",
    "darwin": "macos-specific-command",
    "windows": "windows-specific-command"
  }
}
```

Platform keys correspond to Go `runtime.GOOS` values. If no platform match is found, the top-level `command` is used as fallback.

## Command Details

### Install

The install command uses the official OpenClaw installer and downloads scripts to a tmpfile before execution:

```bash
# Linux/macOS:
tmp_file="$(mktemp -t openclaw-install.XXXXXX)" \
  && curl -fsSL --proto '=https' --tlsv1.2 https://openclaw.ai/install.sh -o "$tmp_file" \
  && bash "$tmp_file"; code=$?; rm -f "$tmp_file"; exit $code
```

### Start

```bash
openclaw --config ./config.yaml
```

On Windows (WSL2):
```bash
wsl.exe bash -lc 'openclaw --config ./config.yaml'
```

### Stop

```bash
openclaw --stop
```

On Windows (WSL2):
```bash
wsl.exe bash -lc 'openclaw --stop'
```

### Upgrade

Same pattern as install, using the official installer script via tmpfile execution.

## Constraints

- All commands are executed via `/bin/sh -lc` (or `wsl.exe bash -lc` on Windows).
- Commands must not be empty or contain null bytes.
- Manifest commands are considered trusted; no additional sandboxing is applied.

See also: [Security: Fallback Trust Model](./security-fallback-trust-model.md) for checksum verification details.
