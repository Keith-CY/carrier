# Channel Routing Auth Parity Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Finish the current `channel / routing / auth` surface by unifying channel capabilities, inbound routing, pairing/session auth, provider/channel credential auth, and the corresponding WebUI management flows.

**Architecture:** Keep the execution path centered in `gateway`. Introduce a canonical channel registry plus a typed inbound envelope so Telegram, Discord, Feishu, CLI-style commands, and WebUI all route through one core dispatcher instead of bespoke handlers. Treat auth as three distinct but connected layers: gateway API auth, channel/session auth, and provider/channel credential auth. WebUI should stop hard-coding channel assumptions and consume the same server-side status model.

**Tech Stack:** Go in `gateway`, existing daemon HTTP APIs, existing `baseagent` command/chat path, React + Vite in `webui`, existing gateway RBAC/session stores, current onboarding/settings flows.

---

## Scope

- In scope:
  - channel registry and capability model
  - unified inbound routing envelope
  - pairing/session validation normalization
  - unified provider/channel auth status APIs
  - WebUI channel/auth status and onboarding updates
- Out of scope:
  - voice / speech input
  - new external channel providers beyond `telegram`, `discord`, `feishu`, `webui`
  - media upload / voice bot transport parity

---

### Task 1: Introduce A Canonical Channel Registry

**Files:**
- Create: `gateway/channel_registry.go`
- Create: `gateway/channel_registry_test.go`
- Modify: `gateway/setup.go`
- Modify: `gateway/commands.go`
- Modify: `gateway/managed_instances.go`

**Step 1: Write the failing tests**

Add tests for:
- registry returns descriptors for `telegram`, `discord`, `feishu`, and `webui`
- each descriptor exposes the right capabilities:
  - `telegram`: bot token, webhook or polling, pairing supported
  - `discord`: bot token/public key, webhook interactions, no pairing token flow
  - `feishu`: bot token/verification token, webhook events
  - `webui`: no bot token, no webhook, local gateway auth only
- unsupported channel IDs are rejected centrally rather than by scattered switches

**Step 2: Run the targeted tests**

Run: `go test ./... -run 'TestChannelRegistryDescriptors|TestChannelRegistryRejectsUnsupportedChannels'`
Expected: FAIL because channel metadata is still split across `setup.go`, `commands.go`, onboarding code, and helper switches.

**Step 3: Write the minimal implementation**

Create a small registry with types similar to:

```go
type ChannelID string

type ChannelCapabilities struct {
    SupportsWebhook bool
    SupportsPolling bool
    SupportsPairing bool
    RequiresBotToken bool
    RequiresWebhookSecret bool
    SupportsWebUI bool
}

type ChannelDescriptor struct {
    ID           ChannelID
    DisplayName  string
    Capabilities ChannelCapabilities
}
```

Add helpers:
- `SupportedChannelDescriptors() []ChannelDescriptor`
- `LookupChannelDescriptor(id string) (ChannelDescriptor, bool)`
- `NormalizeChannelID(raw string) (ChannelID, error)`

Refactor existing provider/channel validation to use the registry first, then keep existing behavior intact.

**Step 4: Run the targeted tests again**

Run: `go test ./... -run 'TestChannelRegistryDescriptors|TestChannelRegistryRejectsUnsupportedChannels'`
Expected: PASS

**Step 5: Commit**

```bash
git add gateway/channel_registry.go gateway/channel_registry_test.go gateway/setup.go gateway/commands.go gateway/managed_instances.go
git commit -m "feat: add canonical gateway channel registry"
```

---

### Task 2: Unify Inbound Channel Routing

**Files:**
- Create: `gateway/channel_router.go`
- Create: `gateway/channel_router_test.go`
- Modify: `gateway/commands.go`
- Modify: `gateway/server.go`
- Modify: `gateway/telegram_transport.go`
- Modify: `gateway/server_telegram_webhook_test.go`
- Modify: `gateway/server_discord_webhook_test.go`
- Modify: `gateway/server_feishu_webhook_coverage_test.go`

**Step 1: Write the failing tests**

Add tests for:
- Telegram webhook/polling messages, Discord interaction payloads, and Feishu messages all normalize into one inbound envelope type
- non-command text from each channel routes through the same baseagent chat path
- slash/interaction commands from each channel route through the same command dispatcher
- request IDs, provider/chat identity, and session token context are preserved uniformly

**Step 2: Run the targeted tests**

Run: `go test ./... -run 'TestChannelRouterTelegramEnvelope|TestChannelRouterDiscordEnvelope|TestChannelRouterFeishuEnvelope|TestChannelRouterDispatchesBaseagentChat'`
Expected: FAIL because webhook handlers still dispatch through channel-specific paths.

**Step 3: Write the minimal implementation**

Add a typed envelope:

```go
type InboundChannelEnvelope struct {
    Channel      string
    ChatID       string
    RequestID    string
    SessionToken string
    Command      string
    Text         string
    Kind         string
    Metadata     map[string]string
}
```

