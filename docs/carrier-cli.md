# Carrier CLI Reference

This document reflects the current `carrier --help` command surface.
All command blocks below assume `carrier` is installed and available in `PATH`.

## Command Surface

### Orchestration and execution history

- `carrier orchestrate <goal...> [--host-id <id>]... [--host-label <label>]... [--provider <provider-id>] [--max-concurrency <n>] [--idempotency-key <key>] [--timeout <duration>] [--async] [--dry-run] [--json]`
  - Decompose a goal with the base agent, assign task units to worker agents, and optionally execute the plan.
  - `--dry-run` previews the execution plan without creating an execution.
- `carrier templates [list] [--json]`
  - List built-in execution templates and their required inputs.
- `carrier templates show <template_id> [--json]`
  - Show one built-in execution template with task metadata and input schema.
- `carrier templates run <template_id> --input key=value [--input key=value]... [--json]`
  - Launch a built-in execution template, create the execution record, and authorize it immediately.
- `carrier orchestrate status <execution_id> [--json]`
  - Show one orchestration execution with task results and worker lease state.
- `carrier orchestrate cancel <execution_id> [--json]`
  - Cancel one orchestration execution.
- `carrier executions [list] [--limit <n>] [--json]`
  - List orchestration executions from the remote-control store.
- `carrier executions show <execution_id> [--json]`
  - Alias for orchestration execution status lookup.
- `carrier executions cancel <execution_id> [--json]`
  - Cancel one orchestration execution.
- `carrier executions retry <execution_id> [--json]`
  - Create a derived execution containing only failed tasks from the source execution.
- `carrier executions rerun <execution_id> [--json]`
  - Create a new execution from the full original plan.
- `carrier executions clone <execution_id> [--json]`
  - Clone the original execution plan into a fresh `pending_authorization` execution.
- `carrier executions artifacts <execution_id> [--json]`
  - List execution artifact metadata.
- `carrier executions evidence <execution_id> [--format json|zip] [--output <path>] [--open] [--json]`
  - Export an execution evidence bundle as JSON or download a ZIP archive.
- `carrier executions audit <execution_id> [--output <path>] [--open] [--json]`
  - Export execution-scoped gateway audit events as JSON.
- `carrier triggers [list] [--json]`
  - List execution triggers.
- `carrier triggers show <trigger_id> [--json]`
  - Show one execution trigger.
- `carrier triggers create --type <webhook|github|schedule> --template-id <template_id> [--name <name>] [--host-id <id>]... [--host-label <label>]... [--provider <provider-id>] [--max-concurrency <n>] [--policy-approve] [--webhook-secret <secret>] [--github-command <cmd>] [--github-label <label>] [--github-repository <owner/repo>] [--cron <expr>] [--timezone UTC] [--input key=value]... [--json]`
  - Create one execution trigger.
- `carrier triggers update <trigger_id> [--name <name>] [--template-id <template_id>] [--enable|--disable] [--host-id <id>]... [--host-label <label>]... [--provider <provider-id>] [--max-concurrency <n>] [--policy-approve] [--webhook-secret <secret>] [--github-command <cmd>] [--github-label <label>] [--github-repository <owner/repo>] [--cron <expr>] [--timezone UTC] [--input key=value]... [--json]`
  - Update one execution trigger.
- `carrier triggers delete <trigger_id> [--json]`
  - Delete one execution trigger.

### Knowledge plane

- `carrier memory [list] [--subject <subject>] [--json]`
  - List memory packages, attachments, grants, and audit state through the gateway memory facade.
- `carrier memory search --subject <subject> --query <query> [--limit <n>] [--min-score <f>] [--json]`
  - Search curated memory records for one subject.
- `carrier memory attach --instance <id> --scope <scope> [--json]`
  - Attach one memory scope to an instance before execution.
- `carrier memory detach --instance <id> --scope <scope> [--json]`
  - Detach one memory scope from an instance.
- `carrier memory distill --instance <id> [--scope <scope>] [--dry-run] [--force] [--reason <text>] [--json]`
  - Distill instance learnings back into the base knowledge plane.

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
- `carrier reset`
  - Stop services and remove Carrier-generated local data.
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

- `carrier add <agent_id> [--isolation] [--tui|--cli|--webui] [-q|--quiet]`
- `carrier install <agent_id> [--isolation] [--tui|--cli|--webui] [-q|--quiet]`
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
- `--auth-mode <private_key|ssh_config>`
- `--ssh-config-host <alias>`
- `--key-ref <uploaded-key-ref>`
- `--runtime-mode <on_demand|managed_gateway>`
- `--isolation`
- `--sync-channel <telegram|discord|feishu>` (repeatable)
- `--sync-provider <provider-id>` (repeatable)
- `--telegram-allow-from <id>` (repeatable)
- `--discord-allow-from <id>` (repeatable)
- `--check-retries <n>`
- `--check-retry-delay <seconds>`
- `--no-auto-rollback`
- `--skip-reconnect-check`

Behavior:

- Runs a fixed sequence: upsert host -> pre-check -> install(stream) -> optional local config sync -> post-check -> list -> optional reconnect check.
- From install step onward, failures trigger automatic rollback by default:
  - existing instance: rollback config sync state
  - fresh install: uninstall cleanup
- `--no-auto-rollback` disables automatic rollback.
- Newly discovered remote instances require explicit confirmation before pulling their configs to local profile store.

Additional remote operations:

