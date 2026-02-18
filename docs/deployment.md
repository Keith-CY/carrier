# Production Deployment Guide

This guide covers deploying the Carrier daemon (`agentd`) and gateway in a production environment.

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
cd daemon
go build -o agentd ./cmd/agentd

cd ../gateway
go build -o carrier-gateway .
```

## Configuration

The daemon reads configuration from a JSON file. Create `/etc/carrier/agentd.json`:

```json
{
  "log_level": "info",
  "log_format": "json",
  "health_port": 8081,
  "diagnose_dir": "/var/lib/carrier/diagnose",
  "data_dir": "/var/lib/carrier/data"
}
```

Ensure the data and diagnose directories exist:

```bash
sudo mkdir -p /var/lib/carrier/{data,diagnose}
sudo chown carrier:carrier /var/lib/carrier/{data,diagnose}
```

### Environment Variables

Agent manifests may declare required environment variables. Set them in the systemd unit or a dedicated env file:

```bash
# /etc/carrier/agentd.env
# Example — adjust per your agent manifests
MY_AGENT_API_KEY=changeme
```

## Installation

```bash
sudo install -o root -g root -m 0755 agentd /usr/local/bin/agentd
sudo install -o root -g root -m 0755 carrier-gateway /usr/local/bin/carrier-gateway
```

## systemd Service

### Daemon (`agentd`)

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
ExecStart=/usr/local/bin/agentd --config /etc/carrier/agentd.json
EnvironmentFile=-/etc/carrier/agentd.env
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
After=network-online.target carrier-agentd.service
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
sudo systemctl enable --now carrier-agentd carrier-gateway
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
journalctl -u carrier-agentd -f --output=json
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

1. Stop services: `sudo systemctl stop carrier-agentd carrier-gateway`
2. Restore files from backup
3. Start services: `sudo systemctl start carrier-agentd carrier-gateway`
4. Verify health: `curl http://localhost:8081/healthz`

## Upgrade Procedure

1. Build or download the new binary
2. Stop the service: `sudo systemctl stop carrier-agentd`
3. Replace the binary: `sudo install -o root -g root -m 0755 agentd /usr/local/bin/agentd`
4. Start the service: `sudo systemctl start carrier-agentd`
5. Verify: `curl http://localhost:8081/healthz`

For agent upgrades (managed via the lifecycle API), the daemon automatically creates pre-upgrade backups in the diagnose directory.

## Security Considerations

- Run as a non-root user with minimal privileges
- Use `ProtectSystem=strict` and `NoNewPrivileges=true` in systemd
- Rotate secrets in `/etc/carrier/agentd.env` regularly
- The gateway uses crypto-secure tokens — do not downgrade to sequential generation
- Review the [security audit](../docs/security-audit-command-execution.md) for command execution hardening
