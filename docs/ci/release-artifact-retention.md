# Release Artifact Retention Policy

Source issue: #854

Retention policy is enforced by release workflow and documented here for operators.

## Time-Based Retention

- CI artifacts uploaded by `release.yml` use `retention-days: 14`.
- Applies to packaged ZIP and checksum artifacts uploaded by `actions/upload-artifact`.

## Count-Based Retention

- For `main-<sha>` releases, workflow keeps the newest `20` releases.
- Older `main-*` releases are pruned automatically after a successful new release.

## Why both rules

- Time-based retention limits storage for workflow artifacts.
- Count-based retention limits release history growth while preserving rollback headroom.
