# Carrier

Carrier is a local-first agent control plane for decomposing goals, dispatching work to managed agents, and operating remote workers over SSH.

## What Works Today

- Task-first orchestration via `carrier orchestrate` with execution history in `carrier executions`.
- Local bootstrap/onboarding via CLI/TUI/WebUI.
- Managed local install lifecycle for `openclaw`, `picoclaw`, `zeroclaw`.
- Remote host control-plane APIs (host upsert/check/install/list/config/sync).
- Deterministic remote install CLI workflow (`carrier remote add`, no LLM decision path).
- Remote instance discovery with confirmation-gated config pull for newly discovered instances.

## Core Docs

- CLI reference: [`docs/carrier-cli.md`](./docs/carrier-cli.md)
- Task-first guide: [`docs/task-first-quickstart.md`](./docs/task-first-quickstart.md)
- Glossary: [`GLOSSARY.md`](./GLOSSARY.md)
- Remote sync API: [`docs/api/remote-sync-api.md`](./docs/api/remote-sync-api.md)
- Remote codeagent API: [`docs/api/remote-codeagent-api.md`](./docs/api/remote-codeagent-api.md)
- Architecture: [`ARCHITECTURE.md`](./ARCHITECTURE.md)

## Quick Start

All commands below assume `carrier` is already installed and available in `PATH`.
You do not need to run them from the Carrier repository directory.

### 1) Install Carrier

Install from release script (recommended):

```bash
curl -fsSL https://raw.githubusercontent.com/Keith-CY/carrier/main/scripts/install.sh | bash
carrier --version
```

Notes:
- Installer resolves `main` HEAD SHA, downloads `carrier-main-<full_sha>-<platform>.zip`, verifies `.sha256`, then installs `carrier`.
- GitHub Releases also publish App artifacts named `carrier-app-<tag>-<platform>.zip` for the Tauri app distribution.
- Default install path is `/usr/local/bin/carrier` (falls back to `~/.local/bin/carrier` when needed).

### App (Tauri Desktop)

The App distribution is a desktop shell that opens the same Carrier WebUI (`webui`) at:

- `http://127.0.0.1:8787/`

When you launch the App, it tries to auto-start:

- `carrier daemon`
- `carrier gateway`

After startup, you can use the full WebUI in the desktop window, including:

- onboarding and configuration
- remote host/agent lifecycle operations
- logs, diagnostics, and provider/profile management
- remote chat controls and remote instance controls

The App artifact naming is:

- CLI: `carrier-<tag>-<platform>.zip`
- App: `carrier-app-<tag>-<platform>.zip`

### 2) Onboard Carrier

```bash
# bootstrap: runs onboarding automatically if config is missing
carrier

# explicit onboarding entry
carrier onboard
```

WebUI onboarding:

```bash
carrier onboard --webui
```

Notes:
- Onboarding stores config at `${CARRIER_CONFIG:-~/.carrier/config.v2.json}`.
- Chat `/onboard` is intentionally blocked for credential safety.
- Chat `/install` is policy-gated; default policy requires explicit host binding (`/install <agent_id> <host_id>`).

### 3) Install Local Workers

```bash
carrier add zeroclaw
carrier add picoclaw
carrier status zeroclaw
carrier status picoclaw
```

Notes:
- `carrier install <agent_id>` is an alias of `carrier add <agent_id>`.
- `carrier orchestrate` currently expects the target local worker agents to already be installed.

### 4) Run A Task

Preview the plan first:

```bash
carrier orchestrate "triage this issue and summarize next actions" --dry-run
```

Run the task and wait for results:

```bash
carrier orchestrate "triage this issue and summarize next actions"
```

Inspect execution history:

```bash
carrier executions
carrier executions show <execution_id>
carrier executions cancel <execution_id>
```

Expected result:
- base agent decomposes the goal into task units
- tasks are assigned to local `picoclaw` / `zeroclaw` workers
- the final output shows execution status, task results, and worker targets

### 5) Install OpenClaw Locally (Optional)

```bash
carrier add openclaw
carrier status openclaw
carrier list
```

### 6) Install Agent To VPS (Deterministic)

Use `carrier remote add` to run a fixed, repeatable sequence:

1. upsert remote host
2. pre-check host
3. install agent via remote install stream
4. post-check host
5. list instances
6. reconnect simulation check (optional)

By default, failures after install trigger automatic rollback:
- existing instance: rollback config sync state
- fresh install: uninstall cleanup

Disable this behavior with `--no-auto-rollback`.

#### Prerequisites

