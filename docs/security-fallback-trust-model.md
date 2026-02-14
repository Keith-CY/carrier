# Trust Model: Manifest Fallback Install

## Overview

When the primary npm-based install path is unavailable, the manifest runtime commands fall back to a direct binary download from GitHub Releases. This document describes the security assumptions of that fallback path and reflects the **current** release/installer implementation.

## Artifact and Checksum Flow (Current)

Current release packaging (see `.github/workflows/release.yml`) publishes per-platform ZIP archives and a paired SHA-256 sidecar file:

- `carrier-${GITHUB_SHA}-<platform>.zip`
- `carrier-${GITHUB_SHA}-<platform>.zip.sha256`

Example:

- `carrier-3f2c...-linux-x64.zip`
- `carrier-3f2c...-linux-x64.zip.sha256`

The fallback trust model is:

1. Download the ZIP artifact.
2. Obtain the expected SHA-256 digest from trusted release metadata (or a pinned deployment source).
3. Verify the local artifact hash before install/upgrade.
4. Abort if hashes do not match.

## Installer Pinning Model

`catalog/scripts/install-openclaw.sh` is fail-closed and does **not** consume `checksums.txt`.

Instead, it requires:

- `OPENCLAW_CHECKSUM` to be provided explicitly (pinned expected digest), and
- local SHA-256 verification to match that pinned value.

If `OPENCLAW_CHECKSUM` is missing or mismatched, installation fails.

## What Is Verified

| Check | Status |
|-------|--------|
| Binary integrity (SHA-256 vs pinned expected digest) | ✅ Verified |
| Sidecar checksum file authenticity/provenance | ❌ Not cryptographically signed in-repo flow |
| TLS transport integrity | ✅ Via HTTPS |
| Release ref immutability | ⚠️ Mutable refs are risky; pin exact tag/SHA-derived assets |

## The `releases/latest` Tradeoff

By default, manifest commands may resolve install URLs via `https://github.com/<owner>/<repo>/releases/latest`, which always points to the most recent published release.

**Advantages:**
- Operators get the newest version without config changes.
- Simplifies zero-touch upgrade flows.

**Risks:**
- A compromised release or mutable ref could be picked up automatically.
- No explicit version control in deployment history.

## Recommendation: Pinning for Stricter Supply-Chain Control

Operators who require deterministic, auditable deployments should pin exact release assets and expected checksums.

```yaml
# Avoid mutable latest pointers in production:
install_url: https://github.com/Keith-CY/carrier/releases/latest/download/carrier-linux-x64.tar.gz

# Prefer pinned release assets (example pattern):
install_url: https://github.com/Keith-CY/carrier/releases/download/main-<sha>/carrier-<sha>-linux-x64.zip
# and pin expected SHA-256 separately (e.g., OPENCLAW_CHECKSUM or deployment metadata)
```

This ensures:
- Exact binary version is known and auditable.
- Upgrades are explicit.
- Rollback is straightforward by changing pinned artifact/version.

## Legacy Note

Older docs and examples may reference `carrier-<platform>.tar.gz` and `checksums.txt`. That is **legacy/non-current** behavior and should not be used as the source of truth.

## Drift Guardrail (Docs ↔ Release/Installer)

When changing release packaging or installer verification behavior, update this file in the same PR and validate alignment against:

- `.github/workflows/release.yml`
- `catalog/scripts/install-openclaw.sh`

A simple reviewer checklist item for release/install PRs:

- [ ] `docs/security-fallback-trust-model.md` still matches artifact naming + checksum verification flow.
