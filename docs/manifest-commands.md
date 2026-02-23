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

The install command uses npm as the primary installation method with a binary fallback:

```bash
# Primary (all platforms):
npm install -g openclaw

# Fallback (Linux/macOS only):
curl -fsSL https://raw.githubusercontent.com/Keith-CY/carrier/main/scripts/install.sh | bash
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

Same pattern as install, using `npm update -g` with binary fallback.

## Constraints

- All commands are executed via `/bin/sh -lc` (or `wsl.exe bash -lc` on Windows).
- Commands must not be empty or contain null bytes.
- Manifest commands are considered trusted; no additional sandboxing is applied.

See also: [Security: Fallback Trust Model](./security-fallback-trust-model.md) for checksum verification details.
