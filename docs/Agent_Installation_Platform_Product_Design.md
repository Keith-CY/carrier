# Agent Installation and Distribution Platform (Daemon + Gateway + Memory Platform) — Product Design

> One-line pitch: A desktop Daemon (Go) that installs, runs, repairs, and monitors local Agents, controlled through Telegram/Discord/Feishu via a Gateway (TypeScript), with a unified Memory Platform based on Per-Agent, Shared, and Public memory.

---

## 1. Context and Problem

### 1.1 User pain points
Running Agents on desktop environments is fragmented and failure-prone:
- Dependency sprawl (Node, Go, Python, system libraries, env vars, ports)
- Inconsistent install and run procedures between Agents
- Startup/runtime failures are hard to debug (scattered logs, weak diagnosis tooling)
- Memory is hard to reuse across Agents in a controlled, versioned way

### 1.2 Target users
- Desktop users (developers, indie hackers, internal evaluators)
- Non-technical users blocked by complex install and runtime dependencies

### 1.3 Key usage scenarios
- Fresh machine: install platform -> pair Telegram/Discord/Feishu -> install OpenClaw -> start -> verify status
- Train memory in OpenClaw -> attach it to another Agent later
- Agent fails -> platform attempts auto-repair through Base Agent
- Base Agent cannot resolve -> generate diagnose package and ask user for remote diagnosis consent

---

## 2. Vision and Principles

### 2.1 North Star
- Within 10 minutes: from fresh machine to healthy OpenClaw runtime + paired chat control
- Unified lifecycle management:
  - Agent lifecycle: install/start/stop/upgrade/health/logs/diagnose
  - Memory lifecycle: import/export/duplicate/share/attach/detach
- Repair-first operation: platform attempts automated triage and guided fixes before escalating

### 2.2 Design principles
1. Daemon is the security boundary:
   - gateway = adapter + session + minimal authz
   - daemon = security boundary + policy + execution
2. Native runtime first:
   - macOS/Linux: local runtime on host
   - Windows: local runtime inside WSL2
   - Docker is out of scope for this version
3. No mandatory GUI:
   - primary UX is Telegram/Discord/Feishu commands
   - CLI is optional
4. Diagnosability before deep automation:
   - always produce actionable evidence
5. Memory is infrastructure:
   - versioned, shareable, attachable, auditable

---

## 3. Core Concepts

| Term | Definition |
|---|---|
| Agent | Installable and runnable agent application distributed through catalog + manifest |
| Base Agent | Internal Daemon capability that uses LLM-assisted analysis for installation/runtime issues |
| Daemon (Go) | Resident service handling policy, execution, state, storage, and runtime control |
| Gateway (TS) | Telegram/Discord/Feishu adapter + session layer + read-only download endpoint |
| Manifest | Single source of truth for install, run, health, and memory requirements |
| Per-Agent Memory | Memory owned by one Agent instance |
| Shared Memory | Memory explicitly shared by user across local Agents |
| Public Memory | Platform/community memory package templates |

---

## 4. Architecture Overview

### 4.1 Component boundaries
- Daemon (Go): policy + execution + health + repair + state
- Base Agent (inside Daemon): LLM-assisted troubleshooting and fix recommendation/execution policy
- Gateway (TypeScript): Telegram/Discord/Feishu command adapter and session layer
- Memory Platform (local): package and lifecycle management for Per-Agent/Shared/Public memory

### 4.2 Diagram
```mermaid
flowchart LR
  TG[Telegram Bot] --> GW[Gateway (TypeScript)]
  DC[Discord Bot] --> GW
  FSK[Feishu Bot] --> GW
  GW -->|local RPC| D[Daemon (Go)]
  GW -->|read-only HTTP| HTTP[(Hidden Download Endpoint)]
  D --> BA[Base Agent (LLM-assisted diagnostics)]
  D --> FS[(Local FS: agents/memories/logs)]
  D --> DB[(Local DB: metadata/state)]
  FS --> A1[OpenClaw Runtime]
```

