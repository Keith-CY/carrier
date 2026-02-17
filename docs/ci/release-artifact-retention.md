# Release Artifact Retention Policy

Source issue: #854

Retention policy is enforced by release workflow and documented here for operators.

## Time-Based Retention

- CI artifacts uploaded by `release.yml` use `retention-days: 14`.
- Applies to packaged ZIP and checksum artifacts uploaded by `actions/upload-artifact`.

## Release Trigger Policy

- Release publication is tag-driven (`v*`) in `.github/workflows/release.yml`.
- Main-branch pushes do not create GitHub Releases automatically.

## Why this rule

- Time-based retention limits storage for workflow artifacts.
- Tag-driven releases prevent noisy non-release artifacts on routine main merges.
