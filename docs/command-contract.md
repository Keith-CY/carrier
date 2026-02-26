# Command Contract

This document defines the unified input/output contract for all gateway commands. Every command MUST produce identical results regardless of which provider (Telegram, Discord, Feishu) delivers it.

Related references:
- Daemon payload examples: `./daemon-api-contract.md`
- Cross-provider parity taxonomy: `./e2e-parity-taxonomy.md`
- Base-agent boundary policy spec: `../baseagent/spec/baseagent-boundary.v1.json`

## Input Format

All commands are parsed from a single string with the format:

```
<provider> <chat_id> <request_id> [session_token] <command> [...args]
```

| Field        | Type                                  | Description                        |
|-------------|---------------------------------------|------------------------------------|
| `provider`  | `"telegram" \| "discord" \| "feishu"` | Originating platform               |
| `chat_id`   | `string`                              | Platform-specific chat identifier  |
| `request_id`| `string`                              | Unique request identifier          |
| `session_token` | `string` (optional)               | Session token for authenticated commands |
| `command`   | `CommandName`                         | One of the commands listed below   |
| `args`      | `string[]`                            | Positional arguments (space-split) |

When commands are sent via gateway HTTP (`POST /command`):
- request body can be plain text command input, or JSON:
  - `{ "input": "<provider> <chat_id> <request_id> [session_token] <command> [...args]" }`
  - `{ "input": "...", "sessionToken": "<session_token>" }`
- if `CARRIER_GATEWAY_API_TOKEN` is configured, include `Authorization: Bearer <gateway_api_token>`
- command request body is capped by `CARRIER_MAX_COMMAND_BODY_BYTES` (default: `65536`); oversized requests return `413 E_PAYLOAD_TOO_LARGE`
- for non-`/pair` commands, session token is required and should be sent via `x-session-token` header or JSON `sessionToken` field

**Session token transport:** When `CARRIER_GATEWAY_API_TOKEN` is enabled, the
`Authorization` header is reserved for the gateway API token. Session tokens
must be sent via the `x-session-token` header or the `sessionToken` body field
instead. When the gateway token is **not** configured, `Authorization: Bearer
<session_token>` continues to work as a backward-compatible transport.

## Response Schema

Every command returns a `GatewayResponse`:

```typescript
type GatewayResponse = {
  requestId: string;
  result: "ok" | "error";
  message: string;
  errorCode?: string;         // present when result = "error"
  sessionToken?: string;      // only /pair success
  downloadUrl?: string;       // /logs, /diagnose, /diagnose-consent
  handoffId?: string;         // /diagnose-consent success
  handoffStatus?: "pending" | "declined"; // /diagnose-consent success
};
```

## Error Codes