- `carrier remote status <host_id> <agent_id>`
- `carrier remote logs <host_id> <agent_id> [--tail <n>]`
- `carrier remote rollback <host_id> <agent_id> [--commit <sha>]`
- `carrier remote uninstall <host_id> <agent_id>`
- `carrier remote key import --file <pem-path>`
- `carrier remote key generate [--type <ed25519|rsa>] [--output <private-key-path>]`

Remote preflight notes:

- Host check now includes platform detection.
- Deterministic remote install currently requires Linux.
- Alpine is rejected at preflight with an explicit unsupported-platform error.

Config/remote store backup and restore:

- `carrier config backup [--output <path>]`
- `carrier config restore --from <path>`
- `carrier remote-store backup [--output <path>]`
- `carrier remote-store restore --from <path>`

Optional remote alert webhook watchdog:

- `CARRIER_REMOTE_ALERT_WEBHOOK_URL`: enable webhook alert delivery for remote rollout alerts.
- `CARRIER_REMOTE_ALERT_INTERVAL_SEC`: polling interval for evaluating alert state (default `30`).
- `CARRIER_REMOTE_ALERT_COOLDOWN_SEC`: resend active alert after cooldown (default `300`).

## Recommended Workflows

### Local first-time setup

```bash
carrier
carrier onboard
carrier add zeroclaw
carrier add picoclaw
carrier orchestrate "triage this issue and summarize next actions" --dry-run
carrier orchestrate "triage this issue and summarize next actions"
```

WebUI variant:

```bash
carrier onboard --webui
carrier add zeroclaw --webui
```

WebUI variant with isolation preselected:

```bash
carrier add zeroclaw --webui --isolation
```

### Install local managed agents

```bash
carrier add zeroclaw
```

```bash
carrier add zeroclaw --isolation
```

```bash
carrier add picoclaw
```

```bash
carrier add openclaw
```

### Preview and run orchestration

```bash
carrier orchestrate "triage this issue and summarize next actions" --dry-run
```

```bash
carrier orchestrate "triage this issue and summarize next actions"
```

### Browse and launch templates

```bash
carrier templates
```

```bash
carrier templates show incident-diagnosis
```

```bash
carrier templates run incident-diagnosis \
  --input service=checkout \
  --input environment=prod \
  --input incidentSummary="Checkout API returns 502s after deploy"
```

```bash
carrier executions
```

```bash
carrier executions show <execution_id>
```

```bash
carrier executions artifacts <execution_id>
```

```bash
carrier executions evidence <execution_id>
```

```bash
carrier executions evidence <execution_id> --format zip --output evidence.zip
```

```bash
carrier executions audit <execution_id> --output audit.json
```

```bash
carrier executions evidence <execution_id> --format zip --open
```

```bash
carrier executions audit <execution_id> --open
```

```bash
carrier executions retry <execution_id>
```

```bash
carrier executions rerun <execution_id>
```

```bash
carrier executions clone <execution_id>
```

### Manage triggers

```bash
carrier triggers
```

```bash
carrier triggers show <trigger_id>
```

```bash
carrier triggers create --type webhook --template-id incident-diagnosis --name incident-webhook --webhook-secret secret
```

```bash
carrier triggers update <trigger_id> --enable
```

```bash
carrier triggers delete <trigger_id>
```

### Inspect memory and distill learnings

```bash
carrier memory list --subject agent-a
```

```bash
carrier memory search --subject agent-a --query "fusion"
```

```bash
carrier memory attach --instance picoclaw-main --scope shared:profile
```

```bash
carrier memory distill --instance picoclaw-main --dry-run --reason "promote learnings"
```

### Install OpenClaw on VPS

```bash
carrier remote add openclaw \
  --host-id vps-1 \
  --host 203.0.113.10 \
  --port 22 \
  --user ubuntu \
  --key-path ~/.ssh/id_ed25519 \
  --isolation
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
carrier orchestrate "<goal>"
```

```bash
carrier orchestrate "<goal>" --dry-run
```

```bash
carrier templates
```

```bash
carrier templates show <template_id>
```

```bash
carrier templates run <template_id> --input key=value
```

```bash
carrier executions
```

```bash
carrier executions show <execution_id>
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
- `carrier orchestrate` requires the target local worker agents to already be installed (`picoclaw`, `zeroclaw`).
- Chat `/onboard` is intentionally blocked for credential safety.
- Chat `/install` is policy-gated; default policy requires explicit host binding (`/install <agent_id> <host_id>`).
- For newly discovered remote instances, config pull can require explicit confirmation.
- `--isolation` is explicit opt-in and instance-scoped; if isolation backend is unavailable, command flow fails instead of silently falling back.
- In `--webui` mode, `carrier add ... --webui --isolation` opens WebUI with isolation preselected; deployment/start runs in WebUI flow.

## Additional Commands

- `carrier catalog add <manifest-url>` — Add an agent from a manifest URL
- `carrier catalog list` — List registered agent catalogs
- `carrier catalog remove <agent_id>` — Remove agent from catalog
- `carrier config set <key> <value>` — Set a configuration value
- `carrier doctor` — Run diagnostic checks
- `carrier keys list` — List managed SSH keys
- `carrier keys generate` — Generate a new SSH keypair
- `carrier keys delete <key-ref>` — Delete a managed SSH key
- `carrier logs <agent_id>` — View agent logs
- `carrier service` — Manage the carrier service
- `carrier webhooks test <url>` — Test a webhook endpoint
