# Agent Installation and Distribution Platform — PRD (Phase 1 / Demo)

> Product form factor: desktop Daemon (Go), no mandatory GUI, controlled through Telegram/Discord/Feishu via Gateway (Go). Runtime is local-first (macOS/Linux host processes, Windows via WSL2). Memory model is Per-Agent, Shared, and Public.
> Runtime baseline authority: `docs/phase1-runtime-adr.md`.

---

## 0. Document Metadata
- Product codename: Agent Runtime Manager (Local)
- Version: PRD v0.2
- Owner modules: Daemon (Go), Gateway (Go), Memory Platform, Catalog
- This PRD is the **single source of truth** for product scope/priority. If another document conflicts, PRD wins.
- Runtime model changes must update this PRD and `docs/phase1-runtime-adr.md` in the same PR.

### 0.1 Terminology baseline

- Runtime = host (macOS/Linux) or WSL2 (Windows), no Docker path in Phase 1.
- Diagnose = sanitized artifact generation + optional remote diagnosis consent.
- Memory classes = Per-Agent / Shared / Public.
- Priority semantics = P0 (must-have), P1 (important next), P2 (later).

---

## 1. Objectives and Success Metrics

### 1.1 Phase 1 objectives (P0)
1. Users can pair via Telegram/Discord/Feishu and run core commands
2. End-to-end workflow is fully operational for OpenClaw
3. Memory lifecycle works with Per-Agent, Shared, and Public memory
4. Platform supports install/start/stop/status/logs/upgrade/diagnose for OpenClaw
5. Base Agent performs LLM-assisted failure analysis and repair attempts
6. If unresolved, platform generates diagnose bundle and asks for remote diagnosis consent

### 1.2 Success metrics
- Time-to-first-healthy (pair -> install -> start -> healthy) for OpenClaw: <= 10 minutes
- Diagnose coverage when unresolved by Base Agent: >= 95%
- Command path efficiency: install+start average <= 3 commands (including confirmations)
- P0 command success rate on supported environments: >= 90%

---

## 2. Supported Agents and Rollout

### 2.1 Candidate agents
- OpenClaw
- Pi Mono
- NanoClaw
- Pico Claw

### 2.2 Phase 1 implementation scope
- OpenClaw only for full lifecycle support
- Pi Mono, NanoClaw, Pico Claw remain catalog candidates until OpenClaw flow is stable

---

## 3. Scope

### 3.1 In scope (Phase 1)
- Agent lifecycle for OpenClaw:
  - install/start/stop/status/logs/upgrade/diagnose
- Runtime model:
  - macOS/Linux: local host runtime
  - Windows: WSL2 runtime
- Memory:
  - Per-Agent/Shared/Public management
  - import/export/duplicate/share/attach/detach
- Gateway:
  - Telegram + Discord + Feishu
  - pairing/session
  - read-only download endpoint
- Base Agent:
  - LLM-assisted analysis and bounded repair attempts
  - unresolved flow with diagnose bundle and remote diagnosis consent prompt

### 3.2 Out of scope (Phase 1)
- Docker-based runtime
- Full marketplace (ratings/payments/community)
- Cross-device memory sync
- Multi-role permission system
- Direct integration with remote diagnosis backend (consent + handoff placeholder only)

---

## 4. Functional Requirements

> Priority: P0 must / P1 should / P2 later

### 4.1 Daemon (Go) — Agent lifecycle

#### FR-D-001 List agents: `/agents` (P0)
Return fields:
- `id`, `name`, `candidate_status`, `installed`, `version`, `runtime_state`, `health`, `last_error`

Notes:
- OpenClaw can be `active`
- Pi Mono/NanoClaw/Pico Claw appear as `candidate`

#### FR-D-002 Install agent: `/install <agent>` (P0)
Input:
- `agent_id` in catalog allowlist

Steps:
1. validate runtime preconditions (OS-specific + toolchain)
2. parse manifest runtime install command
3. execute install command and persist install state
4. persist env template and memory attachment placeholders

Failure behavior:
- record `last_error`
- emit structured error code
- idempotent re-run allowed for repair-like retry

#### FR-D-003 Start agent: `/start <agent>` (P0)
Steps:
- verify installed
- validate required env vars
- validate required ports availability
- attach selected memory packages
- execute start command
- monitor startup via status and health probes

