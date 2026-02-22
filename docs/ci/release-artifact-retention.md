# Release Artifact Retention Policy

Source issue: #854

Retention policy is enforced by release workflow and documented here for operators.

## Time-Based Retention

- CI artifacts uploaded by `release.yml` use `retention-days: 14`.
- Applies to packaged ZIP and checksum artifacts uploaded by `actions/upload-artifact`.

## Release Trigger Policy

- Published release events (`release.published`) produce release-mode artifacts.
- Pushes to `main` produce prerelease artifacts under tag `main-<full_commit_sha>`.
- Pull requests build test packages as workflow artifacts only (no GitHub Release publication).

## Why this rule

- Time-based retention limits storage for workflow artifacts.
- Main push prereleases provide ready-to-test binaries for environments such as EC2.
