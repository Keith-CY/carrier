# Phase 1 Issue Closure Notes (2026-02-17)

This document captures how the open `Phase 1` label issues are resolved in a single batch PR.

## Addressed by code/doc changes in this PR

| Issue | Resolution |
| --- | --- |
| #615 | Download HTTP/header helpers were extracted to `daemon/internal/gateway/downloads.go`; route logic in `daemon/internal/gateway/server.go` now focuses on orchestration; table-driven helper tests added in `daemon/internal/gateway/downloads_test.go`. |
| #626 | README artifact naming updated to real release outputs: `carrier-<commit-sha>-<os>-<arch>.zip` and matching `.zip.sha256`. |
| #613 | README pairing-code flow now matches runtime behavior: daemon prints `pair-<hex>`, TTL is 5 minutes, and API fallback (`POST /api/v1/pairing/codes`) is documented. |
| #612 | `.github/workflows/release.yml` now documents least-privilege intent explicitly, with default read-only permissions and release-only write scope. |
| #295 | Telegram webhook path is now wired in gateway runtime (`POST /webhook/telegram`) with webhook secret verification and command execution/response rendering coverage in `daemon/internal/gateway/server_webhook_test.go`. |

## Already satisfied on current main baseline (validated and documented here)

| Issue | Evidence |
| --- | --- |
| #296 | Discord webhook verification, parsing, command normalization, and response rendering are implemented in `daemon/internal/gateway/server.go`, `daemon/internal/gateway/providers.go`, and covered by `daemon/internal/gateway/server_webhook_test.go`. |
| #297 | Feishu event token verification, parsing, command normalization, and response rendering are implemented in `daemon/internal/gateway/server.go`, `daemon/internal/gateway/providers.go`, and covered by `daemon/internal/gateway/server_webhook_test.go`. |
| #298 | Cross-provider parity checks exist in `daemon/internal/gateway/providers_test.go` and `daemon/internal/gateway/server_test.go`, and are enforced by CI matrix job in `.github/workflows/ci.yml` (`Provider Parity (...)`). |
| #294 | Round-1 three-provider closure objective is represented by provider webhooks/parsers/renderers + parity suite + runbook coverage (`docs/provider-setup-runbook.md`, `docs/command-contract.md`). |
| #598 | The old pinned-checksum fallback path no longer exists in catalog code. OpenClaw install now uses official installer URL in `daemon/internal/catalog/manifests.go`, with guard tests in `daemon/internal/catalog/manifests_test.go`. |
| #600 | The previous empty `CHECKSUM` embedded-script failure mode is removed by the same installer architecture change; install command contract is validated in `daemon/internal/catalog/manifests_test.go`. |

## Still open

None — all Phase 1 labeled issues are resolved as of this batch.
