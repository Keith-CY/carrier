# Architecture

## Overview

Carrier is an agent lifecycle management system split into dedicated top-level modules:

- **webui**: local visual operations UI
- **gateway**: ingress/message bus and API aggregation
- **daemon**: host lifecycle/runtime scheduler
- **baseagent**: reusable base-agent policies/runtime
- **shared**: cross-module shared data/logic (`config`, `redact`)

Dependency direction:

```
webui -> gateway -> daemon -> shared
webui -> gateway -> baseagent -> shared
```

```
┌─────────────┐         HTTP           ┌─────────────┐        HTTP         ┌──────────┐
│ Chat/Webhook │ ───────────────────► │   Gateway    │ ─────────────────► │  Daemon   │
│   Clients    │   (commands/events)   │    (Go)      │   (JSON over API)  │ (carrier daemon) │
└─────────────┘                        └─────────────┘                     └──────────┘
                                                                                │
                                                                          ┌─────┴─────┐
                                                                          │  Agents    │
                                                                          │ (managed   │
                                                                          │ processes) │
                                                                          └───────────┘
```

## Components

### Daemon (`daemon/`)

The daemon (`carrier daemon`) is the host-side process responsible for:

- **Agent Lifecycle** (`internal/lifecycle/`) — Install, start, stop, upgrade agents with crash-loop detection and automatic cooldown
- **Catalog** (`internal/catalog/`) — Registry of available agent definitions
- **Command Execution** (`internal/commandexec/`) — Sandboxed shell command runner with validation
- **Health Checks** (`internal/health/`) — HTTP health endpoint exposing agent status
- **Logging** (`internal/logging/`) — Structured logging with context propagation (agent name, request ID, operation)
- **Manifest** (`internal/manifest/`) — Agent manifest schema (TOML) for declaring agent requirements
- **Memory** (`internal/memory/`) — Per-agent and shared memory store with state transitions
- **Runtime Checks** (`internal/runtimecheck/`) — Pre-flight validation (env vars, ports, dependencies)

### Gateway (`gateway/`)

The gateway is a top-level Go module that:

- Accepts command requests (`/command`) and provider webhooks (`/webhook/{telegram|discord|feishu}`)
- Manages **sessions** (`session.go`) with authentication
- Issues **download tokens** (`downloads.go`) for artifact retrieval
- Enforces **rate limiting** (`ratelimit.go`)
- Translates between the client protocol and daemon API

### Base Agent (`baseagent/`)

Base-agent logic is isolated from daemon runtime specifics:

- Chat action dispatch (`runtime.go`)
- LLM-based failure triage (`triager_llm.go`)
- Repair policy controls (`policy.go`)
- Shared-model configuration/redaction consumption via `shared/`

### Shared (`shared/`)

Cross-module shared code:

- `shared/config`: runtime config and default-model config loading
- `shared/redact`: sensitive text/env redaction logic

### Command Contract (`docs/command-contract.md`)

Command definitions used by gateway command parsing and response rendering, covering operations such as `/pair`, `/agents`, `/install`, `/start`, `/stop`, `/status`, `/logs`, `/upgrade`, `/diagnose`.

### Daemon API Contract (`docs/daemon-api-contract.md`)

Canonical endpoint/method matrix and error-envelope mapping for daemon HTTP APIs consumed by the gateway.

## Data Flow

1. Client sends command/webhook payload to gateway over HTTP
2. Client sends a command (e.g., `/install agent-name`)
3. Gateway validates session, applies rate limits, and forwards to daemon
4. Daemon executes the operation (install agent, run pre-flight checks, etc.)
5. Response flows back through gateway to client

## Key Design Decisions

- **Separation of concerns**: Gateway handles transport/session/auth; daemon handles host operations
- **Crash-loop protection**: Daemon tracks restart frequency and applies exponential cooldown
- **Audit trail**: All lifecycle operations are logged with bounded audit buffers
- **Redaction by default**: Sensitive environment variables and patterns are automatically redacted in diagnostics
