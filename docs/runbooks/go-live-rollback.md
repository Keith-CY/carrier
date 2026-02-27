# Go-Live Checklist and Rollback

Source issue: #426

## Cross-links

- Deployment guide: [`docs/deployment.md`](../deployment.md)
- Pairing runbook: [`docs/runbooks/pairing-lifecycle.md`](./pairing-lifecycle.md)
- CI first response: [`docs/ci/first-response-playbook.md`](../ci/first-response-playbook.md)

## Go-live Checklist

1. Confirm required CI checks are green for merge candidate.
2. Validate target platform binaries (`carrier --version`).
3. Start services and verify health:
   - daemon: `http://127.0.0.1:9090/healthz`
   - gateway: `http://127.0.0.1:8787/healthz`
4. Run local smoke:
   - `carrier onboard` (or `carrier onboard --webui`)
   - `carrier add openclaw`
   - `carrier status openclaw`
5. Run remote control-plane smoke:
   - host create/update/check/delete
   - provider profile create/edit/test
   - remote observability rollout card status
6. Run deterministic remote install smoke:
   - run:
     ```bash
     carrier remote add openclaw \
       --host-id <host-id> \
       --host <ip-or-domain> \
       --port <ssh-port> \
       --user <ssh-user> \
       --key-path <private-key-path>
     ```
   - verify post-install output shows:
     - `SSH connectivity: OK.`
     - `OpenClaw runtime: detected.`
7. Validate first-discovery confirmation path:
   - newly discovered instances are listed in the interactive confirmation prompt
   - after confirmation, output reports successful config import for discovered instances
8. Verify audit logs and diagnose path.
9. Confirm required secrets/env are present (without printing secret values).
10. Publish release notes and known limits.

## Rollback Triggers

- repeated start failures on healthy hosts
- critical security regression
- diagnose artifacts missing/corrupted
- deterministic remote CLI install path regresses
- remote discovery/confirmation flow regresses

## Rollback Procedure

1. Stop rollout and freeze new changes.
2. Revert to last known good release tag.
3. Restart daemon and gateway with previous binary.
4. Re-verify `/healthz` endpoints.
5. Re-run one local OpenClaw lifecycle smoke.
6. Re-run one remote install smoke (`carrier remote add openclaw ...`).
7. Capture incident summary and open root-cause tracking issue.

## Post-Rollback Validation

- daemon and gateway `/healthz` return `{"status":"ok"}`
- `carrier add openclaw` works in CLI/TUI/WebUI
- chat management commands (`/pair`, `/agents`, `/start`, `/status`, `/stop`) work
- if chat `/install` policy is enabled, `/install <agent_id> <host_id>` works as expected
- diagnose and audit paths remain available