| Code                             | Meaning                                               |
|----------------------------------|-------------------------------------------------------|
| `E_PARSE`                        | Input string could not be parsed                      |
| `E_USAGE`                        | Missing required argument(s) or invalid payload shape |
| `E_PAYLOAD_TOO_LARGE`            | `/command` request body exceeds size limit            |
| `E_GATEWAY_AUTH_REQUIRED`        | Gateway API token is required for `/command` access   |
| `E_GATEWAY_AUTH_INVALID`         | Provided gateway API token is invalid                 |
| `E_AUTH_REQUIRED`                | Session token required for authenticated command      |
| `E_AUTH_INVALID`                 | Provided session token is invalid                     |
| `E_PAIR_CODE_INVALID`            | Pairing code is invalid or expired                    |
| `E_SESSION_REQUIRED`             | Chat is not paired; must run `/pair` first            |
| `E_INSTALL_GUI_ONLY`            | Chat install is disabled; use Carrier TUI/WebUI       |
| `E_ONBOARD_GUI_ONLY`            | Chat onboarding is disabled; use Carrier TUI/WebUI    |
| `E_HOST_BINDING_REQUIRED`        | Install requires a bound/selected remote host         |
| `E_REMOTE_INSTALL_FAILED`        | Remote host install flow failed                       |
| `E_REMOTE_HOST_NOT_FOUND`        | Referenced remote host does not exist                 |
| `E_NOT_INSTALLED`                | Agent is not installed                                |
| `E_ALREADY_RUNNING`              | Agent is already running                              |
| `E_ALREADY_STOPPED`              | Agent is already stopped                              |
| `E_AGENT_NOT_FOUND`              | Target agent does not exist                           |
| `E_CONSENT_FLAG_INVALID`         | Consent flag must be `yes` or `no`                    |
| `E_REMOTE_DIAG_NOT_NEEDED`       | Remote diagnosis is not needed for this agent         |
| `E_DOWNLOAD_TOKEN_INVALID`       | Download token is invalid or expired                  |
| `E_DOWNLOAD_FILE_MISMATCH`       | Requested filename does not match token artifact      |
| `E_DOWNLOAD_NOT_FOUND`           | Artifact file is missing                              |
| `E_DISCORD_SIGNATURE_INVALID`    | Discord webhook signature verification failed         |
| `E_FEISHU_VERIFICATION_FAILED`   | Feishu event token verification failed                |
| `E_TELEGRAM_VERIFICATION_FAILED` | Telegram webhook secret verification failed           |
| `E_NOT_FOUND`                    | Route not found                                       |
| `E_COMMAND_UNSUPPORTED`          | Command exists in parser but has no handler           |
| `E_COMMAND_FAILED`               | Unexpected daemon/runtime error                       |

## Commands

### `/pair <code>`

Pairs a chat with the gateway using a one-time pairing code.

- **Args:** `code` (required)
- **Success:** `{ result: "ok", sessionToken: "<token>", message: "paired <provider>:<chat_id>" }`
- **Errors:** `E_USAGE`, `E_PAIR_CODE_INVALID`

### `/agents`

Lists all known agents and their install status.

- **Args:** none
- **Requires session:** yes
- **Success:** `{ result: "ok", message: "listed <n> agents (<m> installed)" }`

### `/install <agent_id> <host_id>`

Installs an agent.

- **Args:** `agent_id` (required), `host_id` (required when policy requires host binding; current default)
- **Requires session:** yes
- **Policy gate:** behavior is controlled by boundary policy `command_policies.chat_install`.
- **Current policy value:** `requires_host_binding`.
- **Success:** `{ result: "ok", message: "remote install completed for <agent_id> on host <host_id>" }`
- **Errors:** `E_USAGE`, `E_HOST_BINDING_REQUIRED`, `E_REMOTE_HOST_NOT_FOUND`, `E_REMOTE_INSTALL_FAILED`
- **Notes:** current remote chat install path supports `openclaw` only.

### `/onboard`

Starts onboarding.

- **Args:** none
- **Requires session:** yes
- **Policy gate:** behavior is controlled by boundary policy `command_policies.chat_onboard`.
- **Current policy value:** `disabled` (chat path intentionally blocked for credential safety).
- **Result:** `{ result: "error", errorCode: "E_ONBOARD_GUI_ONLY", message: "...Open Carrier GUI..." }`
- **Alternative flows:** `carrier onboard` (TUI) or `carrier onboard --webui`.

### `/boundaries`

Returns the current BaseAgent boundary summary generated from the executable policy spec.

- **Args:** none
- **Requires session:** yes
- **Success:** `{ result: "ok", message: "BaseAgent boundaries: ..." }`

### `/start <agent_id>`

Starts an installed agent.

- **Args:** `agent_id` (required)
- **Requires session:** yes
- **Success:** `{ result: "ok", message: "start completed for <agent_id>" }`
- **Errors:** `E_USAGE`, `E_NOT_INSTALLED`, `E_ALREADY_RUNNING`

### `/stop <agent_id>`

Stops a running agent.

- **Args:** `agent_id` (required)
- **Requires session:** yes
- **Success:** `{ result: "ok", message: "stop completed for <agent_id>" }`
- **Errors:** `E_USAGE`, `E_ALREADY_STOPPED`

