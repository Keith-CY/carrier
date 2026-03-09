# Architecture

## Overview

Carrier is split into four layers with two product cores:

- **Execution Plane**: goal decomposition, templates, triggers, executions, artifacts, evidence, observability
- **Knowledge Plane**: memory packages, attachments, curated search, instance distill, base-agent promotion inputs
- **Runtime Substrate**: local agent lifecycle, remote hosts, instances, workers, isolation
- **Transport Adapters**: CLI, WebUI, chat/webhook entrypoints

The top-level modules map onto those layers:

- **webui**: local visual operations UI
- **gateway**: control-plane ingress and API aggregation
- **daemon**: host lifecycle, memory backend, runtime scheduler
- **baseagent**: reusable base-agent policies, decomposition, distill authority
- **shared**: cross-module shared data/logic (`config`, `redact`)

Dependency direction:

```
webui -> gateway -> daemon -> shared
webui -> gateway -> baseagent -> shared
```

```
┌─────────────┐         HTTP           ┌─────────────┐        HTTP         ┌──────────┐
│ CLI / WebUI / │ ───────────────────► │   Gateway    │ ─────────────────► │  Daemon   │
│ Chat / Hooks  │   (control-plane)     │    (Go)      │   (JSON over API)  │ (carrier daemon) │
└─────────────┘                        └─────────────┘                     └──────────┘
                                                                                │
                                                                          ┌─────┴──────┐
                                                                          │ Workers +   │
                                                                          │ Memory      │
                                                                          │ Backend     │
                                                                          └────────────┘
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
- **Memory** (`internal/memory/`) — Knowledge-plane backend for packages, attachments, curated search, grants, audit, import/export, and distillation
- **Runtime Checks** (`internal/runtimecheck/`) — Pre-flight validation (env vars, ports, dependencies)

### Gateway (`gateway/`)

The gateway is a top-level Go module that:

- Accepts command requests (`/command`) and provider webhooks (`/webhook/{telegram|discord|feishu}`)
- Manages **sessions** (`session.go`) with authentication
- Issues **download tokens** (`downloads.go`) for artifact retrieval
- Enforces **rate limiting** (`ratelimit.go`)
- Hosts the public control-plane APIs for executions, templates, triggers, governance, workers, and gateway-facing memory operations (`/api/v1/memory/*`)
- Translates between the client protocol and daemon API

### Base Agent (`baseagent/`)

Base-agent logic is isolated from daemon runtime specifics:

- Chat action dispatch (`runtime.go`)
- LLM-based failure triage (`triager_llm.go`)
- Repair policy controls (`policy.go`)
- Distillation authority for promoting execution learnings back into long-lived knowledge
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

1. Client launches an execution or memory action through CLI/WebUI/chat/webhook
2. Gateway validates authz, policy, provider bindings, and memory intent
3. Gateway forwards runtime work to daemon and records execution / audit / evidence metadata
4. Daemon executes worker lifecycle or memory backend operations
5. Execution results and distill outputs flow back through gateway to the base agent and user-facing surfaces

## Key Design Decisions

- **Dual control plane**: execution and knowledge are both first-class product objects
- **Separation of concerns**: Gateway handles control-plane ingress/governance; daemon handles host runtime and memory backend work
- **Crash-loop protection**: Daemon tracks restart frequency and applies exponential cooldown
- **Audit trail**: All lifecycle operations are logged with bounded audit buffers
- **Redaction by default**: Sensitive environment variables and patterns are automatically redacted in diagnostics
