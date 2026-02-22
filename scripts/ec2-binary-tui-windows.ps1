param(
  [string]$Sha = "",
  [string]$Tag = "",
  [switch]$Main,
  [string]$Repo = "Keith-CY/carrier",
  [int]$WaitSeconds = -1,
  [string]$Label = "windows-x64",
  [string]$OutDir = "C:\Temp\carrier-ec2",
  [switch]$SkipOnboard,
  [switch]$SkipAdd
)

$ErrorActionPreference = "Stop"

function Show-Usage {
  @"
Usage:
  .\ec2-binary-tui-windows.ps1 -Sha <full_commit_sha> [options]
  .\ec2-binary-tui-windows.ps1 -Tag <release_tag> [options]
  .\ec2-binary-tui-windows.ps1 -Main [options]
  .\ec2-binary-tui-windows.ps1 [options]

Options:
  -Sha <sha>          Full commit SHA. Tag becomes main-<sha>.
  -Tag <tag>          Explicit release tag (for example main-<sha>).
  -Main               Resolve SHA from repository main HEAD.
  -Repo <owner/repo>  GitHub repository (default: Keith-CY/carrier).
  -WaitSeconds <n>    Wait up to n seconds for release asset (default: 600 with -Main, else 0).
  -Label <label>      Asset label (default: windows-x64).
  -OutDir <dir>       Download/extract directory (default: C:\Temp\carrier-ec2).
  -SkipOnboard        Skip `carrier onboard`.
  -SkipAdd            Skip `carrier add openclaw`.

Environment (optional):
  CARRIER_TELEGRAM_BOT_TOKEN
  CARRIER_PROVIDER_OVERRIDE
  CARRIER_PROVIDER_SECRET

Notes:
  - Default provider selection is openai-codex; OAuth device-code flow is shown in terminal.
  - If OAuth is needed, complete it in browser while command waits.
"@
}

function Wait-ReleaseAsset {
  param(
    [string]$Uri,
    [int]$TimeoutSeconds
  )

  if ($TimeoutSeconds -le 0) {
    return
  }

  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  while ($true) {
    $ready = $false
    $statusCode = 0
    try {
      $response = Invoke-WebRequest -Uri $Uri -Method Head -MaximumRedirection 5
      $statusCode = [int]$response.StatusCode
      if ($statusCode -ge 200 -and $statusCode -lt 400) {
        $ready = $true
      }
    } catch {
      if ($_.Exception.Response -and $_.Exception.Response.StatusCode) {
        $statusCode = [int]$_.Exception.Response.StatusCode.value__
      }
    }

    if ($ready) {
      Write-Host "[ec2] Release asset is ready: $Uri"
      return
    }

    if ((Get-Date) -ge $deadline) {
      throw "Release asset not ready after $TimeoutSeconds seconds: $Uri (last HTTP $statusCode)"
    }

    Write-Host "[ec2] Waiting for release asset (HTTP $statusCode), retrying in 10s..."
    Start-Sleep -Seconds 10
  }
}

if (-not [string]::IsNullOrWhiteSpace($Tag) -and -not [string]::IsNullOrWhiteSpace($Sha)) {
  Write-Error "-Tag and -Sha cannot be used together."
  Show-Usage
  exit 1
}

if ($WaitSeconds -lt -1) {
  Write-Error "-WaitSeconds must be -1 or a non-negative integer."
  Show-Usage
  exit 1
}

if ([string]::IsNullOrWhiteSpace($Tag) -and [string]::IsNullOrWhiteSpace($Sha)) {
  $Main = $true
}

if ($Main -and [string]::IsNullOrWhiteSpace($Tag) -and [string]::IsNullOrWhiteSpace($Sha)) {
  Write-Host "[ec2] Resolving main HEAD SHA from $Repo"
  $mainHead = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/commits/main"
  $Sha = [string]$mainHead.sha
  if ([string]::IsNullOrWhiteSpace($Sha) -or $Sha.Length -ne 40) {
    throw "Failed to resolve main HEAD SHA from $Repo"
  }
  Write-Host "[ec2] Resolved main SHA: $Sha"
}

