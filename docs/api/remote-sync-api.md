# Remote Host and Sync API (Gateway)

This document covers remote host discovery/sync semantics and instance sync actions.

## Base Path

- `/api/v1/remote/hosts`

## Host Endpoints

### `POST /api/v1/remote/hosts`

Upsert remote host metadata.

Request body (example):

```json
{
  "id": "vps-1",
  "name": "vps-1",
  "host": "203.0.113.10",
  "port": 22,
  "user": "ubuntu",
  "authMode": "private_key",
  "keyPath": "~/.ssh/id_ed25519",
  "runtimeMode": "on_demand"
}
```

### `POST /api/v1/remote/hosts/:hostId/check`

Runs host check and remote instance discovery.  
Supports confirmation-gated pulling for newly discovered instances.

Request body:

```json
{
  "pullNewInstances": false,
  "pullAgentIds": ["picoclaw", "zeroclaw"]
}
```

Fields:
- `pullNewInstances`: when `true`, allows pulling all newly discovered instances.
- `pullAgentIds`: optional allow-list for selective pull of newly discovered instances.

Response fields:
- `check`: host health/preflight result
  - includes `platform` (`os`, `distro`, `version`, `supported`, `reason`)
- `instances`: discovered instances
- `pendingPullInstances`: newly discovered instances that were not pulled yet
- `pullConfirmationRequired`: `true` when pending pull confirmation is required

Behavior:
- First discovery can return pending instances with `pullConfirmationRequired=true`.
- Caller confirms by re-calling check with `pullNewInstances=true` (or targeted `pullAgentIds`).
- Already tracked instances continue to sync without new-instance confirmation.
- Unsupported platform is rejected during preflight (for deterministic install path: Linux required, Alpine rejected).

### `GET /api/v1/remote/hosts/:hostId/instances`

List managed instances for the host.

## Instance Sync Endpoints

### `GET /api/v1/remote/hosts/:hostId/instances/:agentId/config`

Reads instance-level config for the target agent (`openclaw`, `picoclaw`, `zeroclaw`).

### `PATCH /api/v1/remote/hosts/:hostId/instances/:agentId/config`

Applies config patch to the target instance.

Request body:

```json
{
  "patch": {
    "channels": {
      "telegram": {
        "enabled": true
      }
    }
  }
}
```

For `zeroclaw`, use `patch.raw_toml` to send full TOML content.

### `POST /api/v1/remote/hosts/:hostId/instances/:agentId/sync`

Run profile sync for one instance.

Request body:

```json
{
  "mode": "always_push"
}
```

`mode`:
- `always_push`
- `pull_validate_push`
- `manual`

Response includes sync result envelope (`status`, `driftState`, `lastRemoteHash`).

### `GET /api/v1/remote/hosts/:hostId/instances/:agentId/sync/status`

Returns persisted sync status for the instance.

### `POST /api/v1/remote/hosts/:hostId/instances/:agentId/diagnose`

Returns drift diagnosis result.

### `POST /api/v1/remote/hosts/:hostId/instances/:agentId/reconcile`

Returns reconcile outcome (`reconciled`, `driftState`).

### `POST /api/v1/remote/hosts/:hostId/instances/:agentId/rollback`

Request body:

```json
{
  "commit": "<target-commit>"
}
```

`commit` is optional when service can infer target rollback point.

Response includes rollback outcome (`rolledBack`, `fromCommit`, `newCommit`).

### `POST /api/v1/remote/hosts/:hostId/instances/:agentId/uninstall`

Best-effort uninstall/cleanup for target remote instance artifacts.

### `POST /api/v1/remote/keys`

Upload remote SSH private key via multipart form field `file`.  
Response includes `keyRef` and fingerprint metadata.

## Notes

- Remote sync status is persisted in remote control store (`instanceSyncs`).
- Existing provider profile APIs remain backward-compatible.
- Remote install is orchestrated through `carrier remote add ...`, which wraps these APIs in a deterministic sequence.
