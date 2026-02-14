# Install/Upgrade Integrity Verification

## Overview

OpenClaw runtime manifests define install and upgrade commands that fetch binaries
from GitHub Releases. This document describes the trust model and verification
steps for those commands.

## Install Flow

The manifest install command uses a two-tier strategy:

1. **npm-first** — If `npm` is available, install via `npm install -g openclaw`.
   This relies on npm's built-in integrity checks (sha512 in `package-lock.json`).

2. **Binary fallback** — If `npm` is not available, download a platform-specific
   archive from GitHub Releases and verify its SHA-256 checksum:

   ```bash
   ARCHIVE="openclaw-$(uname -s)-$(uname -m).tar.gz"
   curl -fsSL -o "$ARCHIVE" "https://github.com/openclaw/openclaw/releases/latest/download/$ARCHIVE"
   curl -fsSL -o "$ARCHIVE.sha256" "https://github.com/openclaw/openclaw/releases/latest/download/$ARCHIVE.sha256"
   sha256sum -c "$ARCHIVE.sha256"
   tar xzf "$ARCHIVE"
   install -m 755 openclaw /usr/local/bin/openclaw
   ```

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

## Recommendations for Hardened Environments

1. Pin a specific release version instead of `latest`.
2. Host release artifacts on an internal mirror with additional integrity checks.
3. Add GPG or Sigstore signature verification to the install command.
4. Run installs in a sandboxed environment and audit the resulting binary before
   promoting to production.
