# Pairing Lifecycle and Troubleshooting Matrix

Source issue: #425

## Pairing Lifecycle

1. Issue pairing code (`/pair <code>` flow with TTL).
2. User submits `/pair <code>` from Telegram/Discord/Feishu.
3. Gateway validates code and binds `(provider, chat_id)` session.
4. Session-enabled commands become available.
5. Session expires/revokes -> user must pair again.

## Operator Actions by Lifecycle Stage

| Stage | Expected Signals | Operator Action |
|---|---|---|
| Pair code issued | Code exists and not expired | Share code through trusted channel only |
| Pair requested | `/pair` received with request_id | Validate provider/chat mapping |
| Paired | response `result=ok` with session token | Proceed with `/agents` smoke check |
| Session required failure | `E_SESSION_REQUIRED` | Re-run pairing flow, verify chat identity |
| Invalid code | `E_PAIR_CODE_INVALID` | Re-issue code and retry within TTL |

## Troubleshooting Matrix

| Error Code | Meaning | Deterministic Operator Action |
|---|---|---|
| `E_PAIR_CODE_INVALID` | code missing/expired/consumed | Generate a new pair code and retry immediately |
| `E_SESSION_REQUIRED` | command sent before pairing | Run `/pair <code>` first, then retry command |
| `E_USAGE` | malformed command args | Re-send command using usage hint from response |
| `E_COMMAND_UNSUPPORTED` | unknown command | Use documented command set only |
| `E_CONSENT_FLAG_INVALID` | `/diagnose-consent` flag not `yes/no` | Re-run with `yes` or `no` |
| `E_REMOTE_DIAG_NOT_NEEDED` | no active unresolved diagnosis | Run `/diagnose <agent>` first or skip consent step |

## Escalation Rule

If the same deterministic error persists after one corrected retry:

1. Collect failed run context (`request_id`, provider, chat_id).
2. Capture daemon/gateway logs.
3. Escalate with the exact error code and reproduction command.
