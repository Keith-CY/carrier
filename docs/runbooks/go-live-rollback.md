# Go-live Checklist and Rollback Steps

Source issue: #426

## Cross-links

- Deployment guide: `docs/deployment.md`
- Pairing runbook: `docs/runbooks/pairing-lifecycle.md`
- CI first response: `docs/ci/first-response-playbook.md`

## Go-live Checklist

1. Confirm CI is green for merge candidate.
2. Validate daemon and gateway binaries for target platform.
3. Run smoke path: `carrier add openclaw` -> pair -> start -> status -> logs.
4. Verify diagnose path and artifact download URL.
5. Confirm audit logs are generated for add/install/start/stop/diagnose.
6. Confirm required secrets/env are present (without printing values).
7. Confirm monitoring and health endpoint checks are active.
8. Publish release notes and known limitations.

## Rollback Triggers

- repeated start failures on healthy hosts
- critical security regression
- diagnose artifacts missing or corrupted
- unsupported command failures on core P0 path

## Rollback Procedure

1. Stop rollout and freeze new changes.
2. Revert to last known good release tag.
3. Restart daemon/gateway with previous binaries.
4. Validate health endpoint and P0 command path.
5. Re-run pairing and one OpenClaw lifecycle smoke test.
6. Capture incident summary and root-cause tracking issue.

## Post-Rollback Validation

- `/health` returns healthy.
- `carrier add openclaw` works in TUI/WebUI.
- `/pair`, `/start`, `/status`, `/stop` execute normally in chat mode.
- audit log and diagnose commands remain available.
