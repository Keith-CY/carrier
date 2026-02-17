# Daemon API Contract (Phase 1)

This document defines the canonical daemon HTTP endpoint and method matrix aligned with `docs/command-contract.md`.

## Base URL

- Local daemon API base path: `/api/v1`
- Health endpoints: `/healthz`, `/readyz`

## Endpoint Matrix

| Gateway Command / Capability | Method | Path | Notes | Implementation Issue |
|---|---|---|---|---|
| Health check (liveness) | `GET` | `/healthz` | Daemon process liveness and metadata | - |
| Health check (readiness) | `GET` | `/readyz` | Readiness gate for traffic | - |
| Pairing code generation | `POST` | `/api/v1/pairing/codes` | Issues short-lived code with TTL | #405 |
| Pairing verify + consume | `POST` | `/api/v1/pairing/verify-consume` | Valid code is one-time and consumed on success | #407 |
| List agents | `GET` | `/api/v1/agents` | Returns catalog + install/runtime state | #386 |
| Install agent | `POST` | `/api/v1/agents/{agent_id}/install` | Lifecycle install path | Planned |
| Start agent | `POST` | `/api/v1/agents/{agent_id}/start` | Lifecycle start path | Planned |
| Stop agent | `POST` | `/api/v1/agents/{agent_id}/stop` | Lifecycle stop path | #389 |
| Agent status (single) | `GET` | `/api/v1/agents/{agent_id}/status` | One-agent status | Planned |
| Agent status (all) | `GET` | `/api/v1/agents/status` | Fleet status summary | Planned |
| Logs | `GET` | `/api/v1/agents/{agent_id}/logs` | Query `tail` optional | Planned |
| Audit logs query | `GET` | `/api/v1/audit/logs` | Query filters: `actor`, `action`, `request_id`, `result`, `limit` | #829 |
| Upgrade | `POST` | `/api/v1/agents/{agent_id}/upgrade` | Returns version transition + rollback metadata | #394 |
| Diagnose | `POST` | `/api/v1/agents/{agent_id}/diagnose` | Returns artifact metadata | #395 |
| Diagnose consent / handoff | `POST` | `/api/v1/diagnosis/handoffs` | Remote diagnosis consent + handoff | Planned |

## Response examples (quick validation)

Lifecycle success example:

```json
{
  "agents": [
    {
      "id": "openclaw",
      "name": "OpenClaw",
      "version": "0.1.0",
      "installState": "installed",
      "installed": true,
      "runtimeState": "running",
      "health": "healthy",
      "ports": [8080],
      "restartCount": 1,
      "lastTriageSummary": "crash loop not detected",
      "needsRemoteDiagnosis": false,
      "updatedAt": "2026-02-14T04:20:00.000Z"
    }
  ]
}
```

Standardized error envelope example:

```json
{
  "error": {
    "code": "E_NOT_INSTALLED",
    "message": "agent is not installed"
  }
}
```

## Required vs optional fields

`DaemonAgentState`:
- Required: `id`, `name`, `version`, `installState`, `runtimeState`, `health`, `restartCount`, `needsRemoteDiagnosis`, `updatedAt`
- Backward-compatible field: `installed` (deprecated; retained for old clients)
- Optional: `ports`, `startedAt`, `lastError`, `lastTriageSummary`, `lastDiagnoseFile`

`AuditLogRecord` (`GET /api/v1/audit/logs`):
- Required: `requestId`, `actor`, `action`, `target`, `result`, `timestamp`
- Optional: `errorCode`, `message`
- Envelope: `{ "auditLogs": [...], "total": <matched-count> }`

`UpgradeResult`:
- Required: `agentId`, `fromVersion`, `toVersion`
- Optional: `backupPath`, `rollbackHint`

`RemoteDiagnosisHandoff`:
- Required: `id`, `agentId`, `consent`, `artifactRef`, `status`, `createdAt`

## Error Envelope

All daemon API error responses MUST use one schema:

```json
{
  "error": {
    "code": "E_ERROR_CODE",
    "message": "Human-readable message"
  }
}
```

## Error Code Mapping (Phase 1 Baseline)

| Error Code | Typical HTTP Status | Meaning |
|---|---|---|
| `E_USAGE` | `400` | Request payload or arguments are invalid |
| `E_PAIR_CODE_INVALID` | `400` | Pairing code is missing, expired, or invalid |
| `E_AGENT_NOT_FOUND` | `404` | Agent ID not found |
| `E_NOT_INSTALLED` | `409` | Agent must be installed first |
| `E_ALREADY_RUNNING` | `409` | Agent is already running |
| `E_ALREADY_STOPPED` | `409` | Agent is already stopped |
| `E_AGENT_RUNNING` | `409` | Operation requires stopped agent |
| `E_UPGRADE_NOT_SUPPORTED` | `400` | Upgrade command is not available for agent |
| `E_RUNTIME_PREREQUISITES` | `422` | Runtime prerequisites check failed |
| `E_MISSING_REQUIRED_ENV` | `422` | Required environment variable missing |
| `E_PORT_CONFLICT` | `422` | Required port is unavailable |
| `E_UPGRADE_FAILED` | `500` | Upgrade command failed after invocation |
| `E_UPGRADE_STRATEGY_UNSUPPORTED` | `400` | Unsupported upgrade strategy in manifest |
| `E_INTERNAL` | `500` | Unexpected internal server error |

## Versioning Rule

- Backward-compatible additions: add new fields, paths, or error codes without changing existing semantics.
- Breaking changes: require a new API version prefix (for example `/api/v2`).
