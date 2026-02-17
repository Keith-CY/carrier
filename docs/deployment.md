# Production Deployment Guide

This guide covers deploying Carrier daemon (`agentd`) and gateway on Linux with systemd.

Related runbooks:
- Go-live + rollback: `./runbooks/go-live-rollback.md`
- Pairing lifecycle troubleshooting: `./runbooks/pairing-lifecycle.md`
- CI first response: `./ci/first-response-playbook.md`

For lifecycle behavior and operator troubleshooting, see `./daemon-lifecycle-runtime.md`.

## Prerequisites

- Linux host (systemd available)
- Go toolchain (`go version` should satisfy `daemon/go.mod`)
- Bun runtime (for gateway service)
- Dedicated non-root user, for example `carrier`

## Build Artifacts

Build daemon binary from repository root:

```bash
cd daemon
go build -o ../bin/agentd ./cmd/agentd
```

Gateway is a Bun runtime service (TypeScript source), not a Go binary. Install dependencies:

```bash
cd ../gateway
bun install --frozen-lockfile --no-progress
```

## Filesystem Layout (Recommended)

```text
/usr/local/bin/agentd
/opt/carrier/gateway/...
/etc/carrier/config.json
/etc/carrier/agentd.env
/etc/carrier/gateway.env
/var/lib/carrier/...
```

Prepare directories and ownership:

```bash
sudo mkdir -p /etc/carrier /opt/carrier /var/lib/carrier
sudo chown -R carrier:carrier /opt/carrier /var/lib/carrier
```

Install daemon binary:

```bash
sudo install -o root -g root -m 0755 bin/agentd /usr/local/bin/agentd
```

Sync gateway source into `/opt/carrier/gateway` (for example by checkout, rsync, or release unpack).

## Configuration

`agentd` loads `config.json` from its working directory.
Use `/etc/carrier/config.json` and run daemon service with `WorkingDirectory=/etc/carrier`.

Example `/etc/carrier/config.json`:

```json
{
  "server": {
    "host": "127.0.0.1",
    "port": 9090
  },
  "log": {
    "level": "info",
    "format": "json"
  },
  "lifecycle": {
    "crash_threshold": 3,
    "crash_window": "5m",
    "crash_cooldown": "5m"
  }
}
```

Daemon env overrides (`/etc/carrier/agentd.env`) are optional:

```bash
# Bind/API auth
CARRIER_SERVER_HOST=127.0.0.1
CARRIER_SERVER_PORT=9090
# Required if binding non-loopback host
# CARRIER_SERVER_API_TOKEN=replace-me

# Logging
CARRIER_LOG_LEVEL=info
CARRIER_LOG_FORMAT=json
```

Gateway env file (`/etc/carrier/gateway.env`) example:

```bash
CARRIER_DAEMON_BASE_URL=http://127.0.0.1:9090
CARRIER_DAEMON_TIMEOUT_MS=30000
CARRIER_GATEWAY_HOST=127.0.0.1
CARRIER_GATEWAY_PORT=8787
# Required when binding gateway to non-loopback host
# CARRIER_GATEWAY_API_TOKEN=replace-me
CARRIER_MAX_COMMAND_BODY_BYTES=65536

# Persist session/download artifacts outside source tree
SESSION_DATA_DIR=/var/lib/carrier
ARTIFACT_ROOT=/var/lib/carrier
```

## systemd Services

### Daemon (`carrier-agentd.service`)

Create `/etc/systemd/system/carrier-agentd.service`:

```ini
[Unit]
Description=Carrier Agent Daemon
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=carrier
Group=carrier
WorkingDirectory=/etc/carrier
EnvironmentFile=-/etc/carrier/agentd.env
ExecStart=/usr/local/bin/agentd
Restart=on-failure
RestartSec=5s
LimitNOFILE=65536

NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/carrier /etc/carrier

[Install]
WantedBy=multi-user.target
```

### Gateway (`carrier-gateway.service`)

Create `/etc/systemd/system/carrier-gateway.service`:

```ini
[Unit]
Description=Carrier Gateway (Bun)
After=network-online.target carrier-agentd.service
Wants=network-online.target

[Service]
Type=simple
User=carrier
Group=carrier
WorkingDirectory=/opt/carrier/gateway
EnvironmentFile=-/etc/carrier/gateway.env
ExecStart=/usr/bin/env bun run src/server.ts
Restart=on-failure
RestartSec=5s
LimitNOFILE=65536

NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/carrier /opt/carrier/gateway

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now carrier-agentd carrier-gateway
```

## Health Checks

Daemon:

```bash
curl -s http://127.0.0.1:9090/healthz
curl -s http://127.0.0.1:9090/readyz
```

Gateway:

```bash
curl -s http://127.0.0.1:8787/healthz
```

## Logging

Tail service logs with journald:

```bash
journalctl -u carrier-agentd -f
journalctl -u carrier-gateway -f
```

## Backup and Restore

Back up:
- `/etc/carrier/` (config + env)
- `/var/lib/carrier/` (sessions/artifacts and runtime data)

Example:

```bash
tar czf /backup/carrier-$(date +%Y%m%d).tar.gz /etc/carrier /var/lib/carrier
```

Restore flow:
1. `sudo systemctl stop carrier-agentd carrier-gateway`
2. Restore files from backup
3. `sudo systemctl start carrier-agentd carrier-gateway`
4. Re-run health checks above

## Upgrade Procedure

1. Deploy new daemon binary to `/usr/local/bin/agentd`
2. Deploy updated gateway source to `/opt/carrier/gateway`
3. Reinstall gateway deps if lockfile changed:
   - `cd /opt/carrier/gateway && bun install --frozen-lockfile --no-progress`
   - If this fails due to lockfile drift, regenerate and commit `gateway/bun.lock` from source before release.
4. Restart services:
   - `sudo systemctl restart carrier-agentd carrier-gateway`
5. Verify health and smoke-test `/pair` + `/agents`

## Security Notes

- Run services as non-root user.
- If daemon binds non-loopback, set `CARRIER_SERVER_API_TOKEN`.
- If gateway binds non-loopback, set `CARRIER_GATEWAY_API_TOKEN`.
- Rotate provider secrets and API tokens periodically.
- Review command-execution hardening details in `./security-audit-command-execution.md`.