### `/status [agent_id]`

Returns runtime status of one or all agents.

- **Args:** `agent_id` (optional)
- **Requires session:** yes
- **Success:** `{ result: "ok", message: "status <id>:<state>/<health>, ..." }` or `"no agent status available"`

### `/logs <agent_id> [tail]`

Returns recent log lines for an agent.

- **Args:** `agent_id` (required), `tail` (optional, default 200, must be positive integer)
- **Requires session:** yes
- **Success:** `{ result: "ok", message: "returned <n> log lines for <agent_id>" }`
- **Download:** `downloadUrl` included when logs are truncated or exceed 50 lines
- **Errors:** `E_USAGE`

### `/upgrade <agent_id>`

Upgrades an agent to the latest version.

- **Args:** `agent_id` (required)
- **Requires session:** yes
- **Success:** `{ result: "ok", message: "upgrade completed for <agent_id>: <from> -> <to>" }`
- **Errors:** `E_USAGE`, `E_COMMAND_FAILED`

### `/diagnose <agent_id>`

Generates a diagnostic artifact for an agent.

- **Args:** `agent_id` (required)
- **Requires session:** yes
- **Success:** `{ result: "ok", downloadUrl: "<url>", message: "diagnose artifact prepared for <agent_id>" }`
- **Errors:** `E_USAGE`, `E_COMMAND_FAILED`

### `/diagnose-consent <agent_id> <yes|no>`

Records consent for remote diagnosis and creates a handoff.

- **Args:** `agent_id` (required), consent `yes|y|true` or `no|n|false` (required)
- **Requires session:** yes
- **Success:** `{ result: "ok", handoffId: "<id>", handoffStatus: "pending"|"declined", message: "remote diagnosis consent recorded for <agent_id>" }`
- **Download:** `downloadUrl` included when artifact is available
- **Errors:** `E_USAGE`, `E_CONSENT_FLAG_INVALID`, `E_REMOTE_DIAG_NOT_NEEDED`

## Provider Isolation Principle

Provider adapters (Telegram, Discord, Feishu) MUST only handle:
- **Transport:** receiving messages, sending responses
- **Formatting:** converting `GatewayResponse` into platform-specific message format

Provider adapters MUST NOT:
- Alter command arguments before passing to `parseInput`
- Add provider-specific logic to command handling
- Return different response shapes per provider
- Filter or transform `GatewayResponse` fields based on provider

The `handleCommand` function is the single command-processing entry point and is provider-agnostic by design. The `provider` field in `GatewayCommand` is used only for session scoping (pairing is per-provider+chat), never for branching command logic.

## Feishu Rendering Notes

Feishu adapter rendering should preserve response semantics in text-message format:
- Success messages are prefixed with `✅`.
- Error messages are prefixed with `❌ <errorCode>:`.
- Optional fields (`downloadUrl`, `handoffId`) are appended as additional lines.

## Discord Rendering Notes

Discord adapter rendering should preserve response semantics while formatting for chat UX:
- Success messages are prefixed with `✅`.
- Error messages are prefixed with `❌ <errorCode>:`.
- Optional fields (`downloadUrl`, `handoffId`) are appended as additional lines.

## Telegram Rendering Notes

Telegram adapter rendering should preserve response semantics while formatting for chat UX:
- Success messages are prefixed with `✅`.
- Error messages are prefixed with `❌ <errorCode>:`.
- Optional fields (`sessionToken`, `downloadUrl`, `handoffId`) are appended as additional lines.

## Failure Parity Drift Reporting

Failure parity checks are implemented in gateway contract tests under `gateway/`.

- `assertFailureParity` validates that all provider responses for a failure scenario:
  - return `result: "error"`
  - share the same `errorCode`
  - share the same `message`
- Drift output includes provider + field-level expected/actual values for fast diagnosis.

These checks are exercised by `gateway/providers_test.go` and `gateway/server_test.go`.
