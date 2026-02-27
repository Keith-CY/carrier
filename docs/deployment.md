# Carrier Deployment Guide

This guide reflects the current runtime model:
- one `carrier` binary
- daemon service (`carrier daemon`)
- gateway service (`carrier gateway`)

Related runbooks:
- [`docs/runbooks/go-live-rollback.md`](./runbooks/go-live-rollback.md)
- [`docs/runbooks/pairing-lifecycle.md`](./runbooks/pairing-lifecycle.md)
- [`docs/ci/first-response-playbook.md`](./ci/first-response-playbook.md)

## 1) Prerequisites

- Linux host (amd64/arm64 recommended for production)
- systemd
- dedicated non-root user (example: `carrier`)
- TLS/ingress policy if exposing gateway outside loopback

## 2) Install Binary

Recommended (release script):

```bash
curl -fsSL https://raw.githubusercontent.com/Keith-CY/carrier/main/scripts/install.sh | bash
carrier --version
```

Source build option:

```bash
go build -o carrier ./cmd/carrier
sudo install -o root -g root -m 0755 carrier /usr/local/bin/carrier
```

## 3) Runtime Configuration

Daemon config source:
- loads `config.json` in current working directory if present
- then applies `CARRIER_*` env overrides

Important daemon env vars:
- `CARRIER_SERVER_HOST` (default `127.0.0.1`)
- `CARRIER_SERVER_PORT` (default `9090`)
- `CARRIER_SERVER_API_TOKEN` (required when binding daemon to non-loopback host)

Important gateway env vars:
- `CARRIER_GATEWAY_HOST` (default `127.0.0.1`)
- `CARRIER_GATEWAY_PORT` (default `8787`)
- `CARRIER_GATEWAY_API_TOKEN` (required when binding gateway to non-loopback host)
- `CARRIER_DAEMON_BASE_URL` (default `http://127.0.0.1:9090`)
- `CARRIER_SERVER_API_TOKEN` (gateway->daemon bearer token when daemon token is enabled)

Remote feature flags:
- `CARRIER_REMOTE_CONTROL_PLANE_ENABLED` (default `true`)
- `CARRIER_REMOTE_CHAT_ENABLED` (default `true`)
- `CARRIER_PROVIDER_BINDING_ENABLED` (default `true`)

## 4) systemd Units

Create runtime directories:

```bash
sudo mkdir -p /var/lib/carrier
sudo chown carrier:carrier /var/lib/carrier
```

Optional env file:

```bash
sudo mkdir -p /etc/carrier
sudo touch /etc/carrier/carrier.env
sudo chmod 600 /etc/carrier/carrier.env
```

Example `/etc/carrier/carrier.env`:

```bash
# daemon
CARRIER_SERVER_HOST=127.0.0.1
CARRIER_SERVER_PORT=9090

# gateway
CARRIER_GATEWAY_HOST=127.0.0.1
CARRIER_GATEWAY_PORT=8787
CARRIER_DAEMON_BASE_URL=http://127.0.0.1:9090

# optional hardening for non-loopback binds
# CARRIER_SERVER_API_TOKEN=<daemon_token>
# CARRIER_GATEWAY_API_TOKEN=<gateway_token>
```

### Daemon unit

`/etc/systemd/system/carrier-daemon.service`

```ini
[Unit]
Description=Carrier Daemon
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=carrier
Group=carrier
WorkingDirectory=/var/lib/carrier
EnvironmentFile=-/etc/carrier/carrier.env
ExecStart=/usr/local/bin/carrier daemon
Restart=on-failure
RestartSec=5s
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/carrier

[Install]
WantedBy=multi-user.target
```

### Gateway unit

`/etc/systemd/system/carrier-gateway.service`

```ini
[Unit]
Description=Carrier Gateway
After=network-online.target carrier-daemon.service
Wants=network-online.target

[Service]
Type=simple
User=carrier
Group=carrier
WorkingDirectory=/var/lib/carrier
EnvironmentFile=-/etc/carrier/carrier.env
ExecStart=/usr/local/bin/carrier gateway
Restart=on-failure
RestartSec=5s
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/carrier

[Install]
WantedBy=multi-user.target
```

Enable/start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now carrier-daemon carrier-gateway
```

## 5) Health Checks

- Daemon: `GET http://127.0.0.1:9090/healthz`
- Gateway: `GET http://127.0.0.1:8787/healthz`

Readiness:
- Daemon also exposes `GET /readyz`

Quick verify:

```bash
curl -fsS http://127.0.0.1:9090/healthz
curl -fsS http://127.0.0.1:8787/healthz
```

## 6) Upgrade

1. install new `carrier` binary
2. restart services:

```bash
sudo systemctl restart carrier-daemon carrier-gateway
```

3. re-check health endpoints and smoke commands

## 7) Backup and Recovery

Back up at least:
- `/etc/carrier/`
- `/var/lib/carrier/`
- user config under `~/.carrier/` for operator accounts that run local onboarding/control

Restore:
1. stop services
2. restore files
3. start services
4. verify `/healthz` and basic command path

## 8) Remote VPS OpenClaw Deployment

Operationally, remote rollout should use deterministic CLI flow:

```bash
carrier remote add openclaw \
  --host-id <id> \
  --host <ip-or-domain> \
  --port <port> \
  --user <ssh-user> \
  --key-path <private-key-path>
```

This wraps gateway remote APIs with fixed sequencing and retry behavior.

## 9) Security Considerations

- Run as a non-root user with minimal privileges.
- Keep `ProtectSystem=strict` and `NoNewPrivileges=true` in systemd units.
- Rotate secrets in `/etc/carrier/carrier.env` regularly.
- If daemon/gateway bind to non-loopback, enforce API tokens.

### SSH Host Key Verification

Remote control plane SSH uses `StrictHostKeyChecking=accept-new` (TOFU):
- first-seen host keys are accepted and stored
- changed host keys are rejected on later connections

For production, pre-populate `known_hosts` where possible:

```bash
ssh-keyscan -H <remote-host> >> ~/.ssh/known_hosts
```
