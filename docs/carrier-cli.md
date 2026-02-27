# Carrier CLI Reference

This document reflects the current `carrier --help` command surface.

## Command Surface

### Bootstrap and runtime

- `carrier`
  - Bootstrap entrypoint.
  - If not onboarded, enters onboarding flow.
  - If onboarded, ensures daemon + gateway are running and exits.
- `carrier daemon`
  - Start daemon API server in foreground.
- `carrier gateway`
  - Start gateway API server in foreground.
- `carrier stop`
  - Stop background daemon and gateway services.
- `carrier version` / `carrier --version` / `carrier -v` / `carrier -V`
  - Print version metadata.
- `carrier update [options]`
  - Update local Carrier to target ref/channel.

### Onboarding

- `carrier onboard`
- `carrier onboard --tui`
- `carrier onboard --cli`
- `carrier onboard --webui`

Onboarding writes config to `${CARRIER_CONFIG:-~/.carrier/config.v2.json}`.

### Managed local instance lifecycle

- `carrier add <agent_id> [--tui|--cli|--webui] [-q|--quiet]`
- `carrier install <agent_id> [--tui|--cli|--webui] [-q|--quiet]`
  - Alias of `carrier add`.
- `carrier list`
- `carrier start <id|name>`
- `carrier stop <id|name>`
- `carrier status <id|name>`
- `carrier upgrade <id|name>`
- `carrier uninstall <id|name>`

Managed agent IDs currently include `openclaw`, `picoclaw`, `zeroclaw`.

### Remote instance operations

`carrier remote <action> <host_id> <agent_id> [options]`

Supported actions:

- `sync [--mode <always_push|pull_validate_push|manual>]`
- `sync-status`
- `diagnose`
- `reconcile`
- `rollback [--commit <hash>]`
- `codeagent-install [--backend <codex|opencode>] [--workspace-root <path>]`
- `codeagent-configure [--backend <codex|opencode>] [--workspace-root <path>] [--profile-json <json>]`
- `codeagent-health [--backend <codex|opencode>] [--workspace-root <path>]`
- `codeagent-version [--backend <codex|opencode>]`
- `codeagent-run --capability <...> [backend/workspace/options]`

Important:
- Remote OpenClaw installation itself is currently done by script: `scripts/remote-openclaw-install.sh`.
- `carrier remote` is currently focused on sync/diagnose/codeagent operations after host/instance is available.

## Recommended Workflows

### Local first-time setup

```bash
carrier
carrier onboard
carrier add openclaw
carrier status openclaw
```

WebUI variant:

```bash
carrier onboard --webui
carrier add openclaw --webui
```

### Install OpenClaw on VPS

```bash
scripts/remote-openclaw-install.sh \
  --host-id vps-1 \
  --host 203.0.113.10 \
  --port 22 \
  --user ubuntu \
  --key-path ~/.ssh/id_ed25519
```

With selected local config sync:

```bash
scripts/remote-openclaw-install.sh \
  --host-id vps-1 \
  --host 203.0.113.10 \
  --port 22 \
  --user ubuntu \
  --key-path ~/.ssh/id_ed25519 \
  --sync-channel telegram \
  --sync-provider openai-codex
```

## Behavior Notes

- Gateway default: `http://127.0.0.1:8787`
- Daemon default: `http://127.0.0.1:9090`
- Chat `/onboard` is intentionally blocked for credential safety.
- Chat `/install` is policy-gated; default policy requires explicit host binding and supports remote OpenClaw install (`/install openclaw <host_id>`).
- For newly discovered remote instances, config pull can require explicit confirmation.
