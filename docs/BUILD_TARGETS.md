# Build Targets

This document describes the build target matrix for Carrier, including currently supported platforms and how to add optional/experimental targets.

## Current Build Matrix

The release workflow (`.github/workflows/release.yml`) currently builds both CLI and App variants for the following platforms:

### Core Targets (Always Built)

| OS      | Architecture | Label           | Binary Extension | Notes                    |
|---------|--------------|-----------------|------------------|--------------------------|
| Linux   | amd64        | linux-x64       | (none)           | Primary Linux target     |
| Linux   | arm64        | linux-arm64     | (none)           | ARM64 Linux (e.g., Pi)   |
| macOS   | amd64        | darwin-x64      | (none)           | Intel Macs               |
| macOS   | arm64        | darwin-arm64    | (none)           | Apple Silicon (M1/M2/M3) |
| Windows | amd64        | windows-x64     | `.exe`           | 64-bit Windows           |
| Windows | arm64        | windows-arm64   | `.exe`           | ARM64 Windows (Surface)  |

These targets are **always built** on every release. They cover the vast majority of deployment scenarios.

For each target, release artifacts are now published as:

- CLI: `carrier-<tag>-<label>.zip`
- App: `carrier-app-<tag>-<label>.zip`

## Optional/Experimental Targets

The following targets are **not built by default** but can be enabled by modifying the build matrix. They are considered experimental or niche:

### Linux RISC-V 64-bit (`linux-riscv64`)

- **Use case**: RISC-V Linux systems (embedded, experimental hardware)
- **Status**: Experimental (Go supports RISC-V, but adoption is limited)
- **Testing**: No CI testing for this platform currently

### Other Potential Targets

- `linux-386` — 32-bit Linux (legacy systems)
- `linux-armv7` — 32-bit ARM Linux (older Raspberry Pi models)
- `freebsd-amd64` — FreeBSD (server deployments)
- `openbsd-amd64` — OpenBSD (security-focused deployments)

## Adding a New Build Target

To add a new target to the build matrix:

### 1. Identify the Target Triple

Determine the Go `GOOS` and `GOARCH` values:

```bash
# Example: RISC-V Linux
GOOS=linux
GOARCH=riscv64
```

See [Go's supported platforms](https://go.dev/doc/install/source#environment) for valid combinations.

### 2. Update the Build Matrix

**⚠️ Important**: You need **workflow write permissions** to modify `.github/workflows/release.yml`. If your token lacks this scope, coordinate with a maintainer.

Edit `.github/workflows/release.yml` and add to the `strategy.matrix.include` section:

```yaml
- goos: linux
  goarch: riscv64
  label: linux-riscv64
  ext: ""
  archive_ext: zip
```

**Matrix fields:**

- `goos`: Target operating system (`linux`, `darwin`, `windows`, etc.)
- `goarch`: Target architecture (`amd64`, `arm64`, `riscv64`, `386`, etc.)
- `label`: Human-readable platform label (used in artifact names)
- `ext`: Binary file extension (`.exe` for Windows, empty otherwise)
- `archive_ext`: Archive format (`zip` is default, could be `tar.gz`)

### 3. Verify Go Toolchain Support

Check that your Go version supports the target:

```bash
cd daemon
go tool dist list | grep -i riscv
# Should show: linux/riscv64
```

### 4. Test Locally

Build the binary locally to ensure it compiles:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=riscv64 \
  go build -trimpath -ldflags="-s -w" -o carrier-riscv64 ./cmd/carrier
```

**Cross-compilation caveats:**

- CGO is **disabled** (`CGO_ENABLED=0`) for cross-compilation
- If you need CGO, you must set up a cross-compiler toolchain
- Test on actual hardware or emulator (e.g., QEMU) when possible

### 5. Update Checksums

After the first build, the release workflow generates SHA-256 checksums. Update `catalog/openclaw.manifest.json` if checksums are tracked there.

### 6. Document Installation

If the target has special installation requirements, update:

- `README.md` — Installation section
- `scripts/install.sh` — Add any architecture normalization logic

Example: Add RISC-V architecture normalization to `install.sh`:

```sh
case "$ARCH" in
    x86_64) ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    riscv64) ARCH="riscv64" ;;
    *) ARCH="$ARCH" ;;
