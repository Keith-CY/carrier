## Summary

- [ ] Briefly describe the change and why it is needed.

## Milestone

- [ ] `Phase 1`
- [ ] `Phase 2`

## Scope Confirmation

- [ ] This PR stays within the selected milestone scope.
- [ ] If marked `Phase 1`, this work is limited to OpenClaw full lifecycle delivery.
- [ ] Candidate agent work is not introduced before Phase 1 exit criteria pass.

## Audit Traceability

- [ ] PR description includes a "Commit highlights" section summarizing key commit-level deltas (for post-merge audit follow-up).

## Review Readiness Checklist

Local verification (run the checks relevant to your change type):

- [ ] **Daemon changes:** `cd daemon && go test ./...`
- [ ] **Gateway changes:** `cd gateway && bun install && bun run check && bun test`
- [ ] **Docs-only:** Verified markdown links render correctly
- [ ] **Full suite:** `./scripts/run-all-tests.sh` (recommended before final push)

CI checks (must be green before merge):

- [ ] Daemon Tests (`daemon-tests`)
- [ ] Gateway Type Check (`gateway-check`)

See [CONTRIBUTING.md](../CONTRIBUTING.md) for detailed guidance.
