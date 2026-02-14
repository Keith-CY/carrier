---
name: Release Checklist
about: Verify release readiness across command paths and environments
title: "release: <version> checklist"
labels: release
assignees: ""
---

## Release Checklist

- [ ] TTFH verification
  - [ ] Measure and record time-to-first-handshake in a clean environment.
  - [ ] Confirm TTFH is within the release target.

- [ ] P0 command path verification
  - [ ] Verify install/start/stop/status commands for the P0 path.
  - [ ] Verify failure diagnostics command path and expected artifacts.

- [ ] Known environment matrix
  - [ ] Validate Linux (ubuntu-latest) with Go toolchain from `daemon/go.mod`.
  - [ ] Validate gateway type check path with Bun 1.1.8.
  - [ ] Validate manifest parseability for `catalog/openclaw.manifest.json`.
