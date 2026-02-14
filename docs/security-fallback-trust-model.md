# Trust Model: Manifest Fallback Install

## Overview

When the primary npm-based install path is unavailable, the manifest runtime commands fall back to a direct binary download from GitHub Releases. This document describes the security assumptions of that fallback path.

## Artifact and Checksum Flow

During a fallback install or upgrade, two artifacts are fetched:

1. **Binary archive** — the platform-specific release tarball (e.g., `carrier-linux-x64.tar.gz`).
2. **Checksum file** — a SHA-256 digest file published alongside the release (e.g., `checksums.txt`).

The local install script:

1. Downloads both files from the GitHub Releases URL.
2. Computes the SHA-256 hash of the downloaded binary archive.
3. Compares the computed hash against the value in the checksum file.
4. Aborts installation if the hashes do not match.

## What Is Verified

| Check | Status |
|-------|--------|
| Binary integrity (SHA-256 vs published checksum) | ✅ Verified |
| Checksum file authenticity (signature/provenance) | ❌ Not verified — trust is delegated to HTTPS + GitHub |
| TLS transport integrity | ✅ Via HTTPS |
| Release tag immutability | ⚠️ GitHub tags can be force-pushed by repo admins |

## The `releases/latest` Tradeoff

By default, manifest commands resolve the install URL via `https://github.com/<owner>/<repo>/releases/latest`, which always points to the most recent published release.

**Advantages:**
- Operators always get the newest version without config changes.
- Simplifies zero-touch upgrade flows.

**Risks:**
- A compromised release or force-pushed tag would be picked up automatically.
- No visibility into version drift across deployments.

## Recommendation: Pinning for Stricter Supply-Chain Control

Operators who require deterministic, auditable deployments should **pin the exact release tag** in their manifest configuration instead of using `releases/latest`:

```yaml
# Instead of:
install_url: https://github.com/Keith-CY/carrier/releases/latest/download/carrier-linux-x64.tar.gz

# Pin to a specific version:
install_url: https://github.com/Keith-CY/carrier/releases/download/v0.3.2/carrier-linux-x64.tar.gz
```

This ensures:
- The exact binary version is known and auditable.
- Upgrades are explicit and require a config change.
- Rollback is straightforward (change the pinned tag).

For maximum assurance, download the checksum file separately and verify it out-of-band before deploying to production hosts.
