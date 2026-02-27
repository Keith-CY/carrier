# Carrier CLI Reference

This document reflects the current `carrier --help` command surface.
All command blocks below assume `carrier` is installed and available in `PATH`.

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

### Remote deterministic install

`carrier remote add <agent_id> --host-id <id> --host <ip-or-domain> --port <port> --user <ssh-user> --key-path <private-key-path> [options]`

Supported `agent_id` values:

- `openclaw`
- `picoclaw`
- `zeroclaw`

Options:

- `--name <display-name>`
- `--runtime-mode <on_demand|managed_gateway>`
- `--sync-channel <telegram|discord|feishu>` (repeatable)
- `--sync-provider <provider-id>` (repeatable)
- `--telegram-allow-from <id>` (repeatable)
- `--discord-allow-from <id>` (repeatable)
- `--check-retries <n>`
- `--check-retry-delay <seconds>`
- `--skip-reconnect-check`

Behavior:

- Runs a fixed sequence: upsert host -> pre-check -> install(stream) -> optional local config sync -> post-check -> list -> optional reconnect check.
- Newly discovered remote instances require explicit confirmation before pulling their configs to local profile store.

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

### Install local managed agents

```bash
carrier add openclaw
```

```bash
carrier add picoclaw
```

```bash
carrier add zeroclaw
```

### Install OpenClaw on VPS

```bash
carrier remote add openclaw \
  --host-id vps-1 \
  --host 203.0.113.10 \
  --port 22 \
  --user ubuntu \
  --key-path ~/.ssh/id_ed25519
```

With selected local config sync:

```bash
carrier remote add openclaw \
  --host-id vps-1 \
  --host 203.0.113.10 \
  --port 22 \
  --user ubuntu \
  --key-path ~/.ssh/id_ed25519 \
  --sync-channel telegram \
  --sync-provider openai
```

### Install PicoClaw on VPS

```bash
carrier remote add picoclaw \
  --host-id vps-1 \
  --host 203.0.113.10 \
  --port 22 \
  --user ubuntu \
  --key-path ~/.ssh/id_ed25519
```

### Install ZeroClaw on VPS

```bash
carrier remote add zeroclaw \
  --host-id vps-1 \
  --host 203.0.113.10 \
  --port 22 \
  --user ubuntu \
  --key-path ~/.ssh/id_ed25519
```

### Skip reconnect verification (faster install path)

```bash
carrier remote add openclaw \
  --host-id vps-1 \
  --host 203.0.113.10 \
  --port 22 \
  --user ubuntu \
  --key-path ~/.ssh/id_ed25519 \
  --skip-reconnect-check
```

### Daily lifecycle commands

```bash
carrier list
```

```bash
carrier status <id|name>
```

```bash
carrier start <id|name>
```

```bash
carrier stop <id|name>
```

```bash
carrier upgrade <id|name>
```

```bash
carrier uninstall <id|name>
```

```bash
carrier stop
```

```bash
carrier reset
```

## Behavior Notes

- Gateway default: `http://127.0.0.1:8787`
- Daemon default: `http://127.0.0.1:9090`
- Chat `/onboard` is intentionally blocked for credential safety.
- Chat `/install` is policy-gated; default policy requires explicit host binding (`/install <agent_id> <host_id>`).
- For newly discovered remote instances, config pull can require explicit confirmation.
