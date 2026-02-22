# Architecture

## Overview

Carrier is an agent lifecycle management system composed of two main components: a **daemon** (Go) that manages agents on the host, and a **gateway** (Go) that provides HTTP command/webhook ingress for remote control.

```
┌─────────────┐         HTTP           ┌─────────────┐        HTTP         ┌──────────┐
│ Chat/Webhook │ ───────────────────► │   Gateway    │ ─────────────────► │  Daemon   │
│   Clients    │   (commands/events)   │    (Go)      │   (JSON over API)  │ (agentd)  │
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

The daemon (`agentd`) is the host-side process responsible for:

- **Agent Lifecycle** (`internal/lifecycle/`) — Install, start, stop, upgrade agents with crash-loop detection and automatic cooldown
- **Catalog** (`internal/catalog/`) — Registry of available agent definitions
- **Command Execution** (`internal/commandexec/`) — Sandboxed shell command runner with validation
- **Configuration** (`internal/config/`) — Runtime configuration loading
- **Health Checks** (`internal/health/`) — HTTP health endpoint exposing agent status
- **Logging** (`internal/logging/`) — Structured logging with context propagation (agent name, request ID, operation)
- **Manifest** (`internal/manifest/`) — Agent manifest schema (TOML) for declaring agent requirements
- **Memory** (`internal/memory/`) — Per-agent and shared memory store with state transitions
- **Redaction** (`internal/redact/`) — Sensitive data redaction for logs and diagnostics
- **Runtime Checks** (`internal/runtimecheck/`) — Pre-flight validation (env vars, ports, dependencies)
- **Base Agent** (`internal/baseagent/`) — Repair action policies and risk classification

### Gateway (`daemon/internal/gateway/`)

The gateway is a Go HTTP server that:

- Accepts command requests (`/command`) and provider webhooks (`/webhook/{telegram|discord|feishu}`)
- Manages **sessions** (`session.go`) with authentication
- Issues **download tokens** (`downloads.go`) for artifact retrieval
- Enforces **rate limiting** (`ratelimit.go`)
- Translates between the client protocol and daemon API

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
