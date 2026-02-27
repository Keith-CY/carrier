# profilesync

`profilesync` is the canonical profile synchronization module for Carrier.

Current scope in this phase:

- canonical sync modes (`always_push`, `pull_validate_push`, `manual`)
- OpenClaw / PicoClaw / ZeroClaw artifact manifest baselines
- reconcile primitives with local-first conflict policy support
- git-backed profile storage helpers (`save`, `load`, `rollback`)

This module is intentionally independent from `codeagent/`.

When `git` is unavailable, `profilesync` attempts to install Git automatically (using the host package manager) and continues with git-backed storage. If installation cannot be completed, the operation fails with an explicit error.
