# Managed Agent Parity V5 Follow-up Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Continue single-agent parity after Tasks 40-43 by deepening outbound media rendering, tightening live media verification, and extending runtime/launcher operator depth.

**Architecture:** Keep the existing rich attachment and managed model surfaces as the canonical contracts. Media improvements should prefer transport-native rendering before falling back to plain text or generic document delivery. Runtime and launcher improvements should continue to read from the managed instance store and daemon runtime endpoints rather than introducing a second state source.

**Tech Stack:** Go (`gateway`, `daemon/server`, `baseagent`, `cmd/carrier`), React WebUI (`AgentDetailPage`, execution detail), shell-based live smoke scripts, Vitest, Playwright.

---

### Task 44: Add Transport-Native Outbound Media Rendering

**Status:** completed on 2026-03-13

**Files:**
- Modify: `gateway/telegram_transport.go`
- Modify: `gateway/telegram_transport_test.go`

**Step 1: Write the failing tests**

Add coverage for:
- rich audio output rendering through Telegram `sendAudio`
- rich voice output rendering through Telegram `sendVoice`
- rich video output rendering through Telegram `sendVideo`
- image/document rendering remains unchanged

**Step 2: Run test to verify it fails**

Run:
- `go test ./gateway -run 'TestTelegramTransportSendRichOutboundMessageAttachmentFallbacks|TestTelegramTransportSendRichOutboundMessageAudioAndVideoPreferNativeRender' -count=1`

Expected: FAIL because Telegram rich output currently downgrades audio/video to `sendDocument`.

**Step 3: Write minimal implementation**

Add:
- `SendAudio`, `SendVoice`, and `SendVideo` to the Telegram API surface
- media-kind selection that distinguishes `image`, `document`, `audio`, `voice`, and `video`
- conservative fallback to `document` if no transport-native match is available

Rules:
- keep outbound text fallback intact
- prefer `voice` when the attachment/block explicitly says `voice` or an OGG/Opus voice-like media type
- do not break existing image/document behavior

**Step 4: Run test to verify it passes**

Run:
- `go test ./gateway -run 'TestTelegramTransportSendRichOutboundMessageAttachmentFallbacks|TestTelegramTransportSendRichOutboundMessageAudioAndVideoPreferNativeRender' -count=1`

### Task 45: Tighten Live Media Verification

**Status:** in progress on 2026-03-13

Strengthen the live-provider smoke so media capability assertions are explicit:
- OpenAI-capable environments: transcription hard-pass
- OpenRouter: soft-optional unless explicitly required
- richer outbound media smoke should verify that agent output is not reduced to plain text when a transport-native media result exists

### Task 46: Add Media Output Drill-Down To Evidence And UI

**Status:** pending

Extend evidence and WebUI detail surfaces so operators can tell:
- which outputs were generated media vs plain text
- which render mode was selected
- which attachment IDs / artifact IDs were used for delivery

### Task 47: Add Skill Provenance And Health Detail

**Status:** pending

Extend the managed skill lifecycle with:
- source/provenance summary
- install/update timestamps
- per-skill health/remediation hints

### Task 48: Add MCP Attach/Detach Runtime Controls

**Status:** pending

Extend managed MCP controls beyond enable/disable/detail:
- attach/detach lifecycle
- persisted config edits
- clearer health and remediation flow

### Task 49: Make Delegated Job History Durable

**Status:** pending

Persist recent delegated job history so runtime and launcher views survive process restart and expose:
- recent jobs
- terminal status
- summary/output preview
- failure reason

### Task 50: Add Launcher Operator Actions

**Status:** pending

Deepen the operator console with:
- direct remediation actions from launcher detail
- richer heartbeat context
- cron run-now / pause / resume / last-run detail
