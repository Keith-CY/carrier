# WSL2 Support Matrix

## Support Status

- Blessed configuration: **Ubuntu 22.04 on WSL2**
- Other WSL2 distros: **best-effort support only** (no guaranteed compatibility or SLA)

## Required Toolchain

Install these tools inside the Linux distro (not only on Windows):

- Go (current stable version)
- Node.js with npm, or Bun
- git

Quick checks:

- `go version`
- `node -v && npm -v` (or `bun --version`)
- `git --version`

## Known Limitations

### Networking

- `localhost` forwarding between Windows and WSL2 can be inconsistent after sleep/resume or network changes.
- VPNs and corporate proxies may break inbound/outbound connectivity for services running in WSL2.
- Firewall or endpoint security software on Windows can block forwarded ports.

### Filesystem

- Running projects from `/mnt/c/...` is slower and more error-prone than using Linux-native paths.
- File watching can be unreliable on mounted Windows filesystems.
- Path case sensitivity differences can cause runtime/build issues.

### Permissions

- Executable bit (`chmod +x`) and Unix permissions may not behave as expected on mounted Windows drives.
- Symlink behavior can vary by Windows policy and git configuration.
- Mixed Windows/Linux ownership and umask settings can produce unexpected access errors.

## Troubleshooting

1. Confirm WSL distro/version:
   - `wsl -l -v` from Windows PowerShell
   - `cat /etc/os-release` inside WSL
2. Verify runtime tools are installed in WSL:
   - `which go node npm bun git`
3. Prefer Linux-native workspace location:
   - e.g. `~/src/...` instead of `/mnt/c/...`
4. Validate network/port behavior:
   - Restart WSL: `wsl --shutdown`
   - Re-open distro and restart the daemon/service
5. Re-check permissions:
   - Ensure executable scripts have `chmod +x`
   - Run `git config core.fileMode true` when executable bits matter

## Best-Effort Disclaimer

For non-Ubuntu-22.04 WSL2 distros, behavior may differ due to package versions, init/system integration, or filesystem defaults. We accept bug reports and reasonable fixes, but compatibility is not guaranteed outside the blessed configuration.