### 4.3 Key runtime flow
1. User sends command through Telegram/Discord/Feishu
2. Gateway validates pairing/session and forwards request to Daemon
3. Daemon executes install/start/stop/status/logs/upgrade actions against local runtime
4. On failure, Base Agent analyzes signals and attempts guided repair
5. If unresolved, Daemon creates diagnose bundle and asks for remote diagnosis consent
6. Short responses return in chat; large artifacts are delivered via read-only endpoint or attachment

---

## 5. Runtime and Installation Model

### 5.1 Runtime strategy
- macOS/Linux:
  - run Agents as local processes/services on host
- Windows:
  - run Agents inside WSL2
- No Docker dependency in Phase 1

### 5.2 Installer model
Phase 1 installation methods supported by manifest runtime type:
- `local_binary`: download artifact and run
- `npm_cli`: install and run via npm-compatible command pipeline
- `go_cli`: install and run via Go toolchain command pipeline

Daemon responsibilities:
- detect required runtime/toolchain per manifest
- validate permissions, network, disk, and required environment variables
- materialize install state and command templates

---

## 6. Distribution and Manifest

### 6.1 Catalog
Candidate Agents:
- OpenClaw
- Pi Mono
- NanoClaw
- Pico Claw

Phase 1 implementation target:
- Full lifecycle support for OpenClaw only
- Other agents remain catalog candidates until OpenClaw workflow is stable

### 6.2 Manifest (YAML) suggested fields
```yaml
id: "openclaw"
name: "OpenClaw"
version: "1.0.0"

runtime:
  type: "local_binary"   # local_binary | npm_cli | go_cli
  install:
    command: "./install.sh"
  start:
    command: "./openclaw --config ./config.yaml"
  stop:
    command: "./openclaw --stop"

network:
  ports:
    - name: "http"
      port: 8080
  healthcheck:
    type: "http"
    url: "http://localhost:8080/health"

env:
  required:
    - name: "OPENAI_API_KEY"
      secret: true
  optional:
    - name: "LOG_LEVEL"
      default: "info"

memory:
  supports:
    - type: "per_agent"
    - type: "shared"
    - type: "public"
  mount_path: "./memory"

upgrade:
  channel: "stable"
  strategy: "in_place_or_reinstall"

diagnostics:
  include:
    - "runtime_logs"
    - "process_state"
    - "env_sanitized"
```

### 6.3 Design requirements
- Manifest is the single source of truth
- No per-agent hardcoding in Daemon
- Secure defaults for process execution, file access, and network exposure
- Upgrade preserves config and memory attachments

---

## 7. Memory Platform Design

### 7.1 Memory classes
1. Per-Agent memory:
   - private memory bound to a single agent context
2. Shared memory:
   - user-authorized memory reusable across local agents
3. Public memory:
   - platform/community templates installable by users

### 7.2 Suggested storage layout
```text
~/.agentd/memories/
  public/
    persona-product-manager@1.2.0/
      memory.yaml
      system_prompt.md
      traits.json
  shared/
    teamwork-style@0.2.0/
      memory.yaml
      notes.md
  per-agent/
    openclaw/user-default@0.1.0/
      memory.yaml
      notes.md
```

### 7.3 Lifecycle
- Import: zip -> validate metadata -> unpack -> index in DB
- Export: package selected memory as zip
- Duplicate: copy from Public/Shared to Per-Agent
- Share: promote Per-Agent memory into Shared
- Attach/Detach: manage memory links per agent
- Mount: Daemon mounts selected memory set during start

### 7.4 Concurrency policy
- Shared memory defaults to read-only when attached
- Writable shared memory requires explicit user authorization

---

## 8. Daemon and Base Agent Design

### 8.1 Daemon responsibilities
- Install/start/stop/upgrade/status/logs/diagnose
- Policy enforcement (allowlist, resource limits, sensitive-action confirmation)
- State persistence and health monitoring
- Audit trail for all privileged actions

