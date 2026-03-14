# Task-First Quickstart

This guide is optimized for execution, not architecture reading.
All commands below assume `carrier` is installed and available in `PATH`.

## Task 1: Install Carrier On Local Machine

Recommended:

```bash
curl -fsSL https://raw.githubusercontent.com/Keith-CY/carrier/main/scripts/install.sh | bash
carrier --version
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

## Task 3: Install Local Workers

```bash
carrier add zeroclaw
carrier add picoclaw
carrier status zeroclaw
carrier status picoclaw
```

Expected result:
- `zeroclaw` and `picoclaw` exist in the managed instance list
- both statuses are reachable and not in install-pending failure

## Task 4: Preview And Run An Orchestrated Task

Preview the execution plan first:

```bash
carrier orchestrate "triage this issue and summarize next actions" --dry-run
```

Run the task:

```bash
carrier orchestrate "triage this issue and summarize next actions"
```

Or launch a built-in template:

```bash
carrier templates
carrier templates show incident-diagnosis
carrier templates run incident-diagnosis \
  --input service=checkout \
  --input environment=prod \
  --input incidentSummary="Checkout API returns 502s after deploy"
```

Inspect execution history:

```bash
carrier executions
carrier executions show <execution_id>
carrier executions cancel <execution_id>
carrier executions artifacts <execution_id>
carrier executions evidence <execution_id>
carrier executions audit <execution_id>
carrier executions retry <execution_id>
```

Expected result:
- the base agent returns a non-empty task plan
- task units are assigned to local `picoclaw` / `zeroclaw`
- the final execution output includes task results, worker targets, and any attached artifacts

## Task 5: Inspect Memory And Attach A Scope

```bash
carrier memory list --subject agent-a
carrier memory search --subject agent-a --query "fusion"
carrier memory attach --instance picoclaw-main --scope shared:profile
carrier memory distill --instance picoclaw-main --dry-run --reason "promote learnings"
```

Expected result:
- the gateway returns a visible memory package list
- curated search results are returned for the subject
- the target instance accepts the requested memory scope
- distill returns a run id for base-agent promotion or later execution evidence

## Task 6: Install OpenClaw Locally (Optional)

```bash
carrier add openclaw
carrier status openclaw
carrier list
```

Expected result:
- `openclaw` exists in managed instance list
- status is reachable and not in install-pending failure

## Task 7: Install OpenClaw To VPS

Prerequisites:
- SSH host/user/key ready
- local gateway running (`carrier` bootstrap can start it)

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
  --sync-provider openai
```

Optional: install PicoClaw and ZeroClaw on the same host

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

Expected result:
- command finishes with `Completed: OpenClaw ...`
- post-install check shows `SSH connectivity: OK.`
- instance list includes `main`

## Task 7: Validate Full Remote Agent Matrix (Optional)

```bash
scripts/remote-vps-agent-suite.sh \
  --host-id vps-1 \
  --host 127.0.0.1 \
  --port 2224 \
  --user carrier \
  --key-path /tmp/carrier-e2e-keys/id_ed25519
```

This runs deterministic validation for OpenClaw + PicoClaw + ZeroClaw + codeagent backends.

## Task 8: Common Failures

| Signal | Action |
|---|---|
| `gateway not healthy` | run `carrier` or `carrier gateway`, then check `http://127.0.0.1:8787/healthz` |
| `local worker agent <id> is not installed` | run `carrier add zeroclaw` or `carrier add picoclaw` before `carrier orchestrate` |
| SSH check failed | verify host/port/user/key and remote network access |
| missing local channel/provider during `--sync-*` | complete `carrier onboard` first or pass correct IDs |
| chat returns `E_ONBOARD_GUI_ONLY` | run onboarding via CLI/TUI/WebUI (`carrier onboard`, `carrier onboard --webui`) |
| chat `/install` returns `E_HOST_BINDING_REQUIRED` | provide host binding: `/install <agent_id> <host_id>` |

References:
- CLI command surface: [`docs/carrier-cli.md`](./carrier-cli.md)
- Pairing lifecycle runbook: [`docs/runbooks/pairing-lifecycle.md`](./runbooks/pairing-lifecycle.md)
