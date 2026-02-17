# Test-Prefix Issue Batch Triage (2026-02-17)

This document records the pass over all open issues with titles starting with `test(...)`.

| Issue | Status | Notes |
| --- | --- | --- |
| #557 | Superseded | Target file `gateway/src/server.test.ts` no longer exists after command-routing refactor. |
| #566 | Superseded | Webhook signature-route layer from the issue scope is no longer in current gateway architecture. |
| #569 | Superseded | `loadPersistedState` path no longer exists in current lifecycle service. |
| #582 | Superseded | `monitorProcess`/ProcessManager deadlock path from issue scope is not present in current lifecycle implementation. |
| #584 | Superseded | Stale PID ProcessManager restart path no longer exists in current lifecycle implementation. |
| #585 | Superseded | Stop-after-unexpected-exit ProcessManager behavior is not part of the current lifecycle model. |
| #594 | Addressed in this PR | Added dedicated kanban config parser tests for with/without `projectId` and malformed config handling. |
| #608 | Addressed in this PR | Added strict numeric parsing regression tests for `/logs` tail and fixed permissive parsing. |
| #609 | Superseded | Process-exit lock re-entry path described in issue no longer exists in current lifecycle design. |
| #610 | Already covered | Command compatibility for start/stop semantics is already guarded by existing lifecycle tests. |
| #623 | Addressed in this PR | Added safe/stable download filename tests and sanitized URL filename behavior. |
| #630 | Addressed in this PR | Added action pinning guard script + tests and wired guard into CI. |
| #634 | Addressed in this PR | Added malformed numeric tail regressions that cover strict parsing edge cases (`+`, `-`, mixed, decimal). |
| #642 | Superseded | Issue targets legacy daemon HTTP API handler paths that are not present in current codebase. |
| #643 | Superseded | Issue targets legacy `cmd/agentd` HTTP mux code paths not present in current codebase. |
| #644 | Superseded | Issue references catalog helper functions that no longer exist in current catalog implementation. |
| #645 | Superseded | Issue references service API functions/coverage targets that changed during lifecycle refactor. |
| #646 | Superseded | Legacy `gateway/src/daemon/http_client.ts` no longer exists in current gateway architecture. |
| #647 | Superseded | Legacy `gateway/src/server.ts` route coverage target no longer exists. |
| #648 | Superseded | Legacy provider parser file from issue scope no longer exists in current gateway architecture. |
| #649 | Superseded | Legacy `daemon/internal/api` package target no longer exists in current codebase. |
| #650 | Addressed in this PR | Added rate-limiter boundary and expired-window pruning tests; implemented expired session pruning. |
| #652 | Addressed in this PR | Added idempotency tests for `startPeriodicCleanup()`/`stopPeriodicCleanup()`. |
| #654 | Addressed in this PR | Extracted NBS parser logic into tested module; added deterministic dedupe regression tests. |
| #656 | Superseded | Natural-exit ProcessManager cleanup path no longer exists in current lifecycle implementation. |
| #661 | Addressed in this PR | Added focused download filename normalization/encoding tests for deterministic output. |
