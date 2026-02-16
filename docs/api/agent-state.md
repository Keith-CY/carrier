# AgentState Contract

> Canonical JSON shape returned by the daemon API for agent state.

## JSON Schema

```jsonc
{
  "id":                   "string",           // unique agent identifier
  "name":                 "string",           // human-readable agent name
  "version":              "string",           // semver version string (e.g. "1.2.3")
  "installState":         "string",           // installation state (see below)
  "runtimeState":         "string",           // runtime state (see below)
  "health":               "string",           // health state (see below)
  "lastError":            "string | omitted", // last error message (omitted when empty)
  "lastTriageSummary":    "string | omitted", // last triage summary (omitted when empty)
  "needsRemoteDiagnosis": "boolean",          // whether remote diagnosis is requested
  "lastDiagnoseFile":     "string | omitted", // path to last diagnosis artifact (omitted when empty)
  "updatedAt":            "string"            // ISO-8601 datetime (e.g. "2026-01-15T08:30:00.000Z")
}
```

## Field Reference

### `installState`

| Value             | Description                    |
| ----------------- | ------------------------------ |
| `not_installed`   | Agent has not been installed   |
| `installed`       | Agent is installed and usable  |
| `broken`          | Installation is corrupt        |

### `runtimeState`

| Value         | Description                          |
| ------------- | ------------------------------------ |
| `stopped`     | Agent is not running                 |
| `starting`    | Agent is in the process of starting  |
| `running`     | Agent is running normally            |
| `crashing`    | Agent is crashing                    |
| `crash_loop`  | Agent is stuck in a crash loop       |

### `health`

| Value       | Description               |
| ----------- | ------------------------- |
| `unknown`   | Health has not been evaluated |
| `healthy`   | Agent is healthy          |
| `degraded`  | Agent is partially degraded |
| `unhealthy` | Agent is unhealthy        |

## Deprecated Fields

### `installed` (boolean)

**Deprecated since PR #958.** The legacy `installed` boolean field is still
emitted by the daemon's list-agents endpoint for backward compatibility but
**must not be used by new consumers**. Use `installState` instead.

Migration mapping:

| Legacy `installed` | Equivalent `installState` |
| ------------------ | ------------------------- |
| `true`             | `"installed"`             |
| `false`            | `"not_installed"`         |

> **Note:** `installState` also expresses the `"broken"` state, which had no
> representation in the legacy boolean field.

## Source Definitions

- **Go (daemon):** `daemon/internal/lifecycle/types.go` — `AgentState` struct
- **TypeScript (gateway):** `gateway/src/daemon/client.ts` — `DaemonAgentState` type
