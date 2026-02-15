# Artifact Signing

This document describes how to sign release artifacts using the provided signing scripts.

## Overview

The `scripts/sign-artifacts.sh` script provides automated signing of release artifacts using either:

- **cosign** (preferred): Keyless OIDC signing for GitHub Actions
- **GPG** (fallback): Traditional GPG signing with a local key

The script automatically detects which tool is available and uses the appropriate method.

## Local Usage

### Prerequisites

Install either cosign or GPG:

```bash
# Install cosign
# See: https://docs.sigstore.dev/cosign/installation/
brew install cosign  # macOS
# or
go install github.com/sigstore/cosign/v2/cmd/cosign@latest

# Install GPG (usually pre-installed on Linux/macOS)
brew install gnupg  # macOS
apt-get install gnupg  # Debian/Ubuntu
```

### Basic Usage

```bash
# Sign all artifacts in a directory
./scripts/sign-artifacts.sh ./dist

# Force a specific signing method
SIGNING_METHOD=gpg ./scripts/sign-artifacts.sh ./dist

# Use a specific GPG key
GPG_KEY_ID=ABCD1234 ./scripts/sign-artifacts.sh ./dist
```

### Environment Variables

- `SIGNING_METHOD`: Force specific method (`cosign` or `gpg`)
- `GPG_KEY_ID`: GPG key ID to use for signing
- `GPG_PASSPHRASE`: GPG passphrase for automated signing (CI use)

## CI Integration (GitHub Actions)

### Using cosign with Keyless Signing

Cosign supports keyless signing in GitHub Actions using OIDC tokens. This is the recommended approach as it requires no secret management.

#### Required Permissions

Add the following permission to your workflow:

```yaml
permissions:
  id-token: write  # Required for cosign keyless signing
  contents: write  # Required for uploading release artifacts
```

#### Example Workflow Step

```yaml
- name: Install cosign
  uses: sigstore/cosign-installer@v3

- name: Sign release artifacts
  run: |
    ./scripts/sign-artifacts.sh ./dist
  env:
    SIGNING_METHOD: cosign

- name: Upload signatures
  uses: actions/upload-artifact@v4
  with:
    name: signatures
    path: ./dist/*.sig
```

### Using GPG Signing

For GPG signing in CI, you need to:

1. Generate a GPG key pair
2. Export the private key
3. Add it as a GitHub secret
4. Import it in your workflow

#### Setup GPG Secrets

```bash
# Generate a new GPG key (or use existing)
gpg --full-generate-key

# Export the private key
gpg --armor --export-secret-keys YOUR_KEY_ID > private-key.asc

# Add to GitHub secrets:
# - GPG_PRIVATE_KEY: Contents of private-key.asc
# - GPG_PASSPHRASE: Key passphrase
# - GPG_KEY_ID: Your key ID
```

#### Example Workflow Step

```yaml
- name: Import GPG key
  run: |
    echo "${{ secrets.GPG_PRIVATE_KEY }}" | gpg --batch --import
    
- name: Sign release artifacts
  run: |
    ./scripts/sign-artifacts.sh ./dist
  env:
    SIGNING_METHOD: gpg
    GPG_KEY_ID: ${{ secrets.GPG_KEY_ID }}
    GPG_PASSPHRASE: ${{ secrets.GPG_PASSPHRASE }}

- name: Upload signatures
  uses: actions/upload-artifact@v4
  with:
    name: signatures
    path: ./dist/*.sig
```

## Complete Workflow Example

Here's a complete example workflow that builds, signs, and releases artifacts:

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

permissions:
  id-token: write
  contents: write

jobs:
  build-and-sign:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Build artifacts
        run: |
          # Your build steps here
          mkdir -p dist
          # ... build commands ...
      
      - name: Install cosign
        uses: sigstore/cosign-installer@v3
      
      - name: Sign artifacts
        run: ./scripts/sign-artifacts.sh ./dist
        env:
          SIGNING_METHOD: cosign
      
      - name: Create Release
        uses: softprops/action-gh-release@v1
        with:
          files: |
            dist/*
            dist/*.sig
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

## Verifying Signatures

### Verify cosign Signatures

```bash
# Download the artifact and signature
# Then verify:
cosign verify-blob \
  --certificate-identity-regexp '.*' \
  --certificate-oidc-issuer-regexp '.*' \
  --signature artifact.bin.sig \
  artifact.bin
```

### Verify GPG Signatures

```bash
# Import the public key first
gpg --import public-key.asc

# Verify the signature
gpg --verify artifact.bin.sig artifact.bin
```

## Testing

Run the test suite to verify the signing script works correctly:

```bash
./scripts/test-sign-artifacts.sh
```

The test script will:
- Verify the signing script exists and is executable
- Test error handling
- Create test artifacts and sign them
- Verify signatures are created correctly

## Security Considerations

### cosign (Keyless)

- **Pros**: No secret management required, transparent audit log
- **Cons**: Requires network access to Sigstore infrastructure
- **Best for**: GitHub Actions and automated CI/CD

### GPG

- **Pros**: Works offline, traditional and widely supported
- **Cons**: Requires secure key management, manual key distribution
- **Best for**: Local development and air-gapped environments

## Troubleshooting

### cosign: "failed to get token"

This error occurs when cosign can't access the OIDC token. Ensure:
- Your workflow has `id-token: write` permission
- You're running in a supported CI environment (GitHub Actions)

### GPG: "signing failed: No secret key"

This means GPG can't find the signing key. Ensure:
- The GPG key is imported: `gpg --list-secret-keys`
- The `GPG_KEY_ID` matches an available key

### No signatures created

Check that:
- The artifacts directory contains files
- The signing tool (cosign or gpg) is installed and in PATH
- There are no existing `.sig` files (they are skipped)

## References

- [cosign Documentation](https://docs.sigstore.dev/cosign/overview/)
- [Sigstore GitHub Actions](https://github.com/sigstore/cosign-installer)
- [GPG Documentation](https://gnupg.org/documentation/)
- [GitHub OIDC Tokens](https://docs.github.com/en/actions/deployment/security-hardening-your-deployments/about-security-hardening-with-openid-connect)
