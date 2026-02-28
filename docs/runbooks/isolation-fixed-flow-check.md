# Isolation Fixed-Flow Check

This runbook provides a single entrypoint to verify the deterministic isolation flow for:

- `openclaw`
- `picoclaw`
- `zeroclaw`
- remote code-agent installer path for `codex` / `opencode`

## Command

```bash
bash scripts/isolation-fixed-flow-check.sh
```

The default mode runs deterministic tests only:

- daemon lifecycle isolation pipeline tests
- gateway remote code-agent installer tests
- CLI `carrier add <agent> --isolation` payload e2e tests

## Optional live smoke

To run real local add-flow smoke after tests:

```bash
bash scripts/isolation-fixed-flow-check.sh --live
```

To run live smoke only:

```bash
bash scripts/isolation-fixed-flow-check.sh --live-only
```

Live mode prerequisites:

1. `carrier` is available in `PATH`.
2. Provider credential is already saved in Carrier and can be auto-reused.
3. Export dedicated Telegram tokens:
   - `OPENCLAW_TELEGRAM_BOT_TOKEN`
   - `PICOCLAW_TELEGRAM_BOT_TOKEN`
   - `ZEROCLAW_TELEGRAM_BOT_TOKEN`

## Expected outcome

When successful, script ends with:

```text
isolation fixed-flow check completed
```
