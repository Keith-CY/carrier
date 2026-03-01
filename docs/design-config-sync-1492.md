# Design: Secure Config Sync (Issue #1492)

**Status:** Draft (Revised 2026-03-01)  
**Date:** 2026-03-01  
**Author:** design-1492-opus (revised based on architecture clarification)

---

## 1. Problem Statement

Carrier manages agent instances (openclaw, picoclaw, etc.) on remote machines via SSH. The configuration for each instance lives in `profiles-repo/instances/<agentID>/`, and needs to be synced to the remote instance securely.

### Current Pain Points

1. **Instance configs contain credentials** — If naïvely committed to git, API keys are exposed
2. **Multi-device Carrier setup** — Users want to sync their Carrier configuration (provider list, instance definitions) across laptop/desktop without re-entering credentials
3. **No clear separation** — Which files are safe to git commit vs. which must stay local?

### Core Tension

An instance config needs both:
- **Structure** (model, provider_id, workspace path) — safe to commit
- **Secrets** (API keys, bot tokens) — must NOT be in git history

---

## 2. Architecture Overview

### 2.1 Two Sync Scenarios

| Scenario | What | How | Secrets? |
|----------|------|-----|----------|
| **Carrier → Instance** | Config + workspace files + secrets | Git (config) + SSH rsync (secrets) | ✅ Synced via rsync |
| **Carrier multi-device** | Instance definitions + provider list | Git push/pull (GitHub) | ❌ Local only, re-enter on new device |

### 2.2 Secrets Separation (OpenClaw Pattern)

**Inspired by OpenClaw's credential management:**

```json
// profiles-repo/instances/openclaw/config.json (safe to git commit)
{
  "model": "anthropic/claude-sonnet-4-6",
  "providers": {
    "anthropic": {
      "apiKey": {
        "source": "file",
        "provider": "carrier_file",
        "id": "/providers/anthropic/apiKey"  // ← JSON pointer reference
      }
    }
  },
  "secrets": {
    "providers": {
      "carrier_file": {
        "source": "file",
        "mode": "json",
        "path": "./carrier-secrets.json"
      }
    }
  }
}
```

```json
// profiles-repo/instances/openclaw/carrier-secrets.json (rsync only, NOT in git)
{
  "providers": {
    "anthropic": {
      "apiKey": "sk-ant-api-xxx..."
    }
  }
}
```

**Key insight:** `config.json` uses **JSON pointer references** to secrets, never storing them inline. The actual secrets live in `carrier-secrets.json`, which is `.gitignored` and only transferred via SSH rsync.

---

## 3. Directory Structure

### 3.1 Carrier (Gateway) Side

```
~/.carrier/
├── credentials.json                    # Global provider credentials (local only)
├── carrier-secrets.json                # Extracted secrets for instances (local only)
└── profiles-repo/
    ├── .git/
    ├── .gitignore                      # Blocks all *-secrets.json files
    │
    ├── config/                         # Carrier global config (future)
    │   └── profile.json                # Model list, channel list (no secrets)
    │
    └── instances/
        ├── openclaw/
        │   ├── config.json             # ✅ Git: structure config (with secret refs)
        │   ├── carrier-secrets.json    # ❌ Git: rsync only
        │   └── workspace/
        │       ├── SOUL.md             # ✅ Git: user files
        │       └── AGENTS.md
        │
        └── picoclaw/
            ├── config.json
            ├── carrier-secrets.json    # ❌ Git
            └── workspace/
```

### 3.2 Instance (Remote Agent) Side

```
/path/to/instance/
├── config.json                         # Pulled via git
├── carrier-secrets.json                # Synced via rsync from Carrier
└── workspace/
    ├── .git/
    ├── SOUL.md
    └── AGENTS.md
```

