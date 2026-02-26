# Daemon Lifecycle Runtime and Troubleshooting

This guide documents the current lifecycle runtime behavior implemented in `daemon/internal/lifecycle/`.

## Start/Stop runtime semantics

- `Install`/`Start`/`Stop`/`Upgrade` execute manifest commands via the lifecycle runner.
- `Start` requires:
  - agent is installed,
  - required environment variables are present,
  - declared ports are available,
  - agent is not in crash-loop cooldown.
- `Stop` executes the manifest stop command and marks runtime as `stopped` on success.
- Graceful-stop timeout / force-kill fallback are delegated to the runtime command implementation (or external supervisor such as systemd); lifecycle service itself does not apply an extra kill timeout layer.
- If a start command fails, runtime transitions to `crashing` and health to `unhealthy`.

## Crash-loop detection and cooldown

Default crash-loop policy:
- threshold: `3` restarts
- window: `5m`
- cooldown: `5m`

Config/override keys:
- `CARRIER_CRASH_THRESHOLD`
- `CARRIER_CRASH_WINDOW`
- `CARRIER_CRASH_COOLDOWN`

State transitions:
- repeated start failures within the window move runtime to `crashing`,
- start attempts during cooldown return `ErrCrashLoop`,
- after cooldown expires, start is allowed again.

## Log location and diagnose artifacts

Current implementation keeps per-agent command logs in an in-memory ring buffer (default limit: `1000` entries).

Persistent log/artifact output is written when diagnose is requested:
- default `diagnose_dir`: `<os temp dir>/carrier-daemon-diagnose` (if not overridden by `WithDiagnoseDir`)
- artifact path pattern: `<diagnose_dir>/<agent>-diagnose-<UTC timestamp>.zip`
- zip entries include:
  - `logs.txt`
  - `state.json`
  - `manifest.json`
  - `env.json` (redacted)
  - `metadata.json`

In the production deployment example, `diagnose_dir` is `/var/lib/carrier/diagnose`, so a typical file path is:
- `/var/lib/carrier/diagnose/openclaw-diagnose-2026-02-14T04-20-00Z.zip`

## Unexpected-exit troubleshooting

If you see "process exited unexpectedly" or an agent exits immediately after start:

1. Check status/runtime state:
   - `/status <agent_id>`
2. Fetch recent command logs:
   - `/logs <agent_id> 200`
3. Generate a redacted diagnose bundle:
   - `/diagnose <agent_id>`
4. Verify runtime prerequisites and required env vars for that manifest.
5. If runtime is `crashing`, wait for cooldown expiry before retrying start.

## Persistent lifecycle state and safe reset

Lifecycle state fields (`install/runtime/health/last_error/updated_at`) are always available in memory and exported through diagnose/backup artifacts.

Operational snapshot convention:
- keep the latest exported state snapshot at `/var/lib/carrier/state.json` for operator-driven recovery workflows.

Missing/corrupt snapshot handling:
- treat snapshot as recoverable metadata,
- rebuild runtime view from manifest registration + fresh status checks,
- regenerate a new snapshot after service is stable.

Safe local reset procedure:

```bash
sudo systemctl stop carrier-daemon
sudo rm -f /var/lib/carrier/state.json   # NOTE: this discards pending diagnose-consent records
sudo systemctl start carrier-daemon
curl -s http://127.0.0.1:8081/healthz
```

Use the reset flow only for local troubleshooting or planned maintenance windows.
