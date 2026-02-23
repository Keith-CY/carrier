# Trust Model: Manifest Fallback Install

## Overview

When the primary npm-based install path is unavailable, the manifest runtime commands fall back to a direct binary download from GitHub Releases. This document describes the security assumptions of that fallback path and reflects the **current** release/installer implementation.

## Artifact and Checksum Flow (Current)

Current release packaging (see `.github/workflows/release.yml`) publishes per-platform ZIP archives and a paired SHA-256 sidecar file:

- `carrier-main-${GITHUB_SHA}-<platform>.zip` (main pre-release flow)
- `carrier-main-${GITHUB_SHA}-<platform>.zip.sha256`

Example:

- `carrier-main-3f2c...-linux-x64.zip`
- `carrier-main-3f2c...-linux-x64.zip.sha256`

The fallback trust model is:

1. Download the ZIP artifact.
2. Download the paired `.sha256` sidecar from the same release tag.
3. Verify the local artifact hash before install/upgrade.
4. Abort if hashes do not match.

## Installer Pinning Model

`scripts/install.sh` is fail-closed for checksum verification by default.

Current behavior:

- resolves `main` HEAD SHA from GitHub API (or accepts explicit `CARRIER_TAG` / `CARRIER_SHA`)
- downloads `carrier-main-<sha>-<label>.zip` and `carrier-main-<sha>-<label>.zip.sha256`
- validates SHA-256 locally (`sha256sum` or `shasum`) before install

If hash validation fails, installation aborts.

## What Is Verified

| Check | Status |
|-------|--------|
| Binary integrity (SHA-256 vs release sidecar) | ✅ Verified |
| Sidecar checksum file authenticity/provenance | ❌ Not cryptographically signed in-repo flow |
| TLS transport integrity | ✅ Via HTTPS |
| Release ref immutability | ⚠️ `main` is mutable; pin exact `main-<sha>` tag for deterministic installs |

## Recommendation: Pinning for Stricter Supply-Chain Control

Operators who require deterministic, auditable deployments should pin exact release assets and expected checksums.

```yaml
# Prefer pinned release assets (example pattern):
install_url: https://github.com/Keith-CY/carrier/releases/download/main-<full_commit_sha>/carrier-main-<full_commit_sha>-linux-x64.zip
checksum_url: https://github.com/Keith-CY/carrier/releases/download/main-<full_commit_sha>/carrier-main-<full_commit_sha>-linux-x64.zip.sha256
```

For scripted installs, pass pinned refs via:
- `CARRIER_TAG=main-<full_commit_sha>`
- `CARRIER_SHA=<full_commit_sha>`
- `CARRIER_LABEL=<platform-label>`

This ensures:
- Exact binary version is known and auditable.
- Upgrades are explicit.
- Rollback is straightforward by changing pinned artifact/version.

## Legacy Note

Older docs and examples may reference `carrier-<platform>.tar.gz` and `checksums.txt`. That is **legacy/non-current** behavior and should not be used as the source of truth.

## Drift Guardrail (Docs ↔ Release/Installer)

When changing release packaging or installer verification behavior, update this file in the same PR and validate alignment against:

- `.github/workflows/release.yml`
- `scripts/install.sh`

A simple reviewer checklist item for release/install PRs:

- [ ] `docs/security-fallback-trust-model.md` still matches artifact naming + checksum verification flow.
