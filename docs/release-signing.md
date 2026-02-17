# Release Artifact Signing

Carrier release artifacts are cryptographically signed so that users can verify
authenticity before installation.

## How It Works

The CI release workflow (`release.yml`) signs every `.zip` and `.sha256` file
using **cosign keyless signing** (OIDC-based, via GitHub Actions' built-in
identity token). No long-lived keys are stored in secrets.

Each artifact `foo.zip` gets a companion `foo.zip.sig` file published alongside
it in the GitHub Release.

## Verifying a Release

Download both the artifact and its `.sig` file, then run:

```bash
./scripts/verify-signature.sh carrier-<commit>-linux-x64.zip
```

The script auto-detects cosign or GPG. For cosign keyless verification the
expected identity is the release workflow in this repository and the OIDC issuer
is `https://token.actions.githubusercontent.com`.

You can also verify manually:

```bash
cosign verify-blob \
  --certificate-identity "https://github.com/Keith-CY/carrier/.github/workflows/release.yml@refs/tags/v<major.minor.patch>" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  --signature carrier-<commit>-linux-x64.zip.sig \
  carrier-<commit>-linux-x64.zip
```

## Signing Methods

| Method | When Used | Details |
|--------|-----------|---------|
| cosign (keyless) | GitHub Actions CI | OIDC token from GitHub, no stored keys |
| GPG | Local / fallback | Requires `GPG_KEY_ID` env var |

See `scripts/sign-artifacts.sh` and `scripts/verify-signature.sh` for
implementation details.