Failure behavior:
- collect process exit code, recent logs, health probe details
- invoke Base Agent analysis flow

#### FR-D-004 Stop agent: `/stop <agent>` (P0)
- execute stop command
- update runtime state

#### FR-D-005 Status: `/status <agent>` (P0)
Return:
- version, runtime state, health, ports, memory attachments, uptime, restart count

Health logic:
- health endpoint probe first when configured
- fallback to process state + restart-loop heuristics

#### FR-D-006 Logs: `/logs <agent> --tail N` (P0)
- default `N=200`
- merged stdout/stderr tail in chat
- downloadable artifact generated for long logs

#### FR-D-007 Upgrade: `/upgrade <agent>` (P0)
- run manifest upgrade strategy (`in_place_or_reinstall`)
- preserve env and memory attachments

P1 optional:
- rollback command path on failed upgrade

#### FR-D-008 Diagnose: `/diagnose <agent>` (P0)
Generate zip containing at least:
- manifest snapshot
- install/start command traces
- runtime logs (size-limited)
- process state and health probe outputs
- missing env keys (no secret values)
- daemon recent audit events
- host diagnostics (OS/version/cpu/memory/disk)

Output:
- artifact filename + read-only download method

### 4.2 Daemon (Go) — Runtime checks and repair

#### FR-D-010 Runtime prerequisite checks (P0)
macOS/Linux:
- required local runtime/toolchain presence by manifest type

Windows:
- WSL2 availability and required distro/runtime checks

Return:
- clear cause + error code + fix guidance

#### FR-D-011 Port conflict checks (P0)
- verify requested ports before start
- on conflict: structured error + recommended free port

#### FR-D-012 Env var validation (P0)
- parse `manifest.env.required`
- block start if missing
- never echo secret values

#### FR-D-013 Base Agent triage and repair loop (P0)
When install/start fails:
1. aggregate evidence (logs, exit code, state, health probes)
2. run Base Agent LLM-assisted analysis
3. apply safe, policy-bounded repair attempt(s)
4. return structured triage summary

Safety:
- risky operations require explicit user confirmation
- all actions are auditable

#### FR-D-014 Unresolved escalation flow (P0)
If Base Agent cannot resolve:
1. auto-generate diagnose bundle
2. prompt user: consent to remote diagnosis handoff
3. if consented: create handoff placeholder record (backend integration deferred)
4. if declined: return local artifact and next-step guidance

### 4.3 Memory Platform

#### FR-M-001 List memory (P0)
List packages with:
- id, name, version, type(`per_agent|shared|public`), owner, created_at, updated_at

#### FR-M-002 Import memory (P0)
- input: zip package
- validate `memory.yaml`
- install into local memory store + index DB

#### FR-M-003 Export memory (P0)
- input: `memory_id@version`
- output: downloadable zip artifact

#### FR-M-004 Duplicate memory (P0)
- duplicate from Public/Shared into Per-Agent memory namespace

#### FR-M-005 Share memory (P0)
- promote Per-Agent memory to Shared namespace

#### FR-M-006 Attach/detach memory (P0)
Per OpenClaw instance:
- 0..1 Per-Agent memory
- 0..N Shared memory
- 0..N Public memory

Mount policy:
- Shared default read-only
- Public read-only
- Per-Agent read-write

### 4.4 Gateway (Go)

#### FR-G-001 Pairing: `/pair <code>` (P0)
Flow:
1. daemon generates short-lived pairing code
2. user sends `/pair <code>` via Telegram/Discord/Feishu
3. gateway verifies with daemon
4. daemon binds `(provider, chat_id)` and returns session token

#### FR-G-002 Command forwarding and formatting (P0)
Supported commands:
- `/pair`
- `/agents`
- `/install <agent>`
- `/start <agent>`
- `/stop <agent>`
- `/status <agent>`
- `/logs <agent> --tail 200`
- `/upgrade <agent>`
- `/diagnose <agent>`

Requirements:
- stable structured output
- include `request_id`
- never echo secrets

#### FR-G-003 Read-only endpoint (P0)
Provide:
- `/health`
- `/downloads/<token>/<file>`

Token properties:
- short-lived
- one-time use recommended
- read-only scope

