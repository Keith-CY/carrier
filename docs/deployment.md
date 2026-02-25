# Production Deployment Guide

This guide covers deploying the Carrier daemon (`carrier`) and gateway in a production environment.

Related runbooks:

- Go-live + rollback: `docs/runbooks/go-live-rollback.md`
- Pairing lifecycle troubleshooting: `docs/runbooks/pairing-lifecycle.md`
- CI first response: `docs/ci/first-response-playbook.md`

For lifecycle state transitions, crash-loop behavior, and operator troubleshooting, see `docs/daemon-lifecycle-runtime.md`.

## Prerequisites

- **Go 1.22+** (build from source) or a pre-built binary
- **Linux** (amd64 or arm64) — the daemon uses `/proc` for port-occupant detection
- **systemd** (recommended for service management)
- A dedicated non-root user (e.g., `carrier`)

## Build

```bash
go build -o carrier ./cmd/carrier
```

## Gateway

The gateway has been rewritten in Go and is now part of the daemon binary.
No separate build step is required — the daemon serves the gateway API directly.

## Configuration

The daemon reads configuration from a JSON file. Create `/etc/carrier/carrier.json`:

```json
{
  "server": {
    "host": "127.0.0.1",
    "port": 9090,
    "api_token": ""
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

> **Note:** If `api_token` is set, the config file must have restrictive permissions (`chmod 0600`).
> See `shared/config/config.go` for all fields and environment variable overrides (`CARRIER_` prefix).

Ensure the data and diagnose directories exist:

```bash
sudo mkdir -p /var/lib/carrier/{data,diagnose}
sudo chown carrier:carrier /var/lib/carrier/{data,diagnose}
```

### Environment Variables

Agent manifests may declare required environment variables. Set them in the systemd unit or a dedicated env file:

```bash
# /etc/carrier/carrier.env
# Example — adjust per your agent manifests
MY_AGENT_API_KEY=changeme
```

## Installation

```bash
sudo install -o root -g root -m 0755 carrier /usr/local/bin/carrier
sudo install -o root -g root -m 0755 carrier-gateway /usr/local/bin/carrier-gateway
```

## systemd Service

### Daemon (`carrier`)

Create `/etc/systemd/system/carrier-daemon.service`:

```ini
[Unit]
Description=Carrier Agent Daemon
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=carrier
Group=carrier
ExecStart=/usr/local/bin/carrier --config /etc/carrier/carrier.json
EnvironmentFile=-/etc/carrier/carrier.env
Restart=on-failure
RestartSec=5s
LimitNOFILE=65536

# Hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/carrier

[Install]
WantedBy=multi-user.target
```

### Gateway

Create `/etc/systemd/system/carrier-gateway.service`:

```ini
[Unit]
Description=Carrier Gateway
After=network-online.target carrier-daemon.service
Wants=network-online.target

[Service]
Type=simple
User=carrier
Group=carrier
ExecStart=/usr/local/bin/carrier-gateway
Restart=on-failure
RestartSec=5s
LimitNOFILE=65536

NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/carrier

[Install]
WantedBy=multi-user.target
```

### Enable and Start

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now carrier-daemon carrier-gateway
```

## Health Monitoring

The daemon exposes a health endpoint (default port 8081):

```bash
curl -s http://localhost:8081/healthz
```

Integrate with your monitoring stack (Prometheus, Datadog, etc.):

- **Liveness**: `GET /healthz` — returns 200 when the daemon process is alive
- **Readiness**: check that agents report `healthy` via the status API

### Alerting Recommendations

| Condition | Alert |
|-----------|-------|
| `/healthz` returns non-200 for >30s | Critical |
| Agent in `crashing` state for >5m | Warning |
| Audit buffer >80% full | Warning |
| Disk usage on data dir >90% | Critical |

## Logging

With structured logging enabled (`log_format: "json"`), logs are written to stdout and captured by journald:

```bash
journalctl -u carrier-daemon -f --output=json
```

Ship logs to your aggregation platform (ELK, Loki, etc.) via journald or a sidecar.

## Backup

### What to Back Up

- `/etc/carrier/` — configuration files and env
- `/var/lib/carrier/data/` — agent state, manifests, memory store
- `/var/lib/carrier/diagnose/` — diagnostic bundles and upgrade backups

### Backup Strategy

```bash
# Daily backup example (add to cron)
tar czf /backup/carrier-$(date +%Y%m%d).tar.gz \
  /etc/carrier/ \
  /var/lib/carrier/data/
```

### Restore

1. Stop services: `sudo systemctl stop carrier-daemon carrier-gateway`
2. Restore files from backup
3. Start services: `sudo systemctl start carrier-daemon carrier-gateway`
4. Verify health: `curl http://localhost:8081/healthz`

## Upgrade Procedure

1. Build or download the new binary
2. Stop the service: `sudo systemctl stop carrier-daemon`
3. Replace the binary: `sudo install -o root -g root -m 0755 carrier /usr/local/bin/carrier`
4. Start the service: `sudo systemctl start carrier-daemon`
5. Verify: `curl http://localhost:8081/healthz`

For agent upgrades (managed via the lifecycle API), the daemon automatically creates pre-upgrade backups in the diagnose directory.

## Security Considerations

- Run as a non-root user with minimal privileges
- Use `ProtectSystem=strict` and `NoNewPrivileges=true` in systemd
- Rotate secrets in `/etc/carrier/carrier.env` regularly
- The gateway uses crypto-secure tokens — do not downgrade to sequential generation
- Review the [security audit](../docs/security-audit-command-execution.md) for command execution hardening
