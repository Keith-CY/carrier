# Carrier

Carrier is a local control plane for onboarding agent credentials, installing managed agents, and operating remote VPS instances over SSH.

## What Works Today

- Local bootstrap/onboarding via CLI/TUI/WebUI.
- Managed local install lifecycle for `openclaw`, `picoclaw`, `zeroclaw`.
- Remote host control-plane APIs (host upsert/check/install/list/config/sync).
- Deterministic remote install CLI workflow (`carrier remote add`, no LLM decision path).
- Remote instance discovery with confirmation-gated config pull for newly discovered instances.

## Core Docs

- CLI reference: [`docs/carrier-cli.md`](./docs/carrier-cli.md)
- Task-first guide: [`docs/task-first-quickstart.md`](./docs/task-first-quickstart.md)
- Remote sync API: [`docs/api/remote-sync-api.md`](./docs/api/remote-sync-api.md)
- Remote codeagent API: [`docs/api/remote-codeagent-api.md`](./docs/api/remote-codeagent-api.md)
- Architecture: [`ARCHITECTURE.md`](./ARCHITECTURE.md)

## Quick Start

### 1) Install Carrier

#### Option A: Install from release script (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/Keith-CY/carrier/main/scripts/install.sh | bash
carrier --version
```

Notes:
- Installer resolves `main` HEAD SHA, downloads `carrier-main-<full_sha>-<platform>.zip`, verifies `.sha256`, then installs `carrier`.
- Default install path is `/usr/local/bin/carrier` (falls back to `~/.local/bin/carrier` when needed).

#### Option B: Build from source

```bash
git clone https://github.com/Keith-CY/carrier.git
cd carrier
go build -o ./carrier ./cmd/carrier
./carrier --version
```

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

### 3) Install OpenClaw Locally

```bash
carrier add openclaw
carrier status openclaw
carrier list
```

`carrier install openclaw` is an alias of `carrier add openclaw`.

### 4) Install Agent To VPS (Deterministic)

Use `carrier remote add` to run a fixed, repeatable sequence:

1. upsert remote host
2. pre-check host
3. install agent via remote install stream
4. post-check host
5. list instances
6. reconnect simulation check (optional)

#### Prerequisites

- Local gateway reachable at `http://127.0.0.1:8787` (`carrier` bootstrap can start it).
- SSH access to VPS (`host`, `port`, `user`, private key path).
- Local tools: `curl`, `jq`.

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
  --sync-provider openai-codex
```

Optional allow-list flags:
- `--telegram-allow-from <id>`
- `--discord-allow-from <id>`

### 5) Full Remote Matrix Validation (Optional)

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
carrier daemon
carrier gateway

# managed instances
carrier list
carrier start <id|name>
carrier stop <id|name>
carrier status <id|name>
carrier upgrade <id|name>
carrier uninstall <id|name>

# install aliases
carrier add <agent_id>
carrier install <agent_id>
```

## Remote Control Notes

- `carrier remote add` is the canonical deterministic remote install command.
- Remote install currently supports `openclaw`, `picoclaw`, `zeroclaw`.
- On first host check, newly discovered remote instances may require explicit confirmation before pulling config to local repo.
- Chat `/install` can perform remote install when boundary policy allows it (default: requires host binding).

## Troubleshooting

- Gateway not reachable:
  - `carrier` (bootstrap) or `carrier gateway`
  - verify `http://127.0.0.1:8787/healthz`
- Remote host check failures:
  - verify SSH key path and remote login
  - retry host check from CLI (`--check-retries`, `--check-retry-delay`)
- OpenClaw runtime issues:
  - `carrier status openclaw`
  - rerun `carrier remote add openclaw ... --skip-reconnect-check`

## Development

- Contributor guide: [`CONTRIBUTING.md`](./CONTRIBUTING.md)
- Test entrypoint: `./scripts/run-all-tests.sh`
- Docs consistency check: `./scripts/check-doc-command-sync.sh`
