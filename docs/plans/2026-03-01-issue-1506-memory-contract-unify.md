# Issue 1506 Execution Plan: Memory Contract Unification

## Summary
- Create isolated worktree on branch `codex/issue-1506-memory-contract-unify`.
- Unify memory injection semantics across local install/start and remote execution.
- Remove lifecycle fallbacks and fail fast when env injection is unsupported.
- Add remote run API and memory sync status fields.
- Add instance-level memory git sync support with real fetch/pull/push.

## Locked Decisions
- Empty attachments still inject an empty memory contract.
- No fallback for process manager env injection.
- Contract mismatch policy: auto-rebuild.
- Remote coverage: openclaw / picoclaw / zeroclaw.
- Fixed remote memory dirs for pico/zero: `$HOME/.picoclaw/memory`, `$HOME/.zeroclaw/memory`.
- Memory git sync runs in gateway-local context; auth uses system git credentials.

## Batches
1. Local lifecycle memory contract pipeline hardening.
2. Memory conflict/provenance semantics.
3. Remote ensure+inject wrapper and new `/api/remote/instances/{id}/run` API.
4. Instance-level memory git sync + status fields.
5. Documentation and regression verification.

## Verification
- `go test ./daemon/internal/lifecycle/... ./daemon/internal/memory/...`
- `go test ./gateway/... -run 'Test(Remote.*Memory|Remote.*Run|Remote.*Sync)'`
- `go test ./profilesync/... -run 'Test(Git.*Memory|Memory.*Sync)'`
- `go test ./daemon/... ./gateway/... ./profilesync/...`
