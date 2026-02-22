# Install/Upgrade Integrity Verification

## Overview

OpenClaw runtime manifests define install and upgrade commands that fetch binaries
from GitHub Releases. This document describes the trust model and verification
steps for those commands.

## Authoritative Installer Path

There is exactly **one** authoritative installer implementation:

- **`daemon/internal/catalog/manifests.go`** — defines the install command via
  `getInstallCommand()`, which delegates to the official upstream installer at
  `https://openclaw.ai/install.sh` (Unix) or `https://openclaw.ai/install.ps1`
  (Windows).

No other installer scripts should exist in the repository. The previously
duplicated `catalog/scripts/install-openclaw.sh` was removed in issue #640 to
eliminate drift risk. Automated tests in `manifests_test.go` enforce this:

- `TestNoStaleInstallerScripts` — fails if any installer script reappears under
  `catalog/scripts/`.
- `TestInstallCommandUsesOfficialURL` — asserts the manifest points to the
  official URL.

## Install Flow

The manifest install command delegates to the official OpenClaw installer:

- **Unix (Linux/macOS/WSL):** `curl -fsSL --proto "=https" --tlsv1.2 "https://openclaw.ai/install.sh" | bash`
- **Windows:** `powershell -NoProfile -Command "irm 'https://openclaw.ai/install.ps1' | iex"`

Integrity verification is handled by the upstream installer.

## What Is Verified

- **Checksum integrity** — The downloaded archive is verified against a
  `.sha256` file published alongside the release artifact. If the checksum
  does not match, the install aborts before extraction.

## What Remains as Operational Risk

- **Trust anchor** — Both the archive and the checksum file are fetched from the
  same origin (GitHub Releases). A compromise of the release pipeline would
  allow an attacker to publish a matching pair. Consider pinning to a specific
  release tag rather than `latest` in hardened environments.

- **TLS-only transport** — Downloads use HTTPS (`curl -fsSL`), but no
  certificate pinning is applied beyond the system trust store.

- **No GPG/Sigstore signature verification** — The current flow does not verify
  a detached signature. Adding `cosign verify-blob` or GPG verification is a
  recommended future enhancement for high-security deployments.

## Trust Model Note: Official Installer Execution

Carrier now prefers invoking OpenClaw's upstream official installer script
(`daemon/internal/catalog/openclaw-installer.sh`) instead of maintaining a
fully in-repo pinned-checksum installer path. This changes the operational
trust boundary:

- **Before (pinned-checksum path)** — Carrier-owned install logic explicitly
  fetched and verified pinned artifact/checksum pairs defined by this repo.
- **Now (official installer path)** — Carrier delegates install logic to the
  upstream installer, inheriting its verification controls and release process.

For threat modeling and operator controls tied to this delegation model, see
[`docs/security-fallback-trust-model.md`](./security-fallback-trust-model.md).

## Recommendations for Hardened Environments

1. Pin a specific release version instead of `latest`.
2. Host release artifacts on an internal mirror with additional integrity checks.
3. Add GPG or Sigstore signature verification to the install command.
4. Run installs in a sandboxed environment and audit the resulting binary before
   promoting to production.
