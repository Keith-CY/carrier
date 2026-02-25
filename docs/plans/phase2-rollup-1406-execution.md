# Phase 2 Rollup Execution Plan (`#1406`)

This plan decomposes the consolidated roadmap epics from `#1224-#1239` into
trackable execution buckets under issue `#1406`.

## Buckets

### Bucket 1 — Multi-agent runtime messaging
- **Source epics:** `#1224`, `#1227`
- **Deliverables:** daemon message bus API, routing policy, bounded queue semantics.

### Bucket 2 — WebUI high-scale communication surfaces
- **Source epics:** `#1227`, `#1228`
- **Deliverables:** conversation room UX model, multi-agent log tabs, virtualization path.

### Bucket 3 — First-run and fix-loop UX
- **Source epics:** `#1232`, `#1233`, `#1234`
- **Deliverables:** resumable onboarding checkpoints, diagnose-and-fix workflow,
  template apply preview and conflict-safe apply.

### Bucket 4 — Safe-by-default platform hardening
- **Source epics:** `#1236`, `#1237`, `#1239`
- **Deliverables:** secure baseline assertions, rollback-safe high-impact operations,
  pre/post upgrade health gates and impact summary.

## Exit Conditions

1. Each bucket has explicit child implementation tasks.
2. Each child task has merged code and verification evidence.
3. `#1406` remains open until all bucket child tasks are complete.

## Tracking Note

Do not reopen `#1224-#1239` for execution-level progress. Track implementation
in `#1406` child tasks to keep one roadmap execution thread.