**Instance only needs:**
- `config.json` (structure + secret references)
- `carrier-secrets.json` (actual secrets, rsync'd from Carrier)

---

## 4. Sync Flows

### 4.1 Carrier → Instance: Initial Setup

```bash
$ carrier add openclaw --remote ssh://vps.example.com

[1/5] Creating instance config...
  ✓ profiles-repo/instances/openclaw/config.json
  ✓ profiles-repo/instances/openclaw/carrier-secrets.json (local)

[2/5] Committing to git...
  $ git add instances/openclaw/config.json instances/openclaw/workspace/
  $ git commit -m "carrier: add instance openclaw"

[3/5] Pushing config to remote...
  $ git push ssh://vps.example.com:~/.carrier/profiles-repo main

[4/5] Syncing secrets (rsync)...
  $ rsync -avz instances/openclaw/carrier-secrets.json \
      vps.example.com:~/.carrier/profiles-repo/instances/openclaw/

[5/5] Starting instance...
  $ ssh vps.example.com "cd ~/.carrier && carrier-daemon start openclaw"

✓ Instance openclaw running at vps.example.com
```

### 4.2 Carrier → Instance: Config Update

```bash
$ carrier config set openclaw model=claude-opus-4-6

Carrier:
  1. Update config.json
  2. git commit + push

Instance daemon (via SSH connection):
  1. Detects git update (polling or webhook)
  2. git pull
  3. Reload config

No secrets transfer needed (unless provider changed)
```

### 4.3 Carrier → Instance: Secrets Update

```bash
$ carrier onboard --provider openai --instance openclaw
Enter API key: sk-xxx

Carrier:
  1. Update carrier-secrets.json (local)
  2. rsync → instance

Instance:
  1. Receive carrier-secrets.json
  2. Reload secrets
```

### 4.4 Carrier Multi-Device: Laptop → Desktop

```bash
# Laptop Carrier
$ git push github.com/user/carrier-config

# Desktop Carrier (new machine)
$ carrier install
$ git clone github.com/user/carrier-config ~/.carrier/profiles-repo

Pulled:
  ✓ instances/openclaw/config.json (structure)
  ✓ instances/openclaw/workspace/ (user files)
  ✗ carrier-secrets.json (.gitignored)

$ carrier onboard --provider anthropic
Enter API key: sk-ant-xxx
✓ Created local credentials.json

$ carrier sync instance openclaw --to ssh://vps
  → rsync new carrier-secrets.json to instance
```

---

## 5. .gitignore Rules

### profiles-repo/.gitignore

```gitignore
# Carrier-level secrets (never sync)
../credentials.json
../carrier-secrets.json

# Instance-level secrets (rsync only, NOT git)
instances/*/carrier-secrets.json

# Instance runtime state
instances/*/logs/
instances/*/.cache/
instances/*/tmp/

# Generic secret patterns (defense in depth)
*secret*.json
*credential*.json
*.key
*.pem
```

**Critical:** `carrier-secrets.json` must NEVER enter git history. The credential references in `config.json` are safe, but the actual values in `carrier-secrets.json` are secret-bearing.

---

## 6. Security Model

### 6.1 Four Layers of Defense

**Layer 1: Structural Separation (OpenClaw pattern)**
- Secrets stored in separate file (`carrier-secrets.json`)
- Config uses JSON pointer references only
- Cannot accidentally inline secrets

**Layer 2: .gitignore**
- All `*-secrets.json` files blocked
- Belt-and-suspenders: even if someone tries `git add -f`, next layer catches it

**Layer 3: Pre-commit Hook (Optional)**
```bash
#!/bin/sh
# .git/hooks/pre-commit
if git diff --cached --name-only | grep -q 'secrets\.json'; then
  echo "ERROR: Attempting to commit secrets file!"
  exit 1
fi
```

**Layer 4: Secret Scanner (Paranoid mode)**
```go
// Before git commit, scan staged files for secret patterns
func validateNoSecretsInStaged() error {
    staged := gitDiffCached()
    patterns := []string{
        `sk-[a-zA-Z0-9]{20,}`,      // OpenAI/Anthropic keys
        `\d{8,}:[A-Za-z0-9_-]{30,}`, // Telegram bot tokens
        // ... extensible
    }
    for _, pat := range patterns {
        if regexp.MustCompile(pat).Match(staged) {
            return errors.New("SECURITY: staged files contain secret-like values")
        }
    }
    return nil
}
```

### 6.2 Secrets Lifecycle

```
┌─────────────────────────────────────────────────┐
│  User Input                                     │
│  $ carrier onboard --provider anthropic         │
│  Enter API key: sk-ant-xxx                      │
└─────────────────────────────────────────────────┘
           ↓
┌─────────────────────────────────────────────────┐
│  Carrier Storage                                │
│  credentials.json: {"anthropic": "sk-ant-xxx"}  │
└─────────────────────────────────────────────────┘
           ↓ (when instance is created/updated)
┌─────────────────────────────────────────────────┐
│  Instance Secrets File                          │
│  carrier-secrets.json (rsync'd to instance)     │
└─────────────────────────────────────────────────┘
           ↓
┌─────────────────────────────────────────────────┐
│  Instance Runtime                               │
│  Reads config.json (ref) → carrier-secrets.json │
│  Loads API key into memory                      │
└─────────────────────────────────────────────────┘
```

**Key property:** Secrets never touch git. Only transferred via SSH (encrypted in transit).

---

## 7. Implementation Plan

### Phase 1: Core Sync Infrastructure

**Files to modify:**
- `profilesync/git_repo.go` — Add `.gitignore` generation
- `profilesync/instance_sync.go` (NEW) — Rsync secrets to instances
- `profilesync/types.go` — Add InstanceSecrets struct

**New functions:**
```go
// Sync instance config (git) + secrets (rsync) to remote
func SyncInstanceToRemote(instanceID, sshHost string) error {
    // 1. Git push config.json + workspace
    if err := gitPushInstance(instanceID, sshHost); err != nil {
        return err
    }
    
    // 2. Rsync carrier-secrets.json (out-of-band)
    if err := rsyncSecrets(instanceID, sshHost); err != nil {
        return err
    }
    
    return nil
}

// Rsync secrets file via SSH
func rsyncSecrets(instanceID, sshHost string) error {
    localPath := filepath.Join(profilesyncRepoRoot(), "instances", instanceID, "carrier-secrets.json")
    remotePath := fmt.Sprintf("%s:~/.carrier/profiles-repo/instances/%s/carrier-secrets.json", sshHost, instanceID)
    
    cmd := exec.Command("rsync", "-avz", "-e", "ssh", localPath, remotePath)
    return cmd.Run()
}
```

### Phase 2: Carrier Multi-Device Sync

**Files to modify:**
- `profilesync/git_repo.go` — Extend SyncUserConfig (already planned)
- `cmd/carrier/main.go` — Add `carrier sync` subcommands

**CLI:**
```bash
$ carrier sync init <remote-url>
  → git remote add origin <url>
  → git push origin main

$ carrier sync pull
  → git pull origin main
  → List missing secrets (if any)

$ carrier sync push
  → git push origin main
```

### Phase 3: Secret Scanner (Optional Hardening)

**Files to add:**
- `profilesync/secret_scanner.go`
- `.git/hooks/pre-commit` (auto-installed)

---

## 8. User Experience

### 8.1 New Instance Creation

```bash
$ carrier add openclaw --remote ssh://vps

  Step 1: Creating instance config...
  ✓ profiles-repo/instances/openclaw/config.json
  ✓ profiles-repo/instances/openclaw/carrier-secrets.json

  Step 2: Select provider for openclaw:
    1. anthropic (sk-ant-***xyz)
    2. openai (sk-***abc)
    3. [Enter new credential]
  Choice: 1

  Step 3: Pushing to remote...
  ✓ Git: config + workspace
  ✓ Rsync: carrier-secrets.json

  Step 4: Starting instance...
  ✓ Instance openclaw running at vps

Done in 8.2s
```

### 8.2 Multi-Device Setup

```bash
# Desktop (new Carrier install)
$ carrier init --from-git github.com/user/carrier-config

  Cloning config repository...
  ✓ 3 instances found: openclaw, picoclaw, zeroclaw

  Missing credentials for providers:
    - anthropic (used by openclaw, picoclaw)
    - openai (used by zeroclaw)

  Enter credentials now? [Y/n]: y

  Provider: anthropic
  API key: sk-ant-xxx
  ✓ Saved

  Provider: openai
  API key: sk-xxx
  ✓ Saved

  Sync secrets to instances? [Y/n]: y
  ✓ Rsync'd to openclaw@vps
  ✓ Rsync'd to picoclaw@vps
  ✓ Rsync'd to zeroclaw@vps

All instances ready.
```

---

## 9. Relationship to PR #1510

PR #1510 introduced `SyncInstanceMemoryContract()` for syncing per-instance memory state. This design extends that foundation:

| Feature | PR #1510 (Existing) | Issue #1492 (New) |
|---------|---------------------|-------------------|
| **Sync target** | Memory contracts | Instance config + secrets + workspace |
| **Transport** | Git only | Git (config) + rsync (secrets) |
| **Security** | No secrets in memory contracts | Explicit secrets separation |
| **Scope** | `instances/<agentID>/memory-contract.json` | `instances/<agentID>/config.json` + `carrier-secrets.json` + `workspace/` |

**Reused infrastructure:**
- `profilesyncRepoRoot()` — same repo
- `ensureGitRepo()`, `gitPush()`, `gitPull()` — git operations
- `writeFileAtomic()` — safe file writes

**New additions:**
- `rsyncSecrets()` — SSH-based secret transfer
- `.gitignore` management — automated secret blocking
- Carrier multi-device sync — user-facing git operations

---

## 10. Open Questions & Future Work

### Q1: Should rsync be built-in or external tool?

**Option A: Shell out to rsync**
```go
exec.Command("rsync", "-avz", "-e", "ssh", src, dst)
```
✅ Simple, uses well-tested tool  
❌ Requires rsync installed (not default on Windows)

**Option B: Implement in Go**
```go
import "github.com/studio-b12/gowebdav" // or similar
```
✅ Cross-platform  
❌ More complexity, less battle-tested

**Decision:** Start with option A (shell out), add option B if Windows users complain.

### Q2: Credential encryption at rest?

Currently `carrier-secrets.json` is plaintext (but `0600` permissions). Should we encrypt it?

**Option A: No encryption (current)**
- Relies on filesystem permissions
- Simpler implementation
- Risk: root user or disk theft

**Option B: Encrypt with user passphrase**
```bash
$ carrier onboard --provider anthropic
Enter API key: xxx
Enter encryption passphrase: [hidden]
✓ Encrypted carrier-secrets.json with passphrase
```
- More secure
- Requires passphrase on every Carrier start (annoying)

**Decision:** Start with option A, add B as opt-in feature (`carrier secure enable`)

### Q3: Git-crypt support?

Should we support git-crypt as an alternative to rsync?

**Pros:**
- Transparent encryption in git
- Works with standard git workflows

**Cons:**
- Windows support poor
- GPG key management complexity
- If key lost, secrets unrecoverable

**Decision:** Document as "advanced option" but don't recommend. Prefer rsync.

---

## 11. Success Criteria

This design succeeds if:

1. ✅ **Zero secrets in git history** — `git log` of profiles-repo never shows API keys
2. ✅ **Instance startup requires config + secrets** — Both must be present
3. ✅ **Multi-device Carrier sync works** — User can clone profiles-repo on new machine and re-enter secrets once
4. ✅ **Existing instances unaffected** — Backward compatible with instances created before this feature
5. ✅ **No manual rsync required** — `carrier add --remote` handles everything

---

## 12. Testing Plan

### Unit Tests
- `TestGitignoreBlocksSecrets` — Verify .gitignore rules
- `TestConfigUsesReferences` — Verify config.json never has inline secrets
- `TestRsyncSecrets` — Mock SSH, verify rsync command

### Integration Tests
- `TestCarrierToInstanceSync` — Full flow: add instance → verify remote has config + secrets
- `TestMultiDeviceSync` — Clone on new machine → verify structure but no secrets

### Security Tests
- `TestSecretScannerDetectsLeaks` — Staged files with API keys should error
- `TestGitHistoryClean` — After full test cycle, `git log -p` contains no secrets

---

**End of Design Document**
