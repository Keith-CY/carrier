# Daemon API Contract

This document defines the gateway-facing daemon HTTP contract.

Related reference:
- Gateway command contract: `./command-contract.md`

## Base URL

- Daemon API base path: `/api/v1`
- Health endpoints: `/healthz`, `/readyz`

## Endpoint Matrix

| Capability | Method | Path | Notes |
|---|---|---|---|
| Liveness | `GET` | `/healthz` | Returns plain text `ok` when process is alive |
| Readiness | `GET` | `/readyz` | Returns `200 ok` when ready, `503 not ready` during startup/shutdown |
| List pairing codes | `GET` | `/api/v1/pairing/codes` | Lists issued pairing codes |
| Issue pairing code | `POST` | `/api/v1/pairing/codes` | Issues one short-lived code |
| Verify + consume pairing code | `POST` | `/api/v1/pairing/verify-consume` | One-time pairing code verification |
| List agents | `GET` | `/api/v1/agents` | Returns `{ "agents": [...] }` |
| Install agent | `POST` | `/api/v1/agents/{agent_id}/install` | Lifecycle install |
| Start agent | `POST` | `/api/v1/agents/{agent_id}/start` | Lifecycle start |
| Stop agent | `POST` | `/api/v1/agents/{agent_id}/stop` | Lifecycle stop |
| Single-agent status | `GET` | `/api/v1/agents/{agent_id}/status` | One agent status |
| Fleet status | `GET` | `/api/v1/agents/status` | All agents status |
| Agent logs | `GET` | `/api/v1/agents/{agent_id}/logs?tail=<n>` | Tail defaults to 200, max 1000 |
| Upgrade | `POST` | `/api/v1/agents/{agent_id}/upgrade` | Lifecycle upgrade |
| Diagnose | `POST` | `/api/v1/agents/{agent_id}/diagnose` | Diagnostic artifact generation |
| Remote diagnosis handoff | `POST` | `/api/v1/diagnosis/handoffs` | Consent + handoff creation |

## Legacy Alias Routes

The daemon currently keeps compatibility aliases under `/api/*` for existing clients:

- `GET /api/agents`
- `POST /api/install`
- `POST /api/start`
- `POST /api/stop`
- `GET /api/status/{agent_id}`
- `GET /api/logs/{agent_id}`
- `POST /api/upgrade`
- `POST /api/diagnose`
- `GET|POST /api/pairing/codes`
- `POST /api/pairing/verify-consume`

New clients should prefer `/api/v1/*`.

## Response Examples

List agents (`GET /api/v1/agents`):

```json
{
  "agents": [
    {
      "id": "openclaw",
      "name": "OpenClaw",
      "version": "1.0.0",
      "installState": "installed",
      "runtimeState": "running",
      "health": "healthy",
      "ports": [
        9090
      ],
      "restartCount": 1,
      "needsRemoteDiagnosis": false,
      "updatedAt": "2026-02-17T08:00:00Z"
    }
  ]
}
```

Fleet status (`GET /api/v1/agents/status`):

```json
{
  "statuses": [
    {
      "id": "openclaw",
      "name": "OpenClaw",
      "version": "1.0.0",
      "installState": "installed",
      "runtimeState": "running",
      "health": "healthy",
      "ports": [
        9090
      ],
      "restartCount": 1,
      "needsRemoteDiagnosis": false,
      "updatedAt": "2026-02-17T08:00:00Z"
    }
  ]
}
```

Agent logs (`GET /api/v1/agents/openclaw/logs?tail=50`):

```json
{
  "lines": [
    "2026-02-17T08:00:00Z [openclaw] started",
    "2026-02-17T08:00:10Z [openclaw] health=healthy"
  ]
}
```

Pairing verify/consume (`POST /api/v1/pairing/verify-consume`):

Request body:

```json
{
  "code": "pair-4e72e19a9f2a"
}
```

Success response:

```json
{
  "code": "pair-4e72e19a9f2a",
  "consumed": true
}
```

## Error Envelope Compatibility

Current daemon behavior is mixed and clients must handle both patterns:

1. Current native daemon errors (common today):

```json
{
  "error": "agent is not installed"
}
```

2. Structured error envelope (gateway-preferred shape):

```json
{
  "error": {
    "code": "E_NOT_INSTALLED",
    "message": "agent is not installed"
  }
}
```

Gateway transport (`gateway/daemonclient.go`) first tries structured codes and falls back to HTTP-status mapping when only string errors are returned.

## `DaemonAgentState` Field Expectations

Required fields:
- `id`, `name`, `version`, `installState`, `runtimeState`, `health`, `restartCount`, `needsRemoteDiagnosis`, `updatedAt`

Optional fields:
- `ports`, `startedAt`, `lastError`, `lastTriageSummary`, `lastDiagnoseFile`

## Versioning Rule

- Backward-compatible changes: additive fields/endpoints/error codes.
- Breaking changes: new versioned prefix (for example `/api/v2`).