esac
```

## Using Matrix Flags for Optional Targets

To avoid bloating every release with rarely-used builds, use **conditional matrix expansion** with workflow dispatch inputs or environment flags:

### Example: Workflow Dispatch Input

```yaml
on:
  workflow_dispatch:
    inputs:
      enable_experimental:
        description: 'Build experimental targets (riscv64, 386, etc.)'
        type: boolean
        default: false

jobs:
  build-packages:
    strategy:
      matrix:
        include:
          # Core targets always built
          - goos: linux
            goarch: amd64
            label: linux-x64
          
          # Conditional experimental targets
          ${{ if github.event.inputs.enable_experimental == 'true' }}:
            - goos: linux
              goarch: riscv64
              label: linux-riscv64
```

### Example: Environment-Based Flag

Set a repository variable `BUILD_ALL_TARGETS=true` and check it:

```yaml
strategy:
  matrix:
    include:
      - goos: linux
        goarch: amd64
        label: linux-x64
      
      # Only build if BUILD_ALL_TARGETS is set
      - ${{ vars.BUILD_ALL_TARGETS == 'true' && fromJSON('{"goos":"linux","goarch":"riscv64","label":"linux-riscv64","ext":"","archive_ext":"zip"}') || null }}
```

This keeps default releases fast while allowing opt-in for comprehensive builds.

## CI Matrix Best Practices

1. **Core targets first**: Keep commonly-used platforms in the default matrix
2. **Experimental behind flags**: Use workflow inputs or repository variables for niche targets
3. **Document support tier**: Mark targets as "Tier 1" (tested), "Tier 2" (built, untested), or "Tier 3" (community-maintained)
4. **Test on real hardware**: Cross-compilation works, but runtime behavior can differ
5. **Monitor build times**: Each target adds ~2-5 minutes to the release workflow

## Testing Matrix Changes

Before merging matrix changes:

1. **Local compilation test** (above)
2. **Dry-run the workflow** (if possible with `act` or GitHub CLI)
3. **Manual trigger** on a branch: `gh workflow run release.yml --ref your-branch`
4. **Check artifact sizes**: Ensure binaries aren't unexpectedly large

## Support Tiers

| Tier | Description                              | Testing      | Example Platforms        |
|------|------------------------------------------|--------------|--------------------------|
| 1    | Fully supported, tested in CI            | Automated    | linux-x64, darwin-arm64  |
| 2    | Built and distributed, manual testing    | Manual       | windows-arm64            |
| 3    | Community-maintained, best-effort builds | None         | linux-riscv64, openbsd   |

## Troubleshooting

### "Binary not found" errors on target platform

- Check `GOOS`/`GOARCH` match the target system: `go env GOOS GOARCH`
- Verify binary was built: `file carrier-riscv64` should show correct architecture

### Cross-compilation fails

- Disable CGO: `CGO_ENABLED=0`
- Check Go version supports the target: `go tool dist list`
- Some stdlib packages require CGO on specific platforms

### Checksum mismatches

- Ensure reproducible builds: use `-trimpath` and `-ldflags="-s -w"`
- Verify downloaded artifact wasn't corrupted
- Checksums are generated **after** build, not before

## References

- [Go: Installing Go from source (supported platforms)](https://go.dev/doc/install/source#environment)
- [GitHub Actions: Matrix strategy](https://docs.github.com/en/actions/using-workflows/workflow-syntax-for-github-actions#jobsjob_idstrategymatrix)
- [Cross-compilation guide](https://www.digitalocean.com/community/tutorials/building-go-applications-for-different-operating-systems-and-architectures)
