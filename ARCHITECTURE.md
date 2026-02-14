# Architecture

## Overview

Carrier is an agent lifecycle management system composed of two main components: a **daemon** (Go) that manages agents on the host, and a **gateway** (TypeScript/Bun) that provides a WebSocket API for remote control.

```
┌─────────────┐       WebSocket        ┌─────────────┐      HTTP/exec      ┌──────────┐
│   Client /   │ ◄──────────────────► │   Gateway    │ ◄────────────────► │  Daemon   │
│   Browser    │   (commands/events)   │  (Bun/TS)   │   (JSON over WS)   │ (agentd)  │
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

### Gateway (`gateway/`)

The gateway is a Bun/TypeScript WebSocket server that:

- Accepts client connections and routes commands to the daemon
- Manages **sessions** (`session/`) with authentication
- Issues **download tokens** (`downloads/`) for artifact retrieval
- Enforces **rate limiting** (`ratelimit/`)
- Translates between the client protocol and daemon API

### Command Contract (`gateway/src/contracts/`)

Typed command definitions shared between gateway and clients, defining the request/response schema for each operation (`/pair`, `/agents`, `/install`, `/start`, `/stop`, `/status`, `/logs`, `/upgrade`, `/diagnose`).

### Daemon API Contract (`docs/daemon-api-contract.md`)

Canonical endpoint/method matrix and error-envelope mapping for daemon HTTP APIs consumed by the gateway.

## Data Flow

1. Client connects to gateway via WebSocket
2. Client sends a command (e.g., `/install agent-name`)
3. Gateway validates session, applies rate limits, and forwards to daemon
4. Daemon executes the operation (install agent, run pre-flight checks, etc.)
5. Response flows back through gateway to client

## Key Design Decisions

- **Separation of concerns**: Gateway handles networking/auth; daemon handles host operations
- **Crash-loop protection**: Daemon tracks restart frequency and applies exponential cooldown
- **Audit trail**: All lifecycle operations are logged with bounded audit buffers
- **Redaction by default**: Sensitive environment variables and patterns are automatically redacted in diagnostics
