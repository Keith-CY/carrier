# PicoClaw Single-Agent Parity Design

**Scope:** Close the next single-agent parity gaps against upstream PicoClaw without expanding multi-channel managed onboarding. The focus is:

1. PicoClaw agent-native tools / skills / MCP / runtime parity
2. media / voice / file / rich message parity
3. provider / model direct-surface parity
4. launcher / cron / heartbeat / standalone UX parity

## Goal

Make Carrier capable of managing PicoClaw as a first-class single-agent runtime with upstream-like capability breadth, while keeping Carrier as the execution and knowledge control plane authority.

This is not a goal to replicate PicoClaw 1:1 as an opaque product. Carrier should own governance, memory, policy, audit, evidence, and remote lifecycle, while PicoClaw-facing runtime surfaces become richer and more direct.

## Non-Goals

- Multi-channel managed parity. Telegram-only managed flow may remain the only canonical channel during this phase.
- Reproducing PicoClaw's exact memory storage layout or launcher internals.
- Replacing Carrier orchestration with PicoClaw-native workflows.
- Hosted SaaS or public marketplace work.

## Current Gap Summary

### 1. Agent-native runtime/tools parity is partial

Carrier baseagent now has a structured tool surface, local skills registry, managed MCP abstraction, and in-memory subagent manager:

- [execution_tools.go](/Users/ChenYu/Documents/Github/carrier/baseagent/execution_tools.go)
- [structured_tool_surface.go](/Users/ChenYu/Documents/Github/carrier/baseagent/structured_tool_surface.go)
- [skills_registry.go](/Users/ChenYu/Documents/Github/carrier/baseagent/skills_registry.go)
- [mcp_manager.go](/Users/ChenYu/Documents/Github/carrier/baseagent/mcp_manager.go)
- [subagent_manager.go](/Users/ChenYu/Documents/Github/carrier/baseagent/subagent_manager.go)

But the surface is still Carrier-first and backend-heavy:

- no agent-native rich attachment result contract
- no browser/mobile-operation abstraction
- no persistent delegated-job lifecycle beyond in-memory
- no skill discovery / install flow aligned with upstream PicoClaw ergonomics
- MCP is managed, but still not presented as a rich runtime capability system

### 2. Media / voice / rich message parity is missing as a first-class contract

Inbound/outbound bus envelopes are still text-only:

- [controlplane_bus.go](/Users/ChenYu/Documents/Github/carrier/baseagent/controlplane_bus.go)

Gateway transports are also text-first:

- [telegram_transport.go](/Users/ChenYu/Documents/Github/carrier/gateway/telegram_transport.go)

Carrier can export execution artifacts and manage memory attachments, but it does not yet expose:

- inbound attachment normalization
- outbound rich message blocks
- voice/audio transcription or speech response contracts
- file send/receive semantics that work across baseagent, gateway, and execution evidence

### 3. Provider/model direct-surface parity is narrower than upstream PicoClaw

Carrier's provider governance is stronger than PicoClaw's, but the direct-surface agent ergonomics are narrower:

- canonical provider catalog is intentionally small: [catalog.go](/Users/ChenYu/Documents/Github/carrier/shared/catalog/catalog.go)
- managed PicoClaw rendering is provider-collapsing and gateway-mediated: [managed_onboard.go](/Users/ChenYu/Documents/Github/carrier/gateway/managed_onboard.go), [picoclaw_onboard.go](/Users/ChenYu/Documents/Github/carrier/gateway/picoclaw_onboard.go)

Compared with upstream PicoClaw's `model_list` and protocol-first provider direction, Carrier still lacks:

- protocol-class provider abstraction at the managed PicoClaw surface
- agent-local multi-model lists and same-model fanout/load balancing
- direct agent-side model aliasing / fallback / timeout ergonomics

### 4. Launcher / standalone UX parity is fragmented

Carrier has:

- TUI onboarding
- embedded WebUI
- Tauri app shell
- baseagent cron scheduling

Relevant files:

