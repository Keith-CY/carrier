# Gateway Command Contract

This folder defines the normalized command and response schema used by all chat providers.

## Commands

`GatewayCommand` is a discriminated union keyed by `name` with strict typed payloads in `data`.

Supported P0 commands:

- `/pair` -> `PairCommandData` (`code`)
- `/agents` -> `AgentsCommandData` (`filter?`, `includeInstalled?`)
- `/install` -> `InstallCommandData` (`agent`, `version?`, `source?`)
- `/start` -> `StartCommandData` (`agent`, `profile?`)
- `/stop` -> `StopCommandData` (`agent`)
- `/status` -> `StatusCommandData` (`agent?`, `verbose?`)
- `/logs` -> `LogsCommandData` (`agent`, `tail?`, `since?`)
- `/upgrade` -> `UpgradeCommandData` (`agent?`, `version?`)
- `/diagnose` -> `DiagnoseCommandData` (`scope?`, `agent?`)

Common command fields:

- `provider`: `telegram | discord | feishu`
- `chatId`: provider-native chat/thread/conversation id
- `requestId`: gateway-assigned request correlation id
- `name`: command name
- `data`: command-specific parsed payload

## Responses

All command responses must use `ResponseEnvelope<TData>` from `response.ts`.

Mandatory fields on every response:

- `requestId`
- `status` (`ok | error`)
- `data` (`TData | null`)
- `errorCode` (`string | null`)
- `errorMessage` (`string | null`)

Rules:

- For `status: "ok"`, `errorCode` and `errorMessage` must be `null`.
- For `status: "error"`, `data` must be `null`.
- `requestId` must match the originating command `requestId`.

## Known Provider Deviations

- Telegram:
  - Supports slash commands natively.
  - Command arguments arrive as a flat token string and must be parsed into `data`.
  - Chat context may be group or direct message; both map to `chatId`.
- Discord:
  - Slash command interactions may provide typed options; these are normalized into the same `data` shape.
  - Follow-up message updates are asynchronous and may arrive after gateway response emission.
- Feishu:
  - Bot payloads can vary by event type and may include command text in nested fields.
  - Some events do not include a direct slash command token; command intent is inferred before mapping to `GatewayCommand`.
