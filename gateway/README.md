# Gateway Contributor Guide

Source issues: #651 #653

## `CARRIER_GATEWAY_PORT` Parsing Rules

Gateway port parsing is strict and uses fallback behavior for invalid inputs.

- default port: `8787`
- accepted format: integer string in range `1..65535`
- invalid values fallback to default

### Valid examples

- `8787` -> `8787`
- `3000` -> `3000`

### Invalid examples (fallback to `8787`)

- `8080.5`
- `8080abc`
- empty string
- `0`
- `65536`

Reference implementation/tests:

- `gateway/src/server.ts`
- `gateway/src/server.test.ts`

## Command-Normalization Fixture Format

Use the fixture format below when adding cross-provider command-normalization tests.

```json
{
  "provider": "telegram|discord|feishu",
  "input": {
    "raw": "<provider> <chat_id> <request_id> <command> [...args]"
  },
  "normalized": {
    "provider": "telegram",
    "chatId": "100",
    "requestId": "req-1",
    "name": "/status",
    "args": ["openclaw"]
  }
}
```

### Telegram example

```json
{
  "provider": "telegram",
  "input": {
    "raw": "telegram 100 req-tg /logs openclaw 200"
  },
  "normalized": {
    "provider": "telegram",
    "chatId": "100",
    "requestId": "req-tg",
    "name": "/logs",
    "args": ["openclaw", "200"]
  }
}
```

### Discord example

```json
{
  "provider": "discord",
  "input": {
    "raw": "discord guild-42 req-dc /install openclaw"
  },
  "normalized": {
    "provider": "discord",
    "chatId": "guild-42",
    "requestId": "req-dc",
    "name": "/install",
    "args": ["openclaw"]
  }
}
```

### Feishu example

```json
{
  "provider": "feishu",
  "input": {
    "raw": "feishu chat-9 req-fs /pair code-123"
  },
  "normalized": {
    "provider": "feishu",
    "chatId": "chat-9",
    "requestId": "req-fs",
    "name": "/pair",
    "args": ["code-123"]
  }
}
```

## How to Add a New Fixture

1. Add the raw provider input and expected normalized command shape.
2. Add/extend assertions in `gateway/src/cross-provider.test.ts`.
3. Add parser-routing edge cases in `gateway/src/command-routing.test.ts` when needed.
4. Run `cd gateway && bun test`.
