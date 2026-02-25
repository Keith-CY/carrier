# Phase 1 Hardening Evidence Checklist

- Source task: #44
- Micro task: #856 (`T064`)
- Report date: 2026-02-17

## Security

| Scope | Related micro task(s) | Evidence |
|---|---|---|
| Secret redaction rules (allowlist/denylist/pattern coverage) | #831, #836 | `docs/security/redaction-baseline.md`, `shared/redact/redact.go`, `shared/redact/redact_test.go`, `gateway/redact.go`, `gateway/redact_test.go`, `gateway/downloads_test.go` |
| Artifact/download token lifecycle (issue, single-use consume, cleanup) | #818 | `gateway/downloads.go`, `gateway/downloads_test.go`, `gateway/server.go` |
| Release signing and verification chain | #17, #853 | `docs/release-signing.md`, `docs/ARTIFACT_SIGNING.md`, `scripts/sign-artifacts.sh`, `scripts/verify-signature.sh`, `scripts/test-sign-artifacts.sh`, `scripts/test-verify-signature.sh` |

## Audit

| Scope | Related micro task(s) | Evidence |
|---|---|---|
| Audit-event schema (`time/actor/action/target/result/request_id`) | #824 | `daemon/internal/lifecycle/types.go`, `docs/audit-event-dictionary.md` |
| Instrument lifecycle audit events | #825, #826, #827, #828 | `daemon/internal/lifecycle/install.go`, `daemon/internal/lifecycle/start_stop.go`, `daemon/internal/lifecycle/upgrade.go`, `daemon/internal/lifecycle/service.go` |
| Audit query/filter (`actor/action/request_id/result`) | #829 | `daemon/internal/api/server.go` (`GET /api/v1/audit/logs`), `daemon/internal/api/server_test.go` |

## Reliability

| Scope | Related micro task(s) | Evidence |
|---|---|---|
| Auto-recovery boundary and stop conditions (crash-loop + backoff) | #842 | `daemon/internal/lifecycle/backoff.go`, `daemon/internal/lifecycle/start_stop.go`, `daemon/internal/lifecycle/backoff_test.go`, `daemon/internal/lifecycle/crash_loop_test.go` |
| User-facing recovery status and aligned errors | #843 | `daemon/internal/api/server.go` (`DaemonAgentState` fields), `daemon/internal/api/errors.go`, `docs/api/agent-state.md`, `docs/command-contract.md` |
| Evidence collector interface (logs/exit/probes/trace) | #844 | `daemon/internal/lifecycle/evidence.go`, `daemon/internal/lifecycle/evidence_test.go` |
| Structured triage/audit summary per failure cycle | #849 | `daemon/internal/lifecycle/service.go` (`HandleFailure` + audit recording), `daemon/internal/lifecycle/service_test.go`, `docs/audit-event-dictionary.md` |
| Remote diagnosis package + explicit consent handshake | #848 | `daemon/internal/lifecycle/service.go` (`Diagnose`, `CreateRemoteDiagnosisHandoff`), `daemon/internal/api/server.go` (`/api/v1/diagnosis/handoffs`), `gateway/commands.go` (`/diagnose-consent`) |

## Verification snapshot

- `go test ./...` (from `daemon`) passed on 2026-02-17.
- `go test ./internal/gateway/...` (from `daemon`) passed on 2026-02-17.
