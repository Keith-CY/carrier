# Design: Secure Config Sync (Issue #1492)

**Status:** Draft  
**Date:** 2026-03-01  
**Author:** design-1492-opus (subagent)

---

## 1. Problem Statement

Users running Carrier on multiple machines (laptop, desktop, VPS) need their agent configs, provider preferences, and model lists to stay in sync. Today this requires manual copy-paste of `~/.carrier/config.v2.json`.

The previous attempt (PR #1500, closed) implemented local git versioning of the config directory but **committed `config.v2.json` in its entirety to git history**, including cleartext channel secrets (bot tokens, webhook secrets). Even if later `.gitignore`d, git history retains them permanently. This is unacceptable.

### Core Tension

`config.v2.json` is a single file that mixes:
- **Sync-safe data:** model list, default model, base agent settings, channel IDs, enabled flags, transport modes
- **Secrets:** `bot_token`, `webhook_secret`, `webhook_url` (per channel)
- **Credential references:** `credential_ref` fields that point to the separate `credentials.json` / macOS Keychain

The secrets are structurally embedded in the `Channel` struct fields. Any naïve "sync the config file" approach will leak them.

---

## 2. Architecture

### 2.1 Principle: Split at the Data Layer, Not the File Layer

Rather than trying to selectively `.gitignore` files, we **split the config struct itself** into two tiers at serialization time:

| Tier | What | Storage | Synced? |
|------|-------|---------|---------|
| **Profile** | Model list, default model, base agent spec, channel *structure* (ID, enabled, transport mode, allow_from) | `~/.carrier/profiles-repo/config/profile.json` | ✅ Yes |
| **Secrets** | Channel bot_token, webhook_secret, webhook_url; credential store | `~/.carrier/secrets.json` + keychain | ❌ Never |

On save, `configv2.Save()` continues writing `config.v2.json` as the runtime truth. A new `configsync` package produces the split:

```
config.v2.json  (runtime, local-only, .gitignored)
    │
    ├──► profiles-repo/config/profile.json     (safe to commit — no secrets)
    └──► secrets.json                           (never committed — stays local)
         credentials.json                       (never committed — stays local)
```

On load during `pull`, the reverse merge happens: `profile.json` fields are overlaid onto the local `config.v2.json`, and secrets are left untouched from the local store.

### 2.2 What Gets Synced

**Synced (in `profiles-repo/` git repository):**

| File | Contents |
|------|----------|
| `profiles-repo/config/profile.json` | Channels (structure only: id, enabled, transport_mode, allow_from), model_list (model_name, model, provider_id, auth_mode, env_var, credential_ref), default_model, base_agent spec, config_version |
| `profiles-repo/config/metadata.json` | Sync version, last-sync timestamp, device ID, carrier version |
| `profiles-repo/instances/<agentID>/memory-contract.json` | Per-instance memory contracts (existing, from PR #1510) |
| `profiles-repo/.gitignore` | Deny-list as defense in depth (see §3) |

**Never synced (local only):**

| File | Contents |
|------|----------|
| `config.v2.json` | Full runtime config (merged from profile + secrets) |
| `credentials.json` | Provider API keys (managed by `credentialstore`) |
| `secrets.json` | Channel secrets extracted from config |

### 2.3 Unified Repository Structure

```
┌─────────────────────────────────────────────────────────────┐
│  ~/.carrier/                                                │
│                                                             │
│  config.v2.json          ← runtime config (local only)      │
│  credentials.json        ← provider keys (local only)       │
│  secrets.json            ← channel secrets (local only)     │
│                                                             │
│  profiles-repo/          ← UNIFIED git repo (synced)        │
│    .git/                 ← shared git repository            │
│    .gitignore            ← blocks secrets (see §3)          │
│                                                             │
│    config/               ← user-global config sync (#1492)  │
│      profile.json        ← sync-safe config extract         │
│      metadata.json       ← sync metadata                    │
│      secrets.json        ← local only (.gitignored)         │
│                                                             │
│    instances/            ← instance state sync (PR #1510)   │
│      <agentID>/                                             │
│        memory-contract.json  ← per-instance memory          │
│        auth-profiles.json    ← local only (.gitignored)     │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

**Key decision:** Config sync is **integrated into** the existing `profiles-repo/` rather than creating a second git repository. This provides:
- Single remote URL for all sync operations
- Code reuse (git infrastructure from `profilesync`)
- Unified user experience (`carrier sync push` handles both config + instances)

### 2.3.1 Integration with Memory Contract Sync (PR #1510)

This design **extends** the existing `profiles-repo/` git repository introduced in PR #1510, rather than creating a separate sync system.

| Feature | Scope | Location | Managed By |
|---------|-------|----------|------------|
| **Config Sync** (#1492) | User-global settings (model list, channels, defaults) | `profiles-repo/config/profile.json` | `profilesync.SyncUserConfig()` (new) |
| **Memory Contract Sync** (PR #1510) | Per-instance memory contracts | `profiles-repo/instances/<agentID>/memory-contract.json` | `profilesync.SyncInstanceMemoryContract()` (existing) |

**Both systems share:**
- The same git repository (`profiles-repo/.git/`)
- The same remote URL (one `git push` syncs both config + instances)
- The same git infrastructure (atomic writes, rebase, conflict handling from PR #1510)

**When a user syncs across devices:**
1. `carrier sync push` commits both `config/profile.json` (if changed) **and** any `instances/*/memory-contract.json` updates
2. `carrier sync pull` fetches and merges both layers
3. Secrets (`config/secrets.json`, `instances/*/auth-profiles.json`) are `.gitignored` and remain device-local

### 2.4 Extending `profilesync` Package

**New functions added to `profilesync/git_repo.go`:**

```go
// SyncUserConfig commits user-global config to the shared profiles-repo.
// Reuses the same git repo as SyncInstanceMemoryContract (PR #1510).
func SyncUserConfig(profile UserConfigProfile, repoURL, branch, reason string) (string, bool, error) {
    repoRoot, _ := profilesyncRepoRoot()  // reuses existing ~/.carrier/profiles-repo/
    ensureGitRepo(repoRoot)
    
    if repoURL != "" {
        ensureGitRemote(repoRoot, "origin", repoURL)
        checkoutBranch(repoRoot, branch)
        pullRemoteBranch(repoRoot, "origin", branch)
    }
    
    // Write config/profile.json (secrets already stripped by caller)
    configPath := filepath.Join(repoRoot, "config", "profile.json")
    writeFileAtomic(configPath, marshal(profile), 0o600)
    
    // Write config/metadata.json
    metadataPath := filepath.Join(repoRoot, "config", "metadata.json")
    writeFileAtomic(metadataPath, marshal(buildMetadata()), 0o600)
    
    runGit(repoRoot, "add", "config/profile.json", "config/metadata.json")
    
    if changed, _ := gitHasStagedChanges(repoRoot); !changed {
        return gitHead(repoRoot), false, nil
    }
    
    runGit(repoRoot, "commit", "-m", fmt.Sprintf("config(%s): %s", reason, hostname()))
    
    if repoURL != "" {
        pushRemoteBranch(repoRoot, "origin", branch)  // reuses PR #1510's rebase logic
    }
    
    return gitHead(repoRoot), true, nil
}

// PullUserConfig fetches remote config and merges with local secrets.
func PullUserConfig(repoURL, branch string) (*UserConfigProfile, error) {
    // fetch + merge, then read config/profile.json
}
```

**Reused infrastructure from PR #1510:**
- `ensureGitRepo()`, `ensureGitRemote()`, `checkoutBranch()`
- `pullRemoteBranch()` — handles fast-forward and rebase
- `pushRemoteBranch()` — handles non-fast-forward with retry
- `writeFileAtomic()` — temp file + rename for crash safety
- `gitHasStagedChanges()`, `gitHead()`

**Key benefit:** Zero code duplication. The existing git tooling from PR #1510 is production-tested and secure.

---

## 3. Security Model

### 3.1 The Secret Fields Problem

The `Channel` struct in `configv2` has these secret-bearing fields:

```go
type Channel struct {
    BotToken      string `json:"bot_token,omitempty"`       // SECRET
    WebhookSecret string `json:"webhook_secret,omitempty"`  // SECRET  
    WebhookURL    string `json:"webhook_url,omitempty"`     // SEMI-SECRET (contains tokens in some cases)
    // ... non-secret fields ...
}
```

### 3.2 Defense in Depth (4 Layers)

**Layer 1: Structural separation.** The `SyncProfile` struct is a separate Go type that *does not have secret fields*. It is impossible to accidentally serialize secrets into `profile.json` because the type system prevents it:

```go
// SyncProfile is the sync-safe subset of configv2.Config.
// It intentionally omits all secret-bearing fields.
type SyncProfile struct {
    ConfigVersion int                `json:"config_version"`
    Channels      []SyncChannel      `json:"channels"`
    ModelList     []configv2.Model   `json:"model_list"`
    DefaultModel  string             `json:"default_model"`
    BaseAgent     configv2.BaseAgentSpec `json:"base_agent"`
    SyncVersion   int                `json:"sync_version"`
}

// SyncChannel contains ONLY non-secret channel fields.
// bot_token, webhook_secret, webhook_url are deliberately absent.
type SyncChannel struct {
    ID            string   `json:"id"`
    TransportMode string   `json:"transport_mode,omitempty"`
    AllowFrom     []string `json:"allow_from,omitempty"`
    Enabled       bool     `json:"enabled"`
    // NO bot_token, NO webhook_secret, NO webhook_url
}
```

**Layer 2: `.gitignore` deny-list.** The `profiles-repo/.gitignore` file explicitly blocks known secret files as a safety net:

```gitignore
# Defense in depth — block secret-bearing files
config/secrets.json
instances/*/auth-profiles.json

# Never sync runtime config or credential stores
../config.v2.json
../credentials.json

# Generic secret patterns
*.key
*.pem
*.env
*secret*
```

**Note:** This `.gitignore` is created automatically during `profilesync.ensureGitRepo()` (extended to include config sync rules).

**Layer 3: Pre-commit validation.** Before every `git add`, the sync engine:
1. Reads `profile.json` back from disk
2. Scans all string values for patterns matching known secret formats (bot tokens: digit-colon pattern, API keys: `sk-*`, `key-*`, etc.)
3. **Aborts the commit** if any suspicious values are found
4. Logs a clear error: `"SECURITY: profile.json appears to contain secret-like values. Commit aborted."`

```go
func ValidateNoSecrets(profile *SyncProfile) error {
    raw, _ := json.Marshal(profile)
    // Check for known secret patterns
    patterns := []string{
        `\d{8,}:[A-Za-z0-9_-]{30,}`,  // Telegram bot token
        `sk-[A-Za-z0-9]{20,}`,         // OpenAI API key
        `xoxb-`,                        // Slack bot token
        // ... extensible list
    }
    for _, pat := range patterns {
        if regexp.MustCompile(pat).Match(raw) {
            return fmt.Errorf("SECURITY: profile.json contains secret-like value matching %q", pat)
        }
    }
    return nil
}
```

**Layer 4: Git hook.** During `init`, install a `.git/hooks/pre-commit` hook that independently validates no secrets are staged. This catches manual `git add` operations outside of Carrier's CLI.

### 3.3 Secrets Lifecycle

```
                    ┌─ On save ──────────────────────────────────────────┐
                    │                                                   │
config.v2.json ─────┤  Extract secrets → secrets.json                   │
  (runtime)         │  Extract profile → profiles-repo/config/profile.json   │
                    │                                                   │
                    └───────────────────────────────────────────────────┘

                    ┌─ On pull ──────────────────────────────────────────┐
                    │                                                    │
profiles-repo/      ┤  Merge with local secrets.json                      │
config/profile.json │  Write merged → config.v2.json                      │
  (from git)        │                                                    │
                    └────────────────────────────────────────────────────┘
```

Secrets remain exclusively in:
1. `~/.carrier/secrets.json` (file-based, `0600` perms)
2. macOS Keychain (via existing `credentialstore` package)
3. Environment variables

On a new machine after `pull`, the user must re-enter channel secrets:
```
$ carrier config sync pull
✓ Pulled profile from remote (3 channels, 2 models)
⚠ Channel 'telegram' has no local secrets — run: carrier onboard --channel telegram
⚠ Channel 'discord' has no local secrets — run: carrier onboard --channel discord
```

### 3.4 The `secrets.json` Format

```json
{
  "channels": {
    "telegram": {
      "bot_token": "123456:ABC-DEF...",
      "webhook_secret": "...",
      "webhook_url": "https://..."
    },
    "discord": {
      "bot_token": "...",
      "webhook_secret": "..."
    }
  }
}
```

File permissions: `0600`. Never committed. Loaded by `configv2.Load()` alongside `config.v2.json`.

---

## 4. Sync Conflict Resolution Strategy

### 4.1 Conflict Detection

Since `profile.json` is structured JSON (not arbitrary text), we do **field-level 3-way merge** rather than line-level git merge.

Three states:
- **Base:** Last-known common version (stored in `metadata.json` as `last_common_hash`)
- **Local:** Current `profile.json` on disk
- **Remote:** `profile.json` from `git fetch origin/main`

### 4.2 Merge Algorithm

Reuse the existing `profilesync.ReconcileProfiles()` three-way merge, which already supports:
- Field-level comparison
- `ConflictPolicyPreferLocal` (default)
- `ConflictPolicyPreferRemote` (via `--theirs` flag)
- Conflict reporting with field paths

```
carrier config sync pull              → local wins on conflicts (default)
carrier config sync pull --theirs     → remote wins on conflicts
carrier config sync pull --manual     → abort on conflicts, show diff
```

### 4.3 Conflict Scenarios and Resolution

| Scenario | Resolution |
|----------|-----------|
| Both changed `default_model` | Local wins (default) or remote with `--theirs` |
| Local added channel, remote added different channel | Both kept (union of channels by ID) |
| Both modified same channel's `transport_mode` | Local wins / `--theirs` / `--manual` |
| Remote deleted a channel, local modified it | Keep local modification (prefer-local) |
| Model list entries diverge | Merge by `model_name` key; conflict on same-name different values |

### 4.4 Conflict Output

```
$ carrier config sync pull
Fetching remote profile...
⚠ 2 conflicts detected:

  1. default_model: local="gpt-4o" vs remote="claude-sonnet-4-20250514"
     → Keeping local value (use --theirs to accept remote)

  2. channels[telegram].transport_mode: local="polling" vs remote="webhook"
     → Keeping local value

✓ Merged 3 remote changes (model_list additions)
✓ Profile updated. Run `carrier config sync push` to share your resolution.
```

### 4.5 Auto-Sync Conflict Handling

When `auto` mode is enabled, conflicts are **never auto-resolved**. Instead:
1. Auto-push works normally (no conflict possible on push if you're ahead)
2. On pull conflict, auto-sync pauses and logs a warning
3. User must manually run `carrier config sync pull` to resolve

---

## 5. File Structure Changes

### 5.1 Modified File Structure

```
~/.carrier/
├── config.v2.json            # UNCHANGED — runtime config
├── credentials.json          # UNCHANGED — provider credentials
├── secrets.json              # NEW — extracted channel secrets (local only)
└── profiles-repo/            # EXTENDED — unified sync repo
    ├── .git/                 # Shared git repo (existing)
    ├── .gitignore            # EXTENDED — now blocks config/secrets.json + instances/*/auth-profiles.json
    ├── config/               # NEW — user-global config sync
    │   ├── profile.json      # Sync-safe config extract (committed)
    │   ├── metadata.json     # Sync metadata (committed)
    │   └── secrets.json      # Channel secrets (local only, .gitignored)
    └── instances/            # EXISTING — per-instance state (PR #1510)
        └── <agentID>/
            ├── memory-contract.json  # Committed
            └── auth-profiles.json    # Local only, .gitignored
```

### 5.2 Modified Go Packages

**Extended `profilesync/` package:**
```
profilesync/
├── git_repo.go               # EXTENDED — add SyncUserConfig(), PullUserConfig()
├── types.go                  # EXTENDED — add UserConfigProfile, UserConfigMetadata
└── secrets.go                # NEW — secrets.json read/write helpers
```

**Modified `configv2/` package:**
```
configv2/
└── config.go                 # EXTENDED — add ExtractSyncProfile(), MergeProfileIntoConfig()
```

**Modified `cmd/carrier/`:**
```
cmd/carrier/
└── main.go                   # EXTENDED — add `carrier config sync` subcommands
```

**Note:** No new top-level packages created. All functionality is integrated into existing packages.

### 5.3 Changes to `configv2`

Add two new functions to `configv2/config.go`:

```go
// ExtractSyncProfile returns the sync-safe subset of a Config.
// All secret fields are stripped.
func ExtractSyncProfile(cfg *Config) *configsync.SyncProfile { ... }

// MergeProfileIntoConfig applies a SyncProfile's non-secret fields
// onto an existing Config, preserving all local secrets.
func MergeProfileIntoConfig(cfg *Config, profile *configsync.SyncProfile) { ... }
```

### 5.4 Changes to Config Load/Save Flow

```
Current flow:
  Load() → read config.v2.json → return Config

New flow:
  Load() → read config.v2.json → inject secrets from secrets.json → return Config
  Save() → write config.v2.json → extract profile → write profiles-repo/config/profile.json
                                → extract secrets → write secrets.json
                                → (if auto-sync) trigger push
```

The `secrets.json` injection during `Load()` is **optional** — if `secrets.json` doesn't exist, `config.v2.json` is used as-is (backward compatible). Over time, `onboard` will write secrets to `secrets.json` instead of embedding them in `config.v2.json`.

---

## 6. CLI Commands Spec

### 6.1 `carrier config sync init`

```
carrier config sync init --backend=git --remote=<url>

  Initialize config sync with the specified backend.
  
  For git backend:
    1. Creates ~/.carrier/profiles-repo/config/ directory
    2. Runs git init (or clones from --remote if provided)
    3. Extracts current config into profiles-repo/config/profile.json
    4. Extracts secrets into secrets.json (local only)
    5. Writes profiles-repo/.gitignore with deny-list
    6. Installs .git/hooks/pre-commit validation hook
    7. Makes initial commit of profile.json + metadata.json
    8. If --remote provided: git remote add origin <url> && git push

  Flags:
    --backend=git|icloud|gdrive   (required)
    --remote=<url>                (required for git; SSH or HTTPS URL)
    --force                       (reinitialize if sync/ already exists)
    
  Example:
    carrier config sync init --backend=git --remote=git@github.com:user/carrier-config.git
```

### 6.2 `carrier config sync push`

```
carrier config sync push [--message=<msg>]

  Push local config profile to remote.
  
  Steps:
    1. Extract current config.v2.json → profiles-repo/config/profile.json
    2. Validate no secrets in profile.json (Layer 3 check)
    3. git add profile.json metadata.json
    4. git commit -m "<auto or custom message>"
    5. git push origin main
    
  Auto-generated commit messages:
    "sync: update default_model to claude-sonnet-4-20250514"
    "sync: add channel telegram"
    "sync: update model_list (3 models)"
    
  Flags:
    --message=<msg>    Custom commit message (overrides auto-generated)
    --dry-run          Show what would be committed without pushing
    
  Exit codes:
    0  Success
    1  Error (network, git, validation)
    2  Nothing to push (already up to date)
```

### 6.3 `carrier config sync pull`

```
carrier config sync pull [--theirs] [--manual]

  Pull remote config profile and merge into local config.
  
  Steps:
    1. git fetch origin
    2. Load remote profile.json from origin/main
    3. Load base profile.json from last common commit
    4. Three-way merge with local profile.json
    5. Apply merged profile onto config.v2.json (preserving local secrets)
    6. Save updated config.v2.json
    7. Report any missing secrets for new channels
    
  Flags:
    --theirs    On conflict, prefer remote values
    --manual    Abort on conflict and show diff (user must edit manually)
    
  Exit codes:
    0  Success (clean merge or conflicts auto-resolved)
    1  Error
    2  Already up to date
    3  Conflicts detected in --manual mode (user must resolve)
```

### 6.4 `carrier config sync status`

```
carrier config sync status

  Show sync state between local and remote.
  
  Output example:
    Sync backend:  git (git@github.com:user/carrier-config.git)
    Local commit:  a1b2c3d (2 minutes ago)
    Remote commit: e4f5g6h (1 hour ago)
    Status:        Local is 1 commit ahead, 2 commits behind
    
    Changes to push:
      ~ default_model: "gpt-4o" → "claude-sonnet-4-20250514"
      + channel: feishu (enabled)
    
    Changes to pull:
      ~ model_list[1].model: "gpt-4" → "gpt-4-turbo"
      + model_list[3]: deepseek-r1 (new)
    
    Secrets status:
      ✓ telegram: secrets present locally
      ✓ discord: secrets present locally
      ⚠ feishu: no local secrets (from remote, needs onboard)
      
  Exit codes:
    0  In sync
    1  Error
    2  Diverged (needs push/pull)
```

### 6.5 `carrier config sync auto`

```
carrier config sync auto [--enable|--disable]

  Enable/disable automatic sync on config changes.
  
  When enabled:
    - Every configv2.Save() triggers an async push (non-blocking)
    - A background watcher checks for remote changes every 5 minutes
    - Conflicts are never auto-resolved; auto-sync pauses and warns
    
  State stored in:
    profiles-repo/config/metadata.json → { "auto_sync": true, "poll_interval_sec": 300 }
    
  Flags:
    --enable     Enable auto-sync (default if no flag)
    --disable    Disable auto-sync
    --interval   Poll interval in seconds (default 300)
```

---

## 7. Migration Path

### 7.1 Backward Compatibility

- `config.v2.json` remains the primary runtime config file
- All existing `configv2.Load()` / `configv2.Save()` calls continue to work
- `secrets.json` extraction is additive — if it doesn't exist, secrets are read from `config.v2.json` as before
- Sync is opt-in via `carrier config sync init`

### 7.2 Migration Flow for Existing Users

```
$ carrier config sync init --backend=git --remote=git@github.com:me/cfg.git

Initializing config sync...
  ✓ Extracted sync profile (2 channels, 3 models)
  ✓ Extracted secrets to ~/.carrier/secrets.json (0600)
  ✓ Created ~/.carrier/profiles-repo/config/ with git repo
  ✓ Installed pre-commit validation hook
  ✓ Initial commit: "sync: initial profile export"
  ✓ Pushed to git@github.com:me/cfg.git

⚠ Your config.v2.json still contains inline secrets.
  Future `carrier onboard` runs will store secrets in secrets.json.
  This is safe — secrets are never synced.
```

### 7.3 New Machine Setup

```
$ carrier config sync init --backend=git --remote=git@github.com:me/cfg.git

  ✓ Cloned remote config into ~/.carrier/profiles-repo/config/
  ✓ Applied profile (2 channels, 3 models, default_model=claude-sonnet-4-20250514)
  ✓ Written config.v2.json

⚠ Missing secrets for 2 channels:
  - telegram: run `carrier onboard --channel telegram`
  - discord: run `carrier onboard --channel discord`

⚠ Missing credentials for 2 providers:
  - anthropic: run `carrier onboard --provider anthropic`
  - openai: run `carrier onboard --provider openai`
```

---

## 8. Relationship to Existing `profilesync` Package

The existing `profilesync` package (introduced in PR #1510) already provides:
- Git repository at `~/.carrier/profiles-repo/`
- Instance-level memory contract sync (`SyncInstanceMemoryContract()`)
- Git infrastructure (atomic writes, rebase, push retry, conflict handling)

**This design extends `profilesync` rather than creating a separate package.** Benefits:
- Single git repository for all sync operations
- Code reuse (zero duplication of git tooling)
- Unified user experience (one remote URL, one `carrier sync push` command)

| | Instance Sync (PR #1510) | Config Sync (#1492) |
|---|---|---|
| What | Per-instance memory contracts | User-global settings (channels, models) |
| Scope | `profiles-repo/instances/<agentID>/` | `profiles-repo/config/` |
| Function | `SyncInstanceMemoryContract()` (existing) | `SyncUserConfig()` (new, reuses git infra) |
| Secret handling | No secrets in memory contracts | Type-system enforced separation |

Both systems **share the same git repo** (`profiles-repo/.git/`), but operate on different subdirectories and have different data models.

---

## 9. Open Questions

1. **Should `webhook_url` be treated as a secret?** It sometimes contains tokens (Telegram webhook URLs). Safer to treat it as secret. Current design does this.

2. **Encryption option for profile.json?** The issue mentions encrypting sensitive fields. Since we're *not putting secrets in profile.json at all*, encryption is unnecessary for the base design. Could be added as a Layer 5 for paranoid users who want to encrypt even model names.

3. **Should we support multiple remotes?** (e.g., push to both a private git and iCloud) — Defer to v2.

4. **Should `configv2.Save()` always update `profiles-repo/config/profile.json`?** Yes for consistency, but the actual git commit/push should only happen in auto-sync mode or explicit `push`. Writing the file is cheap.

5. **What about `carrier onboard` flow?** It should be updated to write channel secrets to `secrets.json` instead of (or in addition to) `config.v2.json`. During migration, both are supported.

---

## 10. Implementation Phases

### Phase 1: Core Split + Git Backend (Priority 1)
- [ ] `configsync` package with `SyncProfile` / `SyncChannel` types
- [ ] `secrets.json` read/write
- [ ] `ExtractSyncProfile()` / `MergeProfileIntoConfig()`
- [ ] Pre-commit secret validation
- [ ] Git backend implementation
- [ ] CLI: `carrier config sync init/push/pull/status`
- [ ] Integration tests

### Phase 2: Auto-Sync + Polish
- [ ] CLI: `carrier config sync auto`
- [ ] File watcher for auto-push
- [ ] Background pull poller
- [ ] Descriptive auto-commit messages
- [ ] `carrier onboard` writes to `secrets.json`

### Phase 3: Additional Backends
- [ ] iCloud backend (macOS)
- [ ] Google Drive backend (OAuth device flow)

---

## 11. Security Checklist

Before merging any implementation PR, verify:

- [ ] `SyncProfile` struct has NO secret fields (type-level guarantee)
- [ ] `profiles-repo/.gitignore` blocks `config.v2.json`, `credentials.json`, `secrets.json`
- [ ] Pre-commit validation scans for secret patterns
- [ ] Git pre-commit hook installed during `init`
- [ ] `secrets.json` has `0600` permissions
- [ ] `git log` of `sync/` repo contains no secrets after full test cycle
- [ ] `carrier config sync push --dry-run` output contains no secrets
- [ ] New machine `pull` correctly reports missing secrets
- [ ] Auto-sync never auto-resolves conflicts
