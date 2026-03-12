# PicoClaw Direct-Surface V2 Design

**Scope:** Continue single-agent parity work after Tasks 1-12 by focusing on provider/model direct-surface ergonomics for managed PicoClaw-class agents.

## Goal

Make managed agents expose a first-class agent-local model surface that is visible, selectable, and auditable from Carrier CLI, WebUI, launcher, and runtime entrypoints.

This phase is about direct-surface parity, not multi-channel parity. Carrier remains the authority for governance, memory, evidence, and execution, but managed agents should stop feeling like they only have an opaque provider choice.

## Problems To Solve

### 1. Managed model profiles exist, but are mostly write-only

Carrier already renders `model_list` / `provider_profiles` during managed onboarding:
- `gateway/managed_onboard.go`
- `cmd/carrier/main.go`

But after install/onboard, the control plane does not expose:
- what model aliases exist for the managed agent
- which model is the default surface
- which provider profile is primary vs fallback
- what base URL / protocol family the agent-local runtime is using

### 2. Launcher summary is provider-centric, not model-centric

`GET /api/v1/agents/:id/launcher` currently shows:
- status
- heartbeat
- memory
- provider readiness
- cron
- session

It does not show:
- model list
- current default model
- model alias map
- fallback chain
- protocol family / provider profile metadata

### 3. CLI agent-native run surface cannot select a model

`carrier agent run` currently supports:
- `--provider`
- `--session-id`

It cannot explicitly select:
- a logical model alias
- a concrete model id

That means the direct-surface UX is weaker than upstream PicoClaw's `model_list`-driven runtime.

### 4. WebUI Agent Detail cannot explain the agent-local model surface

`AgentDetailPage` can show provider readiness and capabilities, but not:
- model aliases
- current default model
- fallback candidates
- model protocol family / base URL

## Design Principles

1. **Persist the surface once, reuse everywhere**
   - do not reparsed managed config files on every request
   - store a normalized model surface in managed instance metadata

2. **Alias-first, model-second**
   - user-facing runtime UX should prefer `model alias`
   - raw `model id` remains available for debugging or exact targeting

3. **Provider governance remains authoritative**
   - this is a runtime surface improvement, not a bypass
   - surfaced model data must be explainable in launcher/evidence/observability

4. **Keep chat/runtime changes incremental**
   - first slice should not redesign all of baseagent model selection
   - thread model selection through managed-agent chat entrypoints with bounded scope

## New Data Model

### ManagedAgentModelSurface

Persist on managed instance records:
- `defaultProfile`
- `profiles[]`

Each profile carries:
- `profileName`
- `modelAlias`
- `modelId`
- `providerId`
- `providerKey`
- `protocolFamily`
- `baseUrl`
- `authMethod`
- `primary`

This creates a stable source for launcher, CLI, and WebUI.

## Public Surface Changes

### Gateway launcher summary

Extend `/api/v1/agents/:id/launcher` with:
- `modelSurface.defaultProfile`
- `modelSurface.profiles[]`

### CLI

Extend:
- `carrier agent run <agent_id> ... --model-alias <alias>`
- `carrier agent run <agent_id> ... --model <model-id>`
- `carrier agent shell ...` should carry forward selected alias/model for the session
- `carrier agent launcher` should render model surface summary

### WebUI

`AgentDetailPage` should show:
- default model alias / model id
- profile list
- protocol family
- fallback/secondary profiles once present

## Phased Delivery

### Task 13
Persist managed model surface in instance metadata and expose it via launcher summary.

### Task 14
Add CLI model selection flags and thread them into `/api/v1/agents/:id/chat`.

### Task 15
Show model surface in WebUI Agent Detail and launcher output.

### Task 16
Add fallback/fanout policy metadata to model surface and observability.

## Non-Goals For This Slice

- no multi-channel parity
- no full provider marketplace or dynamic model discovery
- no remote distributed fanout execution yet
- no complete redesign of baseagent provider resolution
