# Security Baseline: Redaction Guarantees and Limits

Source issue: #837

Implementation reference: `daemon/internal/redact`

## Guarantees

1. Environment keys containing any of these substrings are redacted:
- `API_KEY`
- `SECRET`
- `TOKEN`
- `PASSWORD`
- `CREDENTIAL`

2. Text redaction masks assignment-like secrets, e.g.:
- `OPENAI_API_KEY=...`
- `token: ...`
- `PASSWORD = ...`

3. URL credentials are masked, e.g.:
- `postgres://user:pass@host` -> `postgres://user:***REDACTED***@host`

4. Redacted placeholder value is fixed:
- `***REDACTED***`

5. Diagnostic metadata includes checksum and expiry fields for artifact integrity/retention tracking.

## Explicit Limits

1. Pattern-based matching may miss unconventional secret naming.
2. Free-form sensitive text without recognized key/value shape may not be fully scrubbed.
3. Redaction is not encryption and does not provide confidentiality at rest.
4. Binary payload internals are not semantically parsed for secret extraction.

## Operator Responsibilities

1. Keep secret names aligned with redaction heuristics.
2. Avoid writing raw secrets into free-form logs.
3. Restrict artifact access with short-lived download tokens.
4. Enforce retention and cleanup policies for diagnostic artifacts.

## Validation Pointers

- `daemon/internal/redact/redact_test.go`
- `docs/audit-event-dictionary.md`
- `docs/ci/release-artifact-retention.md`
