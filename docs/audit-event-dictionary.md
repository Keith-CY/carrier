# Audit Event Dictionary

Source issue: #830

This document defines audit event actions and field meanings used by daemon/gateway flows.

## Canonical Daemon Audit Fields

From `daemon/internal/lifecycle/types.go`:

- `request_id`
- `actor`
- `action`
- `target`
- `result` (`success` | `failure` | `neutral`)
- `error_code`
- `message`
- `timestamp`

## Query API

Daemon audit logs can be queried via:

- `GET /api/v1/audit/logs`

Supported query filters:

- `actor`
- `action`
- `request_id`
- `result` (`success` | `failure` | `neutral`)
- `limit` (positive integer, capped server-side)

Response envelope:

```json
{
  "auditLogs": [
    {
      "requestId": "req-100",
      "actor": "operator",
      "action": "remote_diagnosis_consent",
      "target": "openclaw",
      "result": "failure",
      "errorCode": "E_REMOTE_DIAG_NOT_NEEDED",
      "message": "remote diagnosis not required",
      "timestamp": "2026-02-17T10:00:00Z"
    }
  ],
  "total": 1
}
```

## Action Dictionary

| Action | Target | Typical Result | Example Message |
|---|---|---|---|
| `install` | agent id | `success` / `failure` | `install completed` |
| `start` | agent id | `success` / `failure` | `start completed` |
| `stop` | agent id | `success` / `failure` | `stop completed` |
| `status` | agent id or `*` | `success` | `status fetched` |
| `logs` | agent id | `success` | `tail=200` |
| `upgrade` | agent id | `success` / `failure` | `upgrade_success from=... to=...` |
| `diagnose` | agent id | `success` / `failure` | `/tmp/openclaw-diagnose-1.zip` |
| `triage` | agent id | `success` | `<triage summary>` |
| `remote_diagnosis_consent` | agent id | `success` / `neutral` / `failure` | `consent=true handoff_id=...` |
| `handoff_cleanup` | `diagnosis_handoffs` | `success` | `removed=3` |

## Example Audit Events

### Lifecycle success

```json
{
  "request_id": "req-100",
  "actor": "system",
  "action": "start",
  "target": "openclaw",
  "result": "success",
  "error_code": "",
  "message": "start completed",
  "timestamp": "2026-02-17T10:00:00Z"
}
```

### Consent declined (neutral)

```json
{
  "request_id": "req-201",
  "actor": "operator",
  "action": "remote_diagnosis_consent",
  "target": "openclaw",
  "result": "neutral",
  "error_code": "",
  "message": "consent=false handoff_id=handoff-12",
  "timestamp": "2026-02-17T10:05:00Z"
}
```

## Gateway Test-Harness Audit Notes

Gateway tests under `gateway/` also validate request metadata propagation for test flows with fields:

- `requestId`, `actor`, `action`, `target`, `message`, `timestamp`

These are useful for gateway-level consistency tests but daemon lifecycle audit logs remain source-of-truth.