if ([string]::IsNullOrWhiteSpace($Tag)) {
  if ([string]::IsNullOrWhiteSpace($Sha)) {
    Write-Error "Provide -Sha or -Tag (or use -Main)."
    Show-Usage
    exit 1
  }
  $Tag = "main-$Sha"
}

if ($WaitSeconds -eq -1) {
  if ($Main) {
    $WaitSeconds = 600
  } else {
    $WaitSeconds = 0
  }
}

New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

$zipName = "carrier-$Tag-$Label.zip"
$zipPath = Join-Path $OutDir $zipName
$sumPath = Join-Path $OutDir "$zipName.sha256"
$baseUrl = "https://github.com/$Repo/releases/download/$Tag"

Wait-ReleaseAsset -Uri "$baseUrl/$zipName" -TimeoutSeconds $WaitSeconds

Write-Host "[ec2] Downloading release asset: $baseUrl/$zipName"
Invoke-WebRequest -Uri "$baseUrl/$zipName" -OutFile $zipPath

Write-Host "[ec2] Downloading checksum: $baseUrl/$zipName.sha256"
Invoke-WebRequest -Uri "$baseUrl/$zipName.sha256" -OutFile $sumPath

Write-Host "[ec2] Verifying checksum"
$expected = ((Get-Content -Path $sumPath -Raw).Trim() -split '\s+')[0].ToLowerInvariant()
$actual = (Get-FileHash -Path $zipPath -Algorithm SHA256).Hash.ToLowerInvariant()
if ($expected -ne $actual) {
  throw "Checksum mismatch for $zipName. expected=$expected actual=$actual"
}

Write-Host "[ec2] Extracting $zipName"
Expand-Archive -Path $zipPath -DestinationPath $OutDir -Force

$binPath = Join-Path $OutDir "carrier.exe"
if (-not (Test-Path $binPath)) {
  throw "Extracted binary not found: $binPath"
}

Write-Host "[ec2] Binary ready: $binPath"
& $binPath --help | Out-Null

function Invoke-TuiCommand {
  param(
    [string[]]$CarrierArgs
  )

  $token = [string]$env:CARRIER_TELEGRAM_BOT_TOKEN
  $override = [string]$env:CARRIER_PROVIDER_OVERRIDE
  $secret = [string]$env:CARRIER_PROVIDER_SECRET

  if (-not [string]::IsNullOrWhiteSpace($token)) {
    if (-not [string]::IsNullOrWhiteSpace($override) -and $override -ne "openai-codex" -and [string]::IsNullOrWhiteSpace($secret)) {
      throw "CARRIER_PROVIDER_SECRET is required when CARRIER_PROVIDER_OVERRIDE=$override"
    }

    $lines = @($token, $override)
    if (-not [string]::IsNullOrWhiteSpace($override) -and $override -ne "openai-codex") {
      $lines += $secret
    }
    $inputText = ($lines -join "`n") + "`n"
    $inputText | & $binPath @CarrierArgs
    return
  }

  Write-Host "[ec2] CARRIER_TELEGRAM_BOT_TOKEN not set, running interactively for: carrier $($CarrierArgs -join ' ')"
  & $binPath @CarrierArgs
}

if (-not $SkipOnboard) {
  Write-Host "[ec2] Running: carrier onboard (TUI)"
  Invoke-TuiCommand -CarrierArgs @("onboard")
}

if (-not $SkipAdd) {
  Write-Host "[ec2] Running: carrier add openclaw (TUI)"
  Invoke-TuiCommand -CarrierArgs @("add", "openclaw")
}

Write-Host "[ec2] Current managed instances:"
& $binPath list

Write-Host "[ec2] OpenClaw status from daemon API (best effort):"
try {
  Invoke-RestMethod -Uri "http://127.0.0.1:9090/api/v1/agents/openclaw/status" | ConvertTo-Json -Depth 8
} catch {
  Write-Warning $_
}

Write-Host "[ec2] Done."
