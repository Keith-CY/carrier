# EC2 Binary + TUI Validation Runbook

Purpose: validate Carrier on EC2 (Linux/Windows) without rebuilding from source, using:
- `carrier onboard` (TUI)
- `carrier add openclaw` (TUI)

## 1) Main Push Binary Location

For every push to `main`, `.github/workflows/release.yml` creates a **pre-release**:
- Tag: `main-<full_commit_sha>`
- Release name: `main-<full_commit_sha>`

Asset names:
- `carrier-main-<full_commit_sha>-linux-x64.zip`
- `carrier-main-<full_commit_sha>-windows-x64.zip`
- (plus arm64 variants and matching `.sha256/.sig/.sigstore.json`)

Direct download URL pattern:

```text
https://github.com/Keith-CY/carrier/releases/download/main-<full_commit_sha>/carrier-main-<full_commit_sha>-<label>.zip
```

Notes:
- The same workflow also uploads Actions artifacts (retention: 14 days).
- For EC2 validation, prefer release assets (`releases/download/...`) so no local rebuild is needed.

## 2) Linux EC2 Validation (No Rebuild)

Run on Linux EC2:

```bash
chmod +x scripts/ec2-binary-tui-linux.sh
scripts/ec2-binary-tui-linux.sh --sha <full_commit_sha>
```

Optional non-interactive env:

```bash
export CARRIER_TELEGRAM_BOT_TOKEN='<telegram bot token>'
# default provider keeps openai-codex
# export CARRIER_PROVIDER_OVERRIDE='openai'
# export CARRIER_PROVIDER_SECRET='<OPENAI_API_KEY>'
```

If default `openai-codex` is used and no saved credential exists, TUI will print:
- OAuth URL
- one-time device code

Complete OAuth in browser while command is waiting.

## 3) Windows EC2 Validation (No Rebuild)

Run in PowerShell (Admin not required):

```powershell
.\scripts\ec2-binary-tui-windows.ps1 -Sha <full_commit_sha>
```

Optional non-interactive env:

```powershell
$env:CARRIER_TELEGRAM_BOT_TOKEN = '<telegram bot token>'
# default provider keeps openai-codex
# $env:CARRIER_PROVIDER_OVERRIDE = 'openai'
# $env:CARRIER_PROVIDER_SECRET = '<OPENAI_API_KEY>'
```

If default `openai-codex` is used and no saved credential exists, TUI will print device-code login instructions and wait for completion.

## 4) Expected Validation Outputs

Successful flow should include:
- `carrier onboard` completes and starts/reuses daemon+gateway.
- `carrier add openclaw` completes install/start for OpenClaw.
- `carrier list` shows managed instance.
- `GET /api/v1/agents/openclaw/status` returns OpenClaw status payload.

## 5) Important Behavioral Note

Chat commands `/install` and `/onboard` are intentionally blocked in gateway chat mode (`E_INSTALL_GUI_ONLY` / `E_ONBOARD_GUI_ONLY`).

Use either:
- TUI (`carrier onboard`, `carrier add openclaw`), or
- WebUI (`carrier onboard --webui`, `carrier add openclaw --webui`).
