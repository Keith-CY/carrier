# profilesync

`profilesync` is the canonical profile synchronization module for Carrier.

Current scope in this phase:

- canonical sync modes (`always_push`, `pull_validate_push`, `manual`)
- OpenClaw / PicoClaw / ZeroClaw artifact manifest baselines
- reconcile primitives with local-first conflict policy support
- git-backed profile storage helpers (`save`, `load`, `rollback`)

This module is intentionally independent from `codeagent/`.

When `git` is unavailable in constrained environments, the storage helper transparently falls back to a local commit-like history format while keeping the same API semantics.
