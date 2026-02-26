# Test-Prefix Issue Batch Triage (2026-02-17)

This document records the pass over all open issues with titles starting with `test(...)`.

| Issue | Status | Notes |
| --- | --- | --- |
| #557 | Already covered on main | `gateway/server_test.go` contains consolidated HTTP method mismatch coverage. |
| #566 | Already covered on main | Missing-signature/header validation tests are present across provider/server parser tests. |
| #569 | Already covered on main | `daemon/internal/lifecycle/service_test.go` includes `loadPersistedState` restoration coverage. |
| #582 | Already covered on main | Process monitoring and deadlock-regression style coverage exists in lifecycle process/state tests. |
| #584 | Already covered on main | Process restart/stale-state behavior is covered in `daemon/internal/lifecycle/process_test.go`. |
| #585 | Already covered on main | Stop semantics after process transitions are covered in lifecycle tests. |
| #594 | Addressed in this PR | Added dedicated kanban config parser tests for with/without `projectId` and malformed config handling. |
| #608 | Addressed in this PR | Added strict numeric parsing regression tests for `/logs` tail and fixed permissive parsing. |
| #609 | Already covered on main | Process-exit lock/transition safety has dedicated lifecycle regression coverage. |
| #610 | Already covered on main | Lifecycle start/stop command compatibility is guarded by lifecycle tests. |
| #623 | Addressed in this PR | Added safe/stable download filename tests and sanitized URL filename behavior. |
| #630 | Addressed in this PR | Added action pinning guard script + tests and wired guard into CI. |
| #634 | Addressed in this PR | Added malformed numeric tail regressions that cover strict parsing edge cases (`+`, `-`, mixed, decimal). |
| #642 | Already covered on main | Daemon HTTP error envelope/content-type behavior is exercised in daemon API/mux tests. |
| #643 | Already covered on main | `daemon/server/server_test.go` covers HTTP mux routes and auth/error branches. |
| #644 | Already covered on main | `daemon/internal/catalog/manifests_test.go` now covers manifest/install command helpers. |
| #645 | Already covered on main | Lifecycle option and service API coverage exists across lifecycle test suite. |
| #646 | Already covered on main | `gateway/daemonclient_test.go` covers daemon transport branches. |
| #647 | Already covered on main | `gateway/server_test.go` has broad route/path/error coverage. |
| #648 | Already covered on main | `gateway/providers_test.go` contains extensive edge-case parser coverage. |
| #649 | Already covered on main | `daemon/internal/api/server_test.go` and `daemon/server/server_test.go` cover API handlers. |
| #650 | Addressed in this PR | Added rate-limiter boundary and expired-window pruning tests; implemented expired session pruning. |
| #652 | Addressed in this PR | Added idempotency tests for `startPeriodicCleanup()`/`stopPeriodicCleanup()`. |
| #654 | Addressed in this PR | Extracted NBS parser logic into tested module; added deterministic dedupe regression tests. |
| #656 | Already covered on main | Natural-exit cleanup behavior is covered in lifecycle process tests. |
| #661 | Addressed in this PR | Added focused download filename normalization/encoding tests for deterministic output. |
