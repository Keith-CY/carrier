# Cross-Provider E2E Parity Taxonomy

This document defines the parity assertion categories used for Telegram/Discord/Feishu command-path E2E tests.

Reference implementation/tests:
- `daemon/internal/gateway/providers_test.go`
- `daemon/internal/gateway/server_test.go`
- `docs/plans/phase1-e2e-test-matrix.md`

## Categories

| Category | Definition | Telegram/Discord/Feishu example | Assertion style |
| --- | --- | --- | --- |
| Request parsing parity | The same logical command must parse to the same command name and argument shape across providers. | `telegram 100 req-t /install openclaw`, `discord 100 req-d /install openclaw`, `feishu 100 req-f /install openclaw` all parse to `name="/install"` and `args=["openclaw"]`. | Normalized comparison (ignore `provider`, `chatId`, and `requestId`; compare `name` + `args`). |
| Command normalization parity | Defaulting and argument normalization must be provider-agnostic. | `/logs openclaw` should resolve to the same default tail (`200`) for Telegram/Discord/Feishu before daemon call. | Exact match on normalized command intent (same defaulted values and downstream action). |
| Response rendering parity | For the same command outcome, response envelope semantics must match across providers. | `/status` returns the same `result` and equivalent message semantics on all three providers. | Normalized comparison (ignore volatile fields such as `requestId`, `sessionToken`, `downloadUrl`). |
| Error-shape parity | Error responses must keep a stable envelope and error code contract across providers. | Unpaired `/agents` must return `result="error"` and `errorCode="E_SESSION_REQUIRED"` for Telegram/Discord/Feishu. | Exact match on `result`, `errorCode`, and required error fields; message text should remain semantically equivalent. |

## Assertion policy summary

- Use exact assertions for stable contract fields (`result`, `errorCode`, required field presence).
- Use normalized assertions for transport-specific or dynamic fields (`requestId`, generated tokens/URLs).
- When in doubt, prefer asserting behavior-level equivalence over provider-specific string formatting.
