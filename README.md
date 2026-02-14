# Carrier

Phase 1 scaffold for the Agent Installation Platform.

## Current scope
- Runtime: local host (macOS/Linux), WSL2 (Windows)
- Full lifecycle target: OpenClaw
- Candidate-only agents: Pi Mono, NanoClaw, Pico Claw
- Memory model: Per-Agent, Shared, Public
- Gateway providers: Telegram, Discord, Feishu

## Repository layout
- `daemon/`: Go daemon lifecycle, catalog, Base Agent triage interfaces
- `gateway/`: TypeScript gateway contracts and routing scaffold
- `catalog/`: OpenClaw manifest and candidate agent list
- `docs/`: Product and delivery documentation

## M2 daemon scaffold status
- Runtime prerequisite checks are implemented for local host and WSL2 requirement detection.
- Lifecycle service now executes install/start/stop commands through a runner abstraction.
- Start path validates required env vars and port conflicts before execution.
- Logs tail retrieval and diagnose zip artifact generation are available in the lifecycle service.

## M3 remote diagnosis scaffold status
- Lifecycle service now stores audit logs for lifecycle, triage, diagnose, and consent actions.
- Base Agent unresolved flow can create a remote diagnosis handoff placeholder with user consent.
- Handoff records include consent flag, status, and diagnose artifact reference.

## M4 gateway routing scaffold status
- Gateway now supports `pair/agents/install/start/stop/status/logs/upgrade/diagnose/diagnose-consent` routing scaffold.
- `/pair <code>` binds provider chat sessions and enforces session checks before non-pair commands.
- Gateway forwards `provider`, `chat_id`, and `request_id` into daemon request context for command handling.
- Logs/diagnose responses can issue short-lived read-only download tokens and URLs.

## Development flow
- Start work from `origin/main` using a new `codex/*` branch and worktree.
- Submit all feature updates via Pull Request.
- Use GitHub Actions as test source of truth.
- For non-blocking review suggestions, use `NBS:` lines (one suggestion per line). Post-merge automation creates follow-up issues from those lines.

See `CONTRIBUTING.md` for command examples and required process.

## Installation and testing

### Prerequisites
- Go toolchain (1.21+)
- Bun (for gateway TypeScript tasks)

### Install
- Daemon (Go): Go modules are loaded automatically when building or testing; no additional install command needed.
- Gateway (TypeScript):
  - `cd gateway`
  - `bun install`

### Run tests / checks
- Daemon tests:
  - `cd daemon`
  - `go test ./...`
  - `go test ./internal/manifest -run TestLoadFileAcceptsCatalogManifest -count=1`
- Gateway checks:
  - `cd gateway`
  - `bun run check`
- Optional local flow from repo root:
  - `cd gateway && bun install && bun run check && cd ../daemon && go test ./...`

## Local workflow notes

- Before making changes, read the skill instructions under `skills/` (especially `skills/pr-review/SKILL.md` and `skills/review-followup/SKILL.md`) to follow repository-specific conventions.

## Install OpenClaw from release package (non-technical quick path)

> Status note: this project is still in Phase 1 scaffold, but release packages can already be used as the starting point for local installation flow.

### English

1. Open the **Releases** page of this repository.
2. Download the ZIP file for your OS/CPU (for example: `linux-x64`, `darwin-arm64`, `windows-x64`).
3. (Optional but recommended) Download the matching `.sha256` file and verify checksum.
4. Extract the ZIP to a local folder.
5. Start the daemon (`agentd`) from the extracted folder.
6. Use your chat provider flow (Telegram/Discord/Feishu) to pair and run commands:
   - `/pair <code>`
   - `/agents`
   - `/install openclaw`
   - `/start openclaw`
   - `/status openclaw`

If install/start fails, run diagnose flow and use the generated artifact for support.

### 中文（简版）

1. 打开本仓库 **Releases** 页面。
2. 下载与你系统匹配的 ZIP（如 `linux-x64`、`darwin-arm64`、`windows-x64`）。
3. （可选）下载同名 `.sha256` 文件做校验。
4. 解压 ZIP 到本地目录。
5. 运行解压目录里的 daemon（`agentd`）。
6. 通过 Telegram/Discord/Feishu 配对后执行：
   - `/pair <code>`
   - `/agents`
   - `/install openclaw`
   - `/start openclaw`
   - `/status openclaw`

如果安装或启动失败，请走 diagnose 流程并提交诊断产物。
