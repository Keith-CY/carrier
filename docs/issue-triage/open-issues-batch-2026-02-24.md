# Open Issue Batch Handling (2026-02-24)

This document records how the current open-issue set is handled in one aggregate PR.

## Directly implemented in this PR

| Issue | Handling |
| --- | --- |
| #1396 | Replaced misleading `local` provider category usage for OpenAI-compatible endpoints; provider grouping now uses `custom` bucket and onboarding copy is updated. |
| #1395 | Canonical provider ID changed from `vllm` to `openai-compatible`; backward-compatible aliases (`vllm`, `openai-v1`) preserved. |
| #1384 | OpenClaw manifest install/upgrade commands now download installer scripts to tmpfile before execution (no direct `curl | bash`). |
| #1379 | Log append diff moved from quadratic overlap scan to linear-time prefix-function overlap detection in WebUI. |
| #1330 | Log table rendering now uses `DocumentFragment` + DOM node construction instead of one-shot `innerHTML` rebuild. |
| #1336 | Hourly kanban workflow now emits structured clean-tree signal: `dirtyFileCount`, `dirtyFingerprint`, `source`. |
| #1337 | Hourly kanban workflow now enforces clean-tree gate with configurable policy (`fail-fast` or `auto-stash`). |
| #1338 | Existing open audit PR delta comment upsert remains active and is retained (validated while touching same workflow). |
| #1339 | Existing-open-PR skip path now emits `healthy-skip` vs `stale-skip` classification in board summary and delta comment. |
| #1340 | Hourly audit output now includes actionable TODO/TBD/FIXME hit details (`file:line + snippet`) for docs/core scopes. |
| #1341 | Hourly audit output now reports test surface drift with added/removed test file names. |
| #1366 | Added guardrail check to fail if hourly audit sync mutates `docs/current-architecture.md`; canonical/snapshot contract documented. |
| #1326 | Docs marker debt remediated: removed inline TODO/TBD/FIXME markers from docs snapshot artifact; scanner excludes generated snapshot file. |
| #1393 | Same remediation as #1326 (duplicate docs marker debt report). |

## Baseline already satisfied (validated and documented in this PR)

| Issue | Handling |
| --- | --- |
| #1325 | Canonical architecture index linkage and superseded-plan pointers already present; reconfirmed with docs contract update. |
| #1327 | Core TODO/FIXME backlog currently zero in source modules; validated in batch run and reflected in updated audit workflow output. |

## Decomposition-required epics (kept as roadmap items)

| Issue | Handling |
| --- | --- |
| #1224 | Cross-agent message bus is multi-phase architecture work; kept open as roadmap epic (not force-closed in this batch). |
| #1227 | WebUI conversation-room model is a large feature set; kept open as roadmap epic. |
| #1228 | WebUI virtual scroll + multi-agent tabs requires broader UI/state refactor; kept open as roadmap epic. |
| #1232 | First-run onboarding wizard is broad product flow; kept open as roadmap epic. |
| #1233 | Diagnose-and-fix automation is broad feature and safety surface; kept open as roadmap epic. |
| #1234 | Starter template catalog + diff/rollback is broad feature; kept open as roadmap epic. |
| #1236 | Secure-by-default baseline spans policy engine and CI hardening; kept open as roadmap epic. |
| #1237 | Reversible operations + rollback-safe config requires transactional model work; kept open as roadmap epic. |
| #1239 | Upgrade pre/post checks + rollback orchestration is broad release workflow work; kept open as roadmap epic. |

## System issue intentionally kept open

| Issue | Handling |
| --- | --- |
| #119 | Pinned hourly kanban issue is a workflow state sink and should remain open by design. |