- [main.go](/Users/ChenYu/Documents/Github/carrier/cmd/carrier/main.go)
- [server.go](/Users/ChenYu/Documents/Github/carrier/daemon/server/server.go)
- [cron_service.go](/Users/ChenYu/Documents/Github/carrier/baseagent/cron_service.go)
- [tauri.conf.json](/Users/ChenYu/Documents/Github/carrier/src-tauri/tauri.conf.json)

But it does not yet offer a coherent "managed PicoClaw standalone surface" equivalent to upstream's:

- `picoclaw agent -m`
- launcher / web console
- heartbeat-centric local runtime UX
- cron/reminder management as a user-facing feature

## Approach Options

### Option A: Opaque upstream wrapper

Treat PicoClaw as an external black-box binary and only manage install/start/stop/config.

Pros:
- fast initial parity optics
- low baseagent churn

Cons:
- poor policy/memory/evidence integration
- rich media and direct-surface features stay outside Carrier's contracts
- hard to unify with execution and knowledge planes

### Option B: Full PicoClaw behavior clone inside Carrier

Rebuild PicoClaw-native product behavior entirely inside Carrier.

Pros:
- maximum product consistency

Cons:
- wasteful duplication
- high regression risk
- likely to fork away from upstream behavior too quickly

### Option C: Hybrid parity layer inside Carrier

Keep Carrier as the authority, but introduce an explicit "agent-native surface" layer for PicoClaw-class runtimes: richer tool contracts, content blocks, provider/model profiles, launcher UX, heartbeat, and cron.

Pros:
- preserves Carrier governance and memory architecture
- closes practical parity gaps without duplicating everything
- creates reusable contracts for both PicoClaw and ZeroClaw

Cons:
- requires cross-cutting refactor across baseagent, gateway, daemon, WebUI, and CLI

**Recommendation:** Option C.

## Recommended Architecture

Add a new logical layer under Carrier:

- **Agent-Native Surface**
  - runtime tool contracts
  - content/media blocks
  - provider/model direct-surface profiles
  - launcher/heartbeat/cron UX

This layer sits below the execution plane but above raw daemon process management.

### Layering

- **Execution Plane**
  - executions
  - templates
  - triggers
  - evidence / audit
- **Knowledge Plane**
  - memory attach
  - memory provenance
  - distill back to base agent
- **Agent-Native Surface**
  - PicoClaw-like runtime surface and UX
- **Runtime Substrate**
  - install
  - start/stop
  - isolation
  - remote lifecycle

### Core design principle

Rich agent-native behavior must be represented as structured Carrier contracts, not hidden inside opaque subprocess conventions.

That means the following contracts need to become first-class:

- `ContentBlock`
- `AttachmentRef`
- `RichOutboundMessage`
- `AgentRuntimeProfile`
- `AgentHeartbeat`
- `LauncherSession`
- `ManagedModelProfile`

## Design Workstream A: Agent-Native Tools / Skills / MCP / Runtime

### Objective

Make baseagent and managed PicoClaw expose a runtime surface that is close to upstream PicoClaw's practical capability set, while remaining policy-governed.

### Required changes

1. Expand execution tool result model
   - `ExecutionToolResult` needs attachment and structured content support
   - result metadata should distinguish plain text, file artifact, web result, media result, delegated job

2. Promote delegation from helper to runtime primitive
   - subagent jobs need durable handles, polling, cancellation, and evidence linkage

3. Upgrade skills from local registry to runtime capability system
   - install/list/search/use is present
   - next step is discovery, enablement state, and execution-time skill manifests

4. Make MCP a runtime capability plane
   - visible/hidden tools already exist
   - next step is lifecycle state, health, and capability registration that can be surfaced in WebUI/CLI and attached to managed agent profiles

### Result

PicoClaw-managed instances gain a real agent-native runtime surface instead of only "Carrier can call a richer baseagent loop."

## Design Workstream B: Media / Voice / File / Rich Message

### Objective

Introduce a text-plus-media message contract that works end-to-end:

