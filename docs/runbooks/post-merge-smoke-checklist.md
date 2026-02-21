# Post-Merge Smoke Checklist

Scope: lightweight regression pass for onboarding/transport/managed-instance flows after broad PRs.

Related follow-up issues: #1295, #1296, #1297, #1281, #1282.

## Targeted Smoke Pass

1. Transport auto fallback visibility
- Start gateway with Telegram `auto` mode and missing/invalid webhook URL.
- Confirm selected mode is `polling`.
- Confirm fallback reason code and remediation hint appear in gateway status output/logs.

2. Settings transport hint rendering
- Open WebUI onboarding/settings surfaces.
- Verify transport mode, selected mode, and fallback reason/hint are rendered consistently.

3. Managed instance lifecycle
- Run managed add flow for PicoClaw (WebUI or CLI path).
- Verify `add -> start -> stop -> uninstall` all succeed without stale instance metadata.
- Verify failure paths return sanitized user-facing errors.

## Redaction Boundary

`RedactErrorMessage` in `daemon/internal/gateway/redact.go` delegates to `daemon/internal/redact`.

Current boundary:
- Redact key/value pairs with keys containing `API_KEY`, `SECRET`, `TOKEN`, `PASSWORD`, or `CREDENTIAL`.
- Redact URL-embedded credentials (`scheme://user:pass@host`).
- Keep actionable non-secret diagnostics in server logs.

## Stop-Path Performance Note

`lookupPIDsBySocketInode` in `cmd/carrier/main.go` scans `/proc/*/fd` when `lsof` is unavailable.
This fallback is only for `carrier stop` recovery paths and should not be reused for hot-path runtime logic.

## Ongoing Decomposition Notes

- Shared provider credential persistence is centralized in `daemon/credentialstore` and reused by CLI + gateway.
- Continue extracting domain helpers from `cmd/carrier/main.go` incrementally (onboarding flow, managed instance operations, bootstrap lifecycle) in follow-up PRs.