Create one router entry point:
- `RouteInboundChannel(ctx, envelope, daemon, sessions, downloads, rl, onboard) GatewayResponse`

Refactor channel handlers so they:
- verify transport-specific signatures/tokens first
- normalize payload into `InboundChannelEnvelope`
- call the shared router

Keep transport-specific response shaping at the edge only:
- Discord still wraps in interaction response objects
- Feishu still emits Feishu message payloads
- Telegram still sends bot messages

**Step 4: Run the targeted tests again**

Run: `go test ./... -run 'TestChannelRouterTelegramEnvelope|TestChannelRouterDiscordEnvelope|TestChannelRouterFeishuEnvelope|TestChannelRouterDispatchesBaseagentChat'`
Expected: PASS

**Step 5: Commit**

```bash
git add gateway/channel_router.go gateway/channel_router_test.go gateway/commands.go gateway/server.go gateway/telegram_transport.go gateway/server_telegram_webhook_test.go gateway/server_discord_webhook_test.go gateway/server_feishu_webhook_coverage_test.go
git commit -m "feat: unify inbound channel routing"
```

---

### Task 3: Normalize Pairing And Session Auth

**Files:**
- Modify: `gateway/session.go`
- Modify: `gateway/pairing_sessions.go`
- Modify: `gateway/commands.go`
- Modify: `gateway/telegram_pairing.go`
- Modify: `gateway/pairing_sessions_more_test.go`
- Modify: `gateway/commands_test.go`
- Modify: `gateway/telegram_pairing_coverage_test.go`

**Step 1: Write the failing tests**

Add tests for:
- session tokens are strictly scoped to `provider + chatID`
- expired or missing sessions fail the same way whether the caller is CLI, Telegram, Discord, or Feishu
- pairing session listing returns status fields useful for WebUI
- channel-specific pairing rules are enforced centrally, not inside each handler

**Step 2: Run the targeted tests**

Run: `go test ./... -run 'TestSessionAuthScopesTokenToProviderAndChat|TestChannelPairingStatusSummary|TestCommandAuthRejectsExpiredSession'`
Expected: FAIL because session and pairing logic is still spread across command parsing and handler glue.

**Step 3: Write the minimal implementation**

Expand the session model with normalized metadata:

```go
type SessionRecord struct {
    Provider       string
    ChatID         string
    SessionToken   string
    PairState      string
    PairMethod     string
    CreatedAt      string
    LastSeenAt     string
}
```

Add central helpers:
- `ValidateSession(provider, chatID, token string) *apiErr`
- `ListPairingStatus(provider string) []pairingSessionSummary`
- `ChannelSupportsPairing(channel string) bool`

Preserve current token format and persistence file behavior unless a test proves it must change.

**Step 4: Run the targeted tests again**

Run: `go test ./... -run 'TestSessionAuthScopesTokenToProviderAndChat|TestChannelPairingStatusSummary|TestCommandAuthRejectsExpiredSession'`
Expected: PASS

**Step 5: Commit**

```bash
git add gateway/session.go gateway/pairing_sessions.go gateway/commands.go gateway/telegram_pairing.go gateway/pairing_sessions_more_test.go gateway/commands_test.go gateway/telegram_pairing_coverage_test.go
git commit -m "feat: normalize gateway pairing and session auth"
```

---

### Task 4: Unify Provider And Channel Auth Status APIs

**Files:**
- Create: `gateway/auth_status_api.go`
- Create: `gateway/auth_status_api_test.go`
- Modify: `gateway/provider_auth.go`
- Modify: `gateway/setup.go`
- Modify: `gateway/webui_onboard.go`
- Modify: `gateway/server.go`
- Modify: `gateway/credential_store.go`

**Step 1: Write the failing tests**

Add tests for:
- `GET /api/v1/auth/providers` returns provider auth mode, whether saved credential exists, and whether reuse is possible
- `GET /api/v1/channels` returns channel descriptors plus redacted setup state
- onboarding/setup responses match the same auth status model
- invalid provider auth input and invalid channel setup input fail through shared validators

**Step 2: Run the targeted tests**

Run: `go test ./... -run 'TestProviderAuthStatusAPI|TestChannelStatusAPI|TestOnboardReusesSharedAuthValidation'`
Expected: FAIL because WebUI currently reads status from a mix of `/setup`, onboarding responses, and hard-coded assumptions.

**Step 3: Write the minimal implementation**

Expose two small APIs:
- `GET /api/v1/auth/providers`
- `GET /api/v1/channels`

Return redacted, UI-friendly payloads such as:

```json
{
  "providers": [{"id":"openai","authMode":"api_key","reusable":true,"configured":true}],
  "channels": [{"id":"telegram","supportsPairing":true,"configured":false}]
}
```

