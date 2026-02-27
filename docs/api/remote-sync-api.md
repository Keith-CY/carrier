# Remote Sync API (Gateway)

These endpoints extend the remote control plane for managed instance profile synchronization.

## Endpoints

- `POST /api/v1/remote/hosts/:hostId/instances/:agentId/sync`
  - Body: `{ "mode": "always_push|pull_validate_push|manual" }` (optional, default `always_push`)
  - Response: sync result envelope (`status`, `driftState`, `lastRemoteHash`)

- `GET /api/v1/remote/hosts/:hostId/instances/:agentId/sync/status`
  - Response: persisted sync status for that instance

- `POST /api/v1/remote/hosts/:hostId/instances/:agentId/diagnose`
  - Response: drift diagnosis result

- `POST /api/v1/remote/hosts/:hostId/instances/:agentId/reconcile`
  - Response: reconcile outcome (`reconciled`, `driftState`)

- `POST /api/v1/remote/hosts/:hostId/instances/:agentId/rollback`
  - Body: `{ "commit": "<target-commit>" }` (optional if service can infer latest common commit)
  - Response: rollback outcome (`rolledBack`, `fromCommit`, `newCommit`)

## Notes

- Sync status is persisted in remote control store (`instanceSyncs`).
- `syncMode` now supports `always_push`, `pull_validate_push`, and `manual`.
- Existing remote host and provider profile APIs remain backward-compatible.
