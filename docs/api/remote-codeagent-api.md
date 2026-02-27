# Remote CodeAgent API (Gateway)

These endpoints expose remote codeagent lifecycle and execution via managed hosts.

## Endpoints

- `POST /api/v1/remote/hosts/:hostId/instances/:agentId/codeagent/install`
  - Body: `{ "backend": "codex|opencode", "workspaceRoot": "/workspace" }`
  - Installs/verifies backend CLI, then validates health/version.

- `POST /api/v1/remote/hosts/:hostId/instances/:agentId/codeagent/configure`
  - Body: `{ "backend": "...", "workspaceRoot": "...", "profile": { ... } }`

- `GET /api/v1/remote/hosts/:hostId/instances/:agentId/codeagent/health?backend=codex`

- `GET /api/v1/remote/hosts/:hostId/instances/:agentId/codeagent/version?backend=codex`

- `POST /api/v1/remote/hosts/:hostId/instances/:agentId/codeagent/run`
  - Body includes:
    - `backend`
    - `workspaceRoot`
    - `capability` (`read_file|write_file|apply_patch|run_shell|run_shell_redirect`)
    - request payload fields (`path`, `content`, `command`, `stdoutPath`, `stderrPath`, etc.)
  - Response returns normalized run envelope with policy decision and `cost_estimate_usd`.

## Notes

- Strict policy middleware is applied before adapter execution.
- Denied/approval-required requests return normalized policy outcomes without executing remote commands.
- Audit events are appended to gateway audit log (`CARRIER_GATEWAY_AUDIT_LOG` or `~/.carrier/gateway-audit.jsonl`).