Default bind:
- `127.0.0.1`

#### FR-G-004 gateway runtime prerequisite checks (P1)
- validate required gateway webhook/auth configuration before enabling provider-specific ingress
- return actionable setup guidance when configuration is incomplete
- keep `/command` test path operational for local validation

---

## 5. Data and API Design

### 5.1 Daemon internal API (Gateway -> Daemon)
Recommended transport:
- local Unix socket + gRPC (or local HTTP+JSON)

Required auth:
- gateway-daemon local token (or mTLS)

Logical endpoints:
- `ListAgents()`
- `InstallAgent(id)`
- `StartAgent(id, memoryRefs)`
- `StopAgent(id)`
- `GetStatus(id)`
- `GetLogs(id, tail)`
- `UpgradeAgent(id)`
- `DiagnoseAgent(id)`
- `RunBaseAgentTriage(id)`
- `CreateRemoteDiagnosisHandoff(id, consent)`

### 5.2 Minimal data model
- `agents`: id, name, lifecycle_status, runtime_state, health, installed_version, last_error
- `memories`: id, version, type, owner, path, timestamps
- `agent_memory_attachments`: agent_id, memory_id, version, mode, priority
- `sessions`: provider, chat_id, session_token_hash, created_at, last_seen_at
- `audit_logs`: actor, action, target, result, timestamp, request_id
- `diagnosis_handoffs`: agent_id, consent, artifact_ref, created_at, status

---

## 6. Non-Functional Requirements

### 6.1 Security (P0)
- no secret echo
- diagnose artifacts sanitized
- pairing codes TTL-bound
- endpoint localhost-bound by default
- all Base Agent repairs policy-bounded and auditable

### 6.2 Reliability (P0)
- daemon runs as restartable service
- idempotent install/start behavior
- unresolved failures always produce either diagnose artifact or explicit failure reason

### 6.3 Performance (P1)
- `/agents` < 1s from local metadata
- `/status` < 1s with probe rate-limiting

### 6.4 Observability (P0)
- daemon logs with rotation
- per-agent log collection
- audit trail for lifecycle and repair operations

---

## 7. Acceptance Criteria

### 7.1 Phase 1 happy path (OpenClaw)
1. `/pair`
2. `/install openclaw`
3. `/start openclaw`
4. `/status openclaw` reports `healthy`
5. `/logs openclaw --tail 200` returns expected tail
6. `/diagnose openclaw` generates downloadable zip
7. `/stop openclaw`

Memory criteria:
- import a Public memory package
- duplicate into Per-Agent memory
- promote Per-Agent to Shared
- attach memories to OpenClaw and start successfully

### 7.2 Failure-path criteria
- missing runtime prerequisites: clear guidance + error code
- missing env vars: clear keys, no secret leakage
- port conflict: clear detection + recommendation
- unresolved Base Agent repair: diagnose bundle + remote diagnosis consent prompt

---

## 8. Milestones (sequence, no dates)
1. Define OpenClaw manifest + local runtime command contract
2. Implement daemon core lifecycle + runtime checks
3. Implement Base Agent triage + diagnose escalation flow
4. Implement gateway (Telegram/Discord/Feishu) + pairing + command forwarding
5. Implement memory lifecycle (Per-Agent/Shared/Public)
6. Stabilize OpenClaw end-to-end and keep Pi Mono/NanoClaw/Pico Claw as candidates

---

## 9. Terminology Reference
- **Runtime**: the execution environment for agent processes (host on macOS/Linux, WSL2 on Windows).
- **Diagnose**: generate a sanitized diagnostics artifact (logs/state/probe outputs without secret values).
- **Per-Agent memory**: memory scoped to one agent instance.
- **Shared memory**: reusable memory across local agents (default read-only mount).
- **Public memory**: template or community-provided memory package (read-only).
- **P0 / P1 / P2**:
  - **P0** = must-have for Phase 1 acceptance
  - **P1** = should-have after P0 stability
  - **P2** = later backlog

## 10. Open Questions
1. Preferred runtime package format per OS for OpenClaw (`tar.gz`, installer script, package manager)
2. WSL2 distro and shell assumptions for Windows support baseline
3. Remote diagnosis handoff schema and privacy policy contract