- Local gateway reachable at `http://127.0.0.1:8787` (`carrier` bootstrap can start it).
- SSH access to VPS (`host`, `port`, `user`, private key path).

#### Minimal example

```bash
carrier remote add openclaw \
  --host-id vps-1 \
  --host 203.0.113.10 \
  --port 22 \
  --user ubuntu \
  --key-path ~/.ssh/id_ed25519
```

#### Install with selected local config sync

If you already onboarded locally and want to push selected local channel/provider settings to remote config:

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

Optional allow-list flags:
- `--telegram-allow-from <id>`
- `--discord-allow-from <id>`

Install other supported agents on VPS:

```bash
carrier remote add picoclaw \
  --host-id vps-1 \
  --host 203.0.113.10 \
  --port 22 \
  --user ubuntu \
  --key-path ~/.ssh/id_ed25519
```

```bash
carrier remote add zeroclaw \
  --host-id vps-1 \
  --host 203.0.113.10 \
  --port 22 \
  --user ubuntu \
  --key-path ~/.ssh/id_ed25519
```

### 7) Full Remote Matrix Validation (Optional)

For Docker-VPS style end-to-end validation across OpenClaw + PicoClaw + ZeroClaw + codeagent backends:

```bash
scripts/remote-vps-agent-suite.sh \
  --host-id vps-1 \
  --host 127.0.0.1 \
  --port 2224 \
  --user carrier \
  --key-path /tmp/carrier-e2e-keys/id_ed25519
```

## Daily Commands

```bash
# services
carrier
carrier stop
carrier reset
carrier daemon
carrier gateway

# managed instances
carrier list
carrier start <id|name>
carrier stop <id|name>
carrier status <id|name>
carrier upgrade <id|name>
carrier uninstall <id|name>

# orchestration
carrier orchestrate "<goal>"
carrier orchestrate "<goal>" --dry-run
carrier executions
carrier executions show <execution_id>

# install aliases
carrier add <agent_id>
carrier install <agent_id>

# catalog management
carrier catalog add <manifest-url>
carrier catalog list
carrier catalog remove <agent_id>

# configuration & diagnostics
carrier config set <key> <value>
carrier doctor

# key management
carrier keys list
carrier keys generate [--type <ed25519|rsa>]
carrier keys delete <key-ref>

# logs & monitoring
carrier logs <agent_id> [--tail <n>]
carrier service <start|stop|status>

# webhooks
carrier webhooks test <url>
```

## Remote Control Notes

- `carrier remote add` is the canonical deterministic remote install command.
- Remote install currently supports `openclaw`, `picoclaw`, `zeroclaw`.
- On first host check, newly discovered remote instances may require explicit confirmation before pulling config to local repo.
- Remote troubleshooting commands are available:
  - `carrier remote status <host_id> <agent_id>`
  - `carrier remote logs <host_id> <agent_id> [--tail <n>]`
  - `carrier remote rollback <host_id> <agent_id> [--commit <sha>]`
  - `carrier remote uninstall <host_id> <agent_id>`
- Remote SSH key management commands are available:
  - `carrier remote key import --file <pem-path>`
  - `carrier remote key generate [--type <ed25519|rsa>] [--output <private-key-path>]`
- Backup/restore commands are available:
  - `carrier config backup|restore`
  - `carrier remote-store backup|restore`
- Optional remote alert webhook watchdog:
  - set `CARRIER_REMOTE_ALERT_WEBHOOK_URL`
  - optional tuning: `CARRIER_REMOTE_ALERT_INTERVAL_SEC`, `CARRIER_REMOTE_ALERT_COOLDOWN_SEC`
- Chat `/install` can perform remote install when boundary policy allows it (default: requires host binding).

## Troubleshooting

- Gateway not reachable:
  - `carrier` (bootstrap) or `carrier gateway`
  - verify `http://127.0.0.1:8787/healthz`
- Remote host check failures:
  - verify SSH key path and remote login
  - check platform support in check summary (`Linux` required, `alpine` currently rejected for deterministic remote install)
  - retry host check from CLI (`--check-retries`, `--check-retry-delay`)
- OpenClaw runtime issues:
  - `carrier status openclaw`
  - `carrier remote logs <host_id> main --tail 200`
  - `carrier remote rollback <host_id> main`
  - rerun `carrier remote add openclaw ... --skip-reconnect-check`

## Development

- Contributor guide: [`CONTRIBUTING.md`](./CONTRIBUTING.md)
- Test entrypoint: `./scripts/run-all-tests.sh`
- Docs consistency check: `./scripts/check-doc-command-sync.sh`
