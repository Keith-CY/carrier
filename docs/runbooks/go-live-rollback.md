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
4. Run remote control plane WebUI smoke:
   - `#/servers`: create -> edit/update -> check -> delete.
   - `#/profiles`: create/edit profile -> bind -> delete binding -> profile test with explicit host selection.
   - `#/remote-chat`: verify stream starts for `target=remote` and `target=local`.
   - `#/remote-observability`: verify `rollout.state=healthy` before full enablement (`canary` only for limited cohort, `hold` blocks rollout).
5. Verify WebUI TypeScript build artifact step passes (`bash scripts/build-webui.sh`) and generated `webui/static/*.js` assets are present for packaging.
6. Verify diagnose path and artifact download URL.
7. Confirm audit logs are generated for add/install/start/stop/diagnose.
8. Confirm required secrets/env are present (without printing values).
9. Confirm monitoring and health endpoint checks are active.
10. Publish release notes and known limitations.

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