Refactor:
- provider auth parsing remains in `provider_auth.go`
- channel setup parsing remains in `setup.go`
- both flow through one shared status shape used by onboarding and settings

Do not leak secrets in any API response.

**Step 4: Run the targeted tests again**

Run: `go test ./... -run 'TestProviderAuthStatusAPI|TestChannelStatusAPI|TestOnboardReusesSharedAuthValidation'`
Expected: PASS

**Step 5: Commit**

```bash
git add gateway/auth_status_api.go gateway/auth_status_api_test.go gateway/provider_auth.go gateway/setup.go gateway/webui_onboard.go gateway/server.go gateway/credential_store.go
git commit -m "feat: add unified gateway auth status APIs"
```

---

### Task 5: Move WebUI To The Unified Channel/Auth Model

**Files:**
- Modify: `webui/src/app/session.tsx`
- Modify: `webui/src/lib/api.ts`
- Modify: `webui/src/features/onboarding/useOnboardingData.ts`
- Modify: `webui/src/features/onboarding/useOnboardingActions.ts`
- Modify: `webui/src/features/onboarding/components/SetupStep.tsx`
- Modify: `webui/src/features/settings/useSettingsData.ts`
- Create: `webui/src/features/settings/useAuthStatusData.ts`
- Create: `webui/src/features/settings/useAuthStatusData.test.tsx`
- Modify: `webui/src/features/onboarding/OnboardingSection.test.tsx`
- Modify: `webui/src/features/onboarding/useOnboardingWizardState.test.tsx`

**Step 1: Write the failing tests**

Add tests for:
- WebUI settings loads channel/auth status from the new APIs
- onboarding channel choices come from server descriptors instead of a hard-coded select list
- pairing-required channels show the right gating and status text
- WebUI login/auth-expired behavior still works after the new API calls are added

**Step 2: Run the targeted tests**

Run: `bun test`
Expected: FAIL because WebUI still hard-codes channel options and only knows a local gateway token model.

**Step 3: Write the minimal implementation**

WebUI updates:
- load `/api/v1/channels` and `/api/v1/auth/providers`
- stop hard-coding Telegram/Discord/Feishu directly in `SetupStep`
- show channel capability hints:
  - pairing required
  - webhook secret required
  - already configured / reusable
- keep existing token login flow intact in [session.tsx](/Users/ChenYu/Documents/Github/carrier/webui/src/app/session.tsx)

Keep scope tight:
- no redesign of the login overlay
- no new pages if settings can host the status summary

**Step 4: Run the targeted tests again**

Run: `bun test`
Expected: PASS

**Step 5: Commit**

```bash
git add webui/src/app/session.tsx webui/src/lib/api.ts webui/src/features/onboarding/useOnboardingData.ts webui/src/features/onboarding/useOnboardingActions.ts webui/src/features/onboarding/components/SetupStep.tsx webui/src/features/settings/useSettingsData.ts webui/src/features/settings/useAuthStatusData.ts webui/src/features/settings/useAuthStatusData.test.tsx webui/src/features/onboarding/OnboardingSection.test.tsx webui/src/features/onboarding/useOnboardingWizardState.test.tsx
git commit -m "feat: align webui with unified channel auth model"
```

---

### Task 6: Hardening, Docs, And End-To-End Verification

**Files:**
- Modify: `docs/command-contract.md`
- Modify: `gateway/README.md`
- Modify: `gateway/server_test.go`
- Modify: `gateway/webui_onboard_test.go`
- Modify: `gateway/server_webhook_test.go`

**Step 1: Write the failing tests**

Add coverage for:
- one happy path per channel:
  - Telegram paired command
  - Discord interaction command
  - Feishu webhook command
  - WebUI onboarding/settings auth status fetch
- one failure path per auth layer:
  - missing gateway bearer token
  - invalid session token
  - invalid provider/channel credential input

**Step 2: Run the targeted tests**

Run: `go test ./... -run 'TestChannelRoutingAuthHappyPaths|TestChannelRoutingAuthFailurePaths'`
Expected: FAIL until all new plumbing is wired.

**Step 3: Write the minimal implementation**

Finish remaining glue:
- update command/API docs for the new unified status endpoints and routing model
- ensure response codes and error payloads are stable
- make sure channel-specific wrappers still preserve their transport contract

**Step 4: Run the full verification**

Run:
- `go test ./...` in `gateway`
- `go test ./server` in `daemon`
- `go test ./...` in `baseagent`
- `bun test` in `webui`

Expected: PASS

**Step 5: Commit**

```bash
git add docs/command-contract.md gateway/README.md gateway/server_test.go gateway/webui_onboard_test.go gateway/server_webhook_test.go
git commit -m "test: harden channel routing and auth integration"
```

---

## Deferred TODO

Not part of this plan:

- voice input / speech transcription
- media upload / voice bot channel parity
- additional external channels beyond `telegram`, `discord`, `feishu`, `webui`