- transport ingress
- baseagent loop
- tool results
- outbound transport rendering
- execution artifacts / evidence

### New contracts

1. `ContentBlock`
   - `text`
   - `image`
   - `audio`
   - `video`
   - `file`
   - `tool_result`

2. `AttachmentRef`
   - stable id
   - media type
   - file path or artifact id
   - source channel metadata

3. `RichOutboundMessage`
   - text fallback
   - structured blocks
   - optional suggested transport rendering hints

### First phase scope

- file send/receive
- image/audio/video attachment normalization
- voice/audio transcription hook
- rich outbound abstraction, even if Telegram renders only a subset initially

### Important boundary

This work is not blocked on multi-channel parity. One transport can implement the contract first; the contract is the important part.

## Design Workstream C: Provider / Model Direct-Surface

### Objective

Match more of PicoClaw's direct agent-side provider/model ergonomics without weakening Carrier governance.

### Recommended direction

Refactor from provider-id-centric managed onboarding toward protocol/profile-centric runtime profiles:

- protocol family
  - `openai-compatible`
  - `ollama-compatible`
  - `oauth-openai`
- managed profile
  - auth source
  - model alias
  - request timeout
  - fallback chain
  - same-model fanout set

### Needed capabilities

- per-agent `model_list` style rendering
- direct model alias surface
- protocol-based config normalization
- per-model timeout / retry / fallback
- optional same-model load-balancing for standalone PicoClaw mode

### Important boundary

Carrier should still own:

- provider governance
- audit
- resolution trace
- cost attribution

So the design is not "let PicoClaw ignore Carrier policy." It is "make PicoClaw-facing config feel direct, but back it with Carrier contracts."

## Design Workstream D: Launcher / Cron / Heartbeat / Standalone UX

### Objective

Offer a coherent single-agent UX for managed PicoClaw instead of spreading it across TUI, WebUI, daemon API, and app shell.

### UX surfaces

1. Standalone agent run
   - one-shot prompt
   - interactive local chat

2. Launcher surface
   - local status
   - provider readiness
   - memory attach visibility
   - skill / MCP visibility
   - heartbeat and last activity

3. Cron / reminder management
   - schedule jobs
   - list jobs
   - cancel jobs
   - show last run / next run

4. Heartbeat
   - per-agent heartbeat state
   - last successful chat/tool loop
   - last provider success/failure

### Form factor recommendation

Do not build a separate PicoClaw product shell.

Instead:

- add CLI direct surfaces under `carrier picoclaw ...` or `carrier agent ...`
- add a lightweight WebUI launcher page or panel for managed agents
- continue reusing Tauri as the desktop shell

## Delivery Sequence

### Phase 1: Runtime and provider foundations

- structured content/result contracts
- agent-native tools/delegation/skills/MCP refinements
- protocol/profile-based provider surface

### Phase 2: Media and standalone UX

- attachment ingress/egress
- voice/file/rich message pipeline
- launcher page and heartbeat model

### Phase 3: Cron and end-to-end parity hardening

- cron UX
- standalone chat mode
- complete E2E and live-provider validation

## Testing Strategy

### Unit / integration

- baseagent tool and loop tests
- gateway managed PicoClaw renderer tests
- daemon launcher/heartbeat/cron endpoint tests

### Full-stack local

- managed PicoClaw local install
- local one-shot chat
- file send/receive
- heartbeat state update
- cron create/list/run

### Live-provider

- provider/model profile fanout/fallback
- audio/file/rich result evidence
- skill/MCP visibility in launcher

## Success Criteria

Carrier should be able to do all of the following for a managed PicoClaw instance, without leaving Carrier surfaces:

- configure direct model/provider behavior with PicoClaw-like ergonomics
- run one-shot or interactive local chat
- use richer tools/skills/MCP surfaces
- ingest and emit file/media/rich responses
- schedule and observe cron jobs
- see heartbeat and runtime health
- keep all of the above under Carrier memory, policy, evidence, and audit