### 8.2 Base Agent responsibilities (inside Daemon)
- Analyze failures using LLM-assisted reasoning over runtime evidence
- Propose and execute safe repair actions (bounded by policy)
- Produce user-readable explanation and next-step suggestions
- Escalate when unresolved:
  - generate diagnose bundle
  - ask user whether to proceed with remote diagnosis handoff (future integration)

### 8.3 Status model
- install: `not_installed | installed | broken`
- runtime: `stopped | starting | running | crashing`
- health: `unknown | healthy | degraded | unhealthy`

Signals:
- process lifecycle state
- health endpoint checks
- port checks and restart-loop detection
- install/runtime command exit codes

---

## 9. Gateway Design

### 9.1 Responsibilities
- Integrate Telegram Bot API, Discord Bot, and Feishu Bot
- Pairing/session management
- Command parsing and forwarding to Daemon
- Read-only endpoint for large logs and diagnose bundles

### 9.2 bun dependency behavior
- detect bun availability/version on startup
- if missing, request user confirmation before install into gateway private directory
- if user declines, continue core adapter capabilities when possible

### 9.3 Read-only endpoint
- bind to `127.0.0.1` by default
- short-lived one-time token URLs
- GET/HEAD only

Suggested endpoints:
- `/health`
- `/agents`
- `/agents/<id>/logs?tail=200`
- `/downloads/<token>/<file>`

---

## 10. Commands

Must support:
- `/pair`
- `/agents`
- `/install <agent>`
- `/start <agent>` / `/stop <agent>`
- `/status <agent>`
- `/logs <agent> --tail 200`
- `/upgrade <agent>`
- `/diagnose <agent>`

Interaction rules:
- confirmation required for install/upgrade/other sensitive operations
- structured, mobile-friendly outputs
- long logs default to tail with download link/attachment

---

## 11. Security, Permissions, and Audit

### 11.1 Pairing
- Daemon generates short-lived pairing code
- User sends `/pair <code>` in Telegram/Discord/Feishu
- Gateway verifies with Daemon and receives session token
- Gateway maintains chat-id allowlist mapped to session

### 11.2 Policy baseline
- Agent and source allowlist
- resource limits and local safety checks
- secret handling: never echo values; store in keychain/encrypted local file

### 11.3 Auditing
Daemon records:
- actor (provider + chat_id)
- timestamp
- target agent
- action
- result and error code
- diagnose artifact pointer (if generated)

---

## 12. Phase 1 Scope

### 12.1 Must deliver
Daemon:
- local runtime detection for macOS/Linux and WSL2 checks for Windows
- manifest-driven lifecycle for OpenClaw:
  - install/start/stop/status/logs/upgrade/diagnose
- memory management with Per-Agent/Shared/Public support
- health monitoring and Base Agent repair loop

Gateway:
- Telegram/Discord/Feishu integration
- pairing/session
- read-only endpoint for logs and diagnose bundles

Catalog:
- candidate definitions for OpenClaw, Pi Mono, NanoClaw, Pico Claw
- OpenClaw fully operational in Phase 1

### 12.2 Non-goals
- Full marketplace (ratings/payments/community)
- Cross-device memory sync
- Complex multi-role permission system
- Production remote diagnosis workflow integration (only consent prompt and handoff placeholder)

---

## 13. Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Host runtime differences across OS | setup instability | explicit OS checks + clear repair guidance |
| Missing local toolchains | install/start failure | manifest requirement checks + Base Agent triage |
| Shared memory write conflicts | memory corruption | default read-only shared mounts |
| LLM repair overreach | unsafe changes | policy-bounded repair actions + confirmation for risky actions |
| Unresolved failures | poor UX | diagnose zip + explicit remote diagnosis consent prompt |

---

## 14. Roadmap
- Phase 1: OpenClaw end-to-end on local runtime + WSL2 + 3 gateways
- Phase 2: onboard Pi Mono/NanoClaw/Pico Claw after OpenClaw stability gate
- Phase 3: remote catalog updates, remote diagnosis integration, multi-device sync
