# Task-First Quickstart

This guide is optimized for execution, not architecture reading.

## Task 1: Install Carrier On Local Machine

Recommended:

```bash
curl -fsSL https://raw.githubusercontent.com/Keith-CY/carrier/main/scripts/install.sh | bash
carrier --version
```

Source build alternative:

```bash
git clone https://github.com/Keith-CY/carrier.git
cd carrier
go build -o ./carrier ./cmd/carrier
./carrier --version
```

## Task 2: Onboard Carrier

```bash
# bootstrap
carrier

# explicit onboarding
carrier onboard
```

WebUI flow:

```bash
carrier onboard --webui
```

Expected result:
- onboarding completes without error
- config exists at `${CARRIER_CONFIG:-~/.carrier/config.v2.json}`
- gateway health is reachable at `http://127.0.0.1:8787/healthz`

## Task 3: Install OpenClaw Locally

```bash
carrier add openclaw
carrier status openclaw
carrier list
```

Expected result:
- `openclaw` exists in managed instance list
- status is reachable and not in install-pending failure

## Task 4: Install OpenClaw To VPS

Prerequisites:
- SSH host/user/key ready
- local gateway running (`carrier` bootstrap can start it)
- local tools available: `curl`, `jq`

Minimal flow:

```bash
carrier remote add openclaw \
  --host-id vps-1 \
  --host 203.0.113.10 \
  --port 22 \
  --user ubuntu \
  --key-path ~/.ssh/id_ed25519
```

Selected local config sync flow:

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

Expected result:
- command finishes with `done: remote add flow succeeded ...`
- remote host check returns `openclawFound=true`
- instance list includes `<host-id>:main`

## Task 5: Validate Full Remote Agent Matrix (Optional)

```bash
scripts/remote-vps-agent-suite.sh \
  --host-id vps-1 \
  --host 127.0.0.1 \
  --port 2224 \
  --user carrier \
  --key-path /tmp/carrier-e2e-keys/id_ed25519
```

This runs deterministic validation for OpenClaw + PicoClaw + ZeroClaw + codeagent backends.

## Task 6: Common Failures

| Signal | Action |
|---|---|
| `gateway not healthy` | run `carrier` or `carrier gateway`, then check `http://127.0.0.1:8787/healthz` |
| SSH check failed | verify host/port/user/key and remote network access |
| missing local channel/provider during `--sync-*` | complete `carrier onboard` first or pass correct IDs |
| chat returns `E_ONBOARD_GUI_ONLY` | run onboarding via CLI/TUI/WebUI (`carrier onboard`, `carrier onboard --webui`) |
| chat `/install` returns `E_HOST_BINDING_REQUIRED` | provide host binding: `/install <agent_id> <host_id>` |

References:
- CLI command surface: [`docs/carrier-cli.md`](./carrier-cli.md)
- Pairing lifecycle runbook: [`docs/runbooks/pairing-lifecycle.md`](./runbooks/pairing-lifecycle.md)
