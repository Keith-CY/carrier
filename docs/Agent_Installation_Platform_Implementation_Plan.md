# Agent Installation Platform — Implementation Plan (Phase 1)

> Scope/priority source-of-truth: `docs/Agent_Installation_Platform_PRD.md`.
> This document defines execution sequencing only; if conflicts exist, follow PRD.

## 1. Plan Goal
Deliver an end-to-end, production-like OpenClaw workflow on local runtime (macOS/Linux) and WSL2 (Windows), with Telegram/Discord/Feishu control, Per-Agent/Shared/Public memory management, and Base Agent triage + diagnose escalation.

## 2. Scope Baseline
- Runtime: no Docker path
- Phase 1 fully operational Agent: OpenClaw only
- Candidate-only (post-stability onboarding): Pi Mono, NanoClaw, Pico Claw
- Memory taxonomy: Per-Agent, Shared, Public
- Remote diagnosis service: not integrated in Phase 1, consent and handoff placeholder only

## 3. Workstreams

### W1. Manifest and Catalog Contract
Deliverables:
- OpenClaw manifest schema and validation
- Catalog entries for OpenClaw, Pi Mono, NanoClaw, Pico Claw with candidate state flags

Acceptance:
- OpenClaw manifest passes validation and can drive install/start/stop/upgrade paths
- candidate agents are listed but blocked from full run path

### W2. Daemon Core Lifecycle
Deliverables:
- `install/start/stop/status/logs/upgrade/diagnose` handlers
- state machine for install/runtime/health
- runtime prerequisite checks for macOS/Linux host and Windows WSL2

Acceptance:
- OpenClaw can complete full command lifecycle successfully on supported environments
- failure responses include structured error codes and actionable guidance

### W3. Base Agent Triage and Escalation
Deliverables:
- evidence collector (logs, exit codes, probes, command traces)
- Base Agent LLM-assisted analysis pipeline
- policy-bounded repair action executor
- unresolved escalation flow with diagnose bundle and consent prompt

Acceptance:
- simulated failure invokes Base Agent and returns structured triage output
- unresolved case produces diagnose artifact and asks for remote diagnosis consent

### W4. Gateway Integration (3 Providers)
Deliverables:
- Telegram adapter
- Discord adapter
- Feishu adapter
- pairing/session implementation
- command formatter + request_id propagation
- read-only download endpoint

Acceptance:
- all P0 commands work from each provider after pairing
- logs and diagnose artifacts can be downloaded through tokenized endpoint links

### W5. Memory Platform
Deliverables:
- memory package parser (`memory.yaml`)
- import/export
- duplicate (Public/Shared -> Per-Agent)
- share (Per-Agent -> Shared)
- attach/detach logic for OpenClaw
- mount rules and mode enforcement

Acceptance:
- Per-Agent, Shared, Public lifecycle operations are functional and auditable
- mount permissions follow policy (Per-Agent RW, Shared/Public RO by default)

### W6. Security, Audit, and Reliability
Deliverables:
- secure secret handling (masked output + encrypted/keychain storage)
- audit log for lifecycle and repair actions
- daemon service restart behavior and log rotation

Acceptance:
- all sensitive outputs are redacted
- all privileged operations appear in audit logs with actor + request_id

## 4. Milestone Sequence
1. M1: OpenClaw manifest + daemon lifecycle skeleton
2. M2: runtime checks + status/logs/diagnose
3. M3: Base Agent triage + unresolved escalation flow
4. M4: Gateway 3-provider command path + pairing/session
5. M5: Memory lifecycle and attachment model
6. M6: hardening, end-to-end test pass, release-ready demo

## 5. Test and Validation Plan

### 5.1 Happy-path E2E
- Pair via each provider
- Install/start/status/logs/diagnose/stop OpenClaw
- Verify healthy state and expected response shape

### 5.2 Failure-path E2E
- missing runtime dependencies
- missing required env
- port conflict
- forced start crash -> Base Agent triage
- unresolved triage -> diagnose + remote consent prompt

### 5.3 Memory E2E
- import Public package
- duplicate to Per-Agent
- promote to Shared
- attach memories to OpenClaw
- verify mount mode enforcement

## 6. Exit Criteria for Phase 1
- OpenClaw full lifecycle stable on supported environments
- all required commands available through Telegram/Discord/Feishu
- Base Agent escalation flow is functional
- Per-Agent/Shared/Public memory flows are functional
- diagnose coverage target met for unresolved failures

## 7. Terminology alignment
- Runtime = host(macOS/Linux) or WSL2(Windows), no Docker path in Phase 1.
- Diagnose = sanitized artifact generation + optional remote diagnosis consent flow.
- Memory types = Per-Agent / Shared / Public (mount policy per PRD).
- Priority semantics = P0 must-have, P1 should-have, P2 later.

## 8. Phase 2 Entry Gate
Start Pi Mono/NanoClaw/Pico Claw onboarding only when:
- OpenClaw P0 acceptance passes consistently
- no unresolved P0 reliability/security defects
- triage and diagnose flow meets operational expectations
