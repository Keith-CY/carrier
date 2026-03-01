# Design: Consistent Channel-Skip UX for `carrier add` and `carrier onboard`

**Issue:** [#1508](https://github.com/Keith-CY/carrier/issues/1508)  
**Author:** design subagent  
**Date:** 2026-03-01  
**Status:** Draft — awaiting review before implementation

---

## 1. Problem Statement

`carrier onboard` (TUI) allows users to press Enter to skip Telegram configuration and enter **WebUI-only mode**. But `carrier add openclaw` (TUI) **forces** a Telegram bot token prompt with no way to skip — the token field is marked `required: true`, and the channel is auto-resolved as Telegram with no opt-out.

This is an inconsistency that blocks users who:
- Want to use the WebUI only (no chat channels)
- Want to configure channels later (via `carrier config set` or WebUI)
- Don't have a Telegram bot token handy during initial setup

### Root Cause Analysis

| Flow | Channel Selection | Token Prompt | Skip Possible? |
|------|------------------|-------------|----------------|
| `carrier onboard` (TUI) | `promptMinimalChannelSelection` — Enter = WebUI-only | `promptChannelCredentialsMinimal` — only if channel chosen | ✅ Yes |
| `carrier add <agent>` (TUI) | `resolveManagedAgentChannel` — hardcoded first channel | `promptInput(..., required=true)` — always required | ❌ No |
| `carrier add <agent>` (WebUI) | `handleWebUIAdd` — `channelToken is required` error if empty | API validation rejects empty token | ❌ No |
| `carrier onboard` (WebUI) | `handleWebUIOnboard` — `normalizeOnboardChannel` accepts `""`/`"skip"` | Only validated if channel chosen | ✅ Yes |

The fix must unify the channel setup UX so all four paths behave consistently.

---

## 2. Desired UX Flow

### 2.1 `carrier add <agent>` TUI Flow (the main fix)

```
Carrier Add (TUI)
-----------------
Agent: OpenClaw
Isolation runtime: disabled
Tip: for browser flow, run `carrier add openclaw --webui`.
Instance: openclaw-a1b2c3d4
Name: openclaw

Step 1/4: Configure chat channel
  Available channels: telegram, discord, feishu
  Type a channel ID to configure, or press Enter for WebUI-only mode.
  Channel [telegram/discord/feishu/WebUI-only]: █

  (user presses Enter)

  ℹ️  WebUI-only mode selected.
  No chat channel will be configured. You can add one later via:
    • carrier config set openclaw channel.telegram.token=<BOT_TOKEN>
    • Or use the Carrier WebUI at http://127.0.0.1:8787/

Step 2/4: Configure LLM provider
  ...  (unchanged)

Step 3/4: Prepare OpenClaw configuration
  ...  (config generated with no channel block, or channel disabled)

Step 4/4: Install and start OpenClaw
  ...  (unchanged)
✅ OpenClaw installed and started (WebUI-only mode).
```

If the user **types a channel ID** (e.g., `telegram`):

```
  Channel [telegram/discord/feishu/WebUI-only]: telegram
  Using channel: Telegram

  Telegram bot token for OpenClaw: █

  (user enters token or presses Enter)
```

If the user presses Enter at the token prompt and the channel requires a token:

```
  Telegram bot token for OpenClaw: █

  (user presses Enter — empty)

  ⚠️  No token provided. The Telegram channel will be created but disabled.
  You can configure the token later via WebUI or `carrier config set`.
```

### 2.2 `carrier onboard` TUI Flow (already works, minor polish)

The existing flow already handles this correctly. Only minor polish:
- Add the multi-channel display: `telegram, discord, feishu` instead of just `telegram`
- Make the hint text consistent with `carrier add`

### 2.3 `carrier add <agent>` WebUI Flow

The WebUI `handleWebUIAdd` endpoint should accept empty/skipped channel, matching the onboard WebUI behavior:

- If `channel` is `""`, `"skip"`, `"none"`, or `"webui"` → WebUI-only mode
- If `channelToken` is `""` when a channel is selected → create channel config with `enabled: false` and `setup_pending: true`

### 2.4 `carrier add <agent> -q` (Quiet Mode)

Quiet mode should auto-select WebUI-only when no channel token is available from env/config:

```
  Channel: WebUI-only (no token available, quiet mode)
```

---

## 3. Config Schema Changes

### 3.1 Managed Agent Config (PicoClaw/OpenClaw/ZeroClaw)

**No schema changes needed.** The config formats already support the "no channel" case — the gateway WebUI onboard path already does this via `channelSetupPending`. We just need to generate configs without channel blocks (or with disabled channels).

#### PicoClaw (`config.json`) — WebUI-only mode
```json
{
  "agents": { "defaults": { ... } },
  "model_list": [ ... ],
  "providers": { ... },
  "channels": {}
}
```

#### OpenClaw (`openclaw.json`) — WebUI-only mode
The `openclawcfg.BuildManagedConfigPayload` function needs to accept empty `ChannelID` and generate a config with no channel section.

#### ZeroClaw (`config.toml`) — WebUI-only mode
```toml
api_key = "..."
default_provider = "openai"
default_model = "openai/gpt-5.2"

[agent]
max_tool_iterations = 20

# No [channels_config.*] section — WebUI-only
```

### 3.2 Carrier `config.v2.json`

Already supports `channels: []` (empty array). The `needsInitialOnboard` function checks `enabledChannels == 0` and treats it as "not onboarded" — this needs a small fix:

```go
// Current (incorrect):
if enabledChannels == 0 {
    return true  // thinks not onboarded
}

// Fixed:
// An empty channel list is valid for WebUI-only mode.
// "Not onboarded" means no config exists or no providers configured.
if len(cfg.ModelList) == 0 {
    return true
}
```

Wait — `needsInitialOnboard` is used by `runBootstrap` to decide whether to trigger onboarding. If a user onboards with WebUI-only mode and has a provider configured, the bootstrap should NOT re-trigger onboarding. This is a **critical fix**.

### 3.3 Instance Record

The managed agent instance record (`instances.json`) already has an optional `Channel` field. When channel is skipped:
- `Channel: ""` (empty string)
- `PairRequired: false`
- `PairedChatID: ""`

No schema change needed.

---

## 4. Code Changes

### 4.1 `cmd/carrier/main.go` — `runAddManagedAgentTUI`

**Current flow** (lines ~5575–5590):
```go
channel, ok := resolveManagedAgentChannel(cfg.ID)
if !ok {
    return fmt.Errorf("%s channel is unavailable", cfg.ID)
}
// ... forces token prompt
token, err = promptInput(reader, out, channel.TokenLabel, true)  // required=true
```

**New flow:**
```go
// Replace resolveManagedAgentChannel + forced token with shared channel prompt
channel, hasChannel, err := promptManagedChannelSelection(reader, out, cfg.ID)
if err != nil {
    return err
}

token := ""
tokenSource := ""
if hasChannel {
    _, _ = fmt.Fprintf(out, "Using channel: %s\n", channel.Name)
    // Try to reuse existing token
    if managedAddReusesChannelToken(cfg.ID, channel.ID) {
        token, tokenSource = resolveManagedChannelToken(channel.ID)
        if tokenSource != "" {
            _, _ = fmt.Fprintf(out, "Reused %s token from %s.\n", channel.Name, tokenSource)
        }
    }
    if tokenSource == "" {
        // Prompt for token, but allow empty → setup_pending
        token, err = promptInput(reader, out, channel.TokenLabel+" (press Enter to skip)", false)
        if err != nil {
            return err
        }
        if token == "" {
            _, _ = fmt.Fprintln(out, "Channel token skipped. Channel will be created but disabled.")
            _, _ = fmt.Fprintln(out, "Configure later via WebUI or `carrier config set`.")
        }
    }
} else {
    _, _ = fmt.Fprintln(out, "WebUI-only mode: no chat channel configured.")
    _, _ = fmt.Fprintln(out, "Add a channel later via WebUI or `carrier config set`.")
}
```

**New function: `promptManagedChannelSelection`**
```go
func promptManagedChannelSelection(reader *bufio.Reader, out io.Writer, agentID string) (picoclawChannel, bool, error) {
    channels, ok := managedAgentChannels(agentID)
    if !ok || len(channels) == 0 {
        return picoclawChannel{}, false, nil
    }
    
    // Build channel list string
    ids := make([]string, 0, len(channels))
    for _, ch := range channels {
        ids = append(ids, ch.ID)
    }
    
    _, _ = fmt.Fprintf(out, "  Available channels: %s\n", strings.Join(ids, ", "))
    _, _ = fmt.Fprint(out, "  Type a channel ID to configure, or press Enter for WebUI-only mode.\n")
    _, _ = fmt.Fprintf(out, "  Channel [%s/WebUI-only]: ", strings.Join(ids, "/"))
    
    line, err := reader.ReadString('\n')
    if err != nil && !errors.Is(err, io.EOF) {
        return picoclawChannel{}, false, err
    }
    
    trimmed := strings.TrimSpace(line)
    if trimmed == "" {
        return picoclawChannel{}, false, nil  // WebUI-only
    }
    
    for _, ch := range channels {
        if strings.EqualFold(ch.ID, trimmed) {
            return ch, true, nil
        }
    }
    return picoclawChannel{}, false, fmt.Errorf("unknown channel %q for %s", trimmed, agentID)
}
```

### 4.2 `cmd/carrier/main.go` — `prepareManagedAgentAddArtifacts`

**Current:** Hard-rejects empty `channelID` and `channelToken`:
```go
if channelID == "" {
    return nil, fmt.Errorf("%s channel is required", cfg.ID)
}
if channelToken == "" {
    return nil, fmt.Errorf("%s channel token is required", cfg.ID)
}
```

**New:** Accept empty channel (WebUI-only) and empty token (setup pending):
```go
// channelID == "" → WebUI-only mode, skip channel config entirely
// channelID != "" && channelToken == "" → channel created but disabled (setup_pending)
```

The config payload builders need corresponding changes to handle these cases.

### 4.3 `cmd/carrier/main.go` — `buildManagedPicoClawConfigPayload`

Add handling for empty channel:
```go
func buildManagedPicoClawConfigPayload(...) map[string]interface{} {
    // ... existing model/provider setup ...
    
    channels := map[string]interface{}{}
    if channelID != "" {
        channelConfig := map[string]interface{}{
            "allow_from": allowFrom,
        }
        if channelToken != "" {
            channelConfig["enabled"] = true
            channelConfig["token"] = channelToken
        } else {
            channelConfig["enabled"] = false
            channelConfig["setup_pending"] = true
        }
        channels[channelID] = channelConfig
    }
    // If channelID == "", channels map stays empty → WebUI-only
    
    return map[string]interface{}{
        "agents":     ...,
        "model_list": ...,
        "providers":  ...,
        "channels":   channels,
    }
}
```

### 4.4 `cmd/carrier/main.go` — `buildManagedOpenClawConfigPayload`

Propagate `channelSetupPending` flag to `openclawcfg.BuildManagedConfigPayload`:
```go
func buildManagedOpenClawConfigPayload(...) map[string]interface{} {
    return openclawcfg.BuildManagedConfigPayload(openclawcfg.ManagedPayloadParams{
        ChannelID:           channelID,        // may be ""
        ChannelToken:        channelToken,      // may be ""
        ChannelSetupPending: channelToken == "" && channelID != "",
        AllowFrom:           allowFrom,
        ...
    })
}
```

### 4.5 `cmd/carrier/main.go` — `needsInitialOnboard`

Fix the WebUI-only false positive:
```go
func needsInitialOnboard(loadFn func() (*configv2.Config, string, error)) bool {
    cfg, _, err := loadFn()
    if err != nil || cfg == nil {
        return true
    }
    // A config with at least one model is considered "onboarded",
    // even if no channels are enabled (WebUI-only mode).
    if len(cfg.ModelList) == 0 {
        return true
    }
    // Check that config was explicitly saved (not just default empty)
    if cfg.ConfiguredAt == "" {
        return true
    }
    return false
}
```

### 4.6 `gateway/webui_add.go` — `handleWebUIAdd`

Add channel-skip support matching the onboard endpoint:
```go
// Replace the hard error:
//   writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "channelToken is required"))
// With:
channelID, webUIOnly := normalizeAddChannel(req.Channel)
if webUIOnly {
    // WebUI-only mode — skip channel config
    sess.ChannelSetupPending = true
} else if channelToken == "" {
    // Channel selected but no token → setup pending
    sess.ChannelSetupPending = true
}
```

### 4.7 `gateway/managed_onboard.go` — `prepareManagedOnboard`

**Current:** Hard-rejects empty channel:
```go
if strings.TrimSpace(sess.SelectedChannel) == "" {
    return nil, fmt.Errorf("%s channel is required", cfg.ID)
}
```

**New:** Allow empty channel for WebUI-only mode:
```go
// Empty SelectedChannel → WebUI-only mode (valid)
// Non-empty SelectedChannel with empty token → channelSetupPending (valid)
channelSetupPending := sess.ChannelSetupPending
if strings.TrimSpace(sess.SelectedChannel) == "" {
    channelSetupPending = true
}
if !channelSetupPending && channelToken == "" && strings.TrimSpace(sess.SelectedChannel) != "" {
    channelSetupPending = true
}
```

### 4.8 ZeroClaw TOML Renderer

`renderZeroClawConfigTOML` already handles `channelSetupPending` with a comment. For WebUI-only (empty channelID), skip the `[channels_config.*]` block entirely:

```go
if channelID != "" {
    // render channel section as before
} else {
    lines = append(lines, "", "# No chat channel configured (WebUI-only mode)")
}
```

### 4.9 `promptMinimalChannelSelection` (onboard flow) — minor polish

Update to show all available channels from the catalog, not just telegram:
```go
func promptMinimalChannelSelection(reader *bufio.Reader, out io.Writer) (choiceOption, bool, error) {
    channelIDs := make([]string, 0, len(onboardChannelOptions))
    for _, ch := range onboardChannelOptions {
        channelIDs = append(channelIDs, ch.ID)
    }
    _, _ = fmt.Fprintf(out, "Type channel id to enable chat (%s), or press Enter for WebUI-only mode.\n",
        strings.Join(channelIDs, ", "))
    _, _ = fmt.Fprintf(out, "Channel id [%s/WebUI-only]: ", strings.Join(channelIDs, "/"))
    // ... rest unchanged
}
```

---

## 5. Edge Cases

### 5.1 User Later Adds a Channel

**Via CLI:**
```bash
carrier config set openclaw channel.telegram.token=<TOKEN>
carrier config set openclaw channel.telegram.enabled=true
```
The daemon hot-reloads the config and starts the Telegram transport.

**Via WebUI:**
The WebUI settings page already supports adding/editing channel tokens for managed agents. No change needed.

**Via re-running `carrier add`:**
Running `carrier add openclaw` again reuses the existing instance (existing logic). The user can now choose a channel and provide a token. The config is regenerated with the channel enabled.

### 5.2 Bootstrap After WebUI-Only Onboard

With the `needsInitialOnboard` fix (§4.5), `carrier` (bare command) will NOT re-trigger onboarding if a provider is configured. It will just ensure services are running.

### 5.3 Pair Code Handling

When no channel is configured:
- `PairRequired: false` — no pair code is generated or displayed
- The pair-code printing block is gated on `hasChannel`

When channel is selected but token is empty (setup_pending):
- `PairRequired: false` — bot can't connect without a token, so no pair code
- A hint is printed: "Configure channel token to enable chat pairing."

### 5.4 Quiet Mode (`-q`)

In quiet mode, channel selection is automatic:
1. Check env for existing token (e.g., `CARRIER_TELEGRAM_BOT_TOKEN`) → use that channel
2. Check config for existing channel token → reuse
3. No token available → WebUI-only mode (no interactive prompt)

```go
if quiet {
    token, source := resolveManagedChannelToken("telegram")
    if token != "" {
        // use telegram with existing token
    } else {
        // WebUI-only
    }
}
```

### 5.5 Non-Interactive Input (Piped Stdin)

Same as quiet mode: if stdin is not a TTY, auto-select WebUI-only unless a token is available from env/config.

### 5.6 Multiple Channels (Future)

The design naturally supports future multi-channel:
- `promptManagedChannelSelection` already accepts multiple channel IDs
- The user could type `telegram,discord` (comma-separated) and configure both
- Each channel gets its own token prompt
- For now, single-channel is the UX; the architecture is ready

### 5.7 `carrier onboard` Followed by `carrier add`

Common pattern: user runs `carrier onboard` first (sets up Carrier gateway with WebUI-only), then `carrier add openclaw`. The `carrier add` flow should inherit the onboard config's provider credential and let the user choose WebUI-only again for the managed agent.

This already works because provider credential reuse is independent of channel configuration.

### 5.8 Remote Install (`carrier remote add`)

Remote installs use `--sync-channel` flags. If no `--sync-channel` is provided, the remote agent is installed without chat channel config — already correct behavior. No changes needed.

---

## 6. Summary of Files to Modify

| File | Change |
|------|--------|
| `cmd/carrier/main.go` | `runAddManagedAgentTUI`: replace forced channel with `promptManagedChannelSelection` |
| `cmd/carrier/main.go` | `prepareManagedAgentAddArtifacts`: accept empty channelID/token |
| `cmd/carrier/main.go` | `buildManagedPicoClawConfigPayload`: handle empty channel |
| `cmd/carrier/main.go` | `buildManagedOpenClawConfigPayload`: pass `ChannelSetupPending` |
| `cmd/carrier/main.go` | `needsInitialOnboard`: don't require enabled channels |
| `cmd/carrier/main.go` | `promptMinimalChannelSelection`: show all catalog channels |
| `cmd/carrier/main.go` | New function: `promptManagedChannelSelection` |
| `gateway/webui_add.go` | `handleWebUIAdd`: accept empty channel/token (WebUI-only) |
| `gateway/managed_onboard.go` | `prepareManagedOnboard`: allow empty channel |
| `gateway/managed_onboard.go` | `renderZeroClawConfigTOML`: handle empty channelID |
| `gateway/managed_onboard.go` | `buildManagedPicoClawJSONConfigPayload`: handle empty channel |

### New Tests Needed

1. **`main_managed_add_test.go`**: Test `runAddManagedAgentTUI` with empty channel (press Enter)
2. **`main_managed_add_test.go`**: Test `prepareManagedAgentAddArtifacts` with `channelID=""` and `channelToken=""`
3. **`webui_add_round6_test.go`** (or new file): Test `handleWebUIAdd` with `channel: ""` and `channel: "skip"`
4. **`main_onboard_test.go`**: Test `needsInitialOnboard` returns `false` when channels are empty but provider is configured
5. **Config rendering tests**: Verify PicoClaw/OpenClaw/ZeroClaw configs are valid when channel is omitted

---

## 7. Migration / Backward Compatibility

- **No breaking changes.** Users who provide a channel token continue to get the exact same behavior.
- Existing configs with empty `channels` arrays already parse correctly.
- The `needsInitialOnboard` change is the only behavior shift: users with WebUI-only onboard configs will no longer be re-prompted on `carrier` bootstrap.
- The `carrier config set` path for adding channels post-install is existing functionality; we just need to document it clearly in the TUI output.
