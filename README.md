# Carrier

Phase 1 scaffold for the Agent Installation Platform.

## Current scope
- Runtime: local host (macOS/Linux), WSL2 (Windows)
- Full lifecycle target: OpenClaw
- Candidate-only agents: Pi Mono, NanoClaw, Pico Claw
- Memory model: Per-Agent, Shared, Public
- Gateway providers: Telegram, Discord, Feishu

## Read order (single source of truth)
For product scope/priority decisions, use this order:
1. `docs/Agent_Installation_Platform_PRD.md` (source of truth for scope and requirements)
2. `docs/Agent_Installation_Platform_Implementation_Plan.md` (execution sequence and delivery plan)
3. `README.md` (quick orientation and current implementation status)

## Architecture

For a detailed overview of the system design and component interactions, see [`ARCHITECTURE.md`](./ARCHITECTURE.md).

Key topics covered:
- Daemon lifecycle state machine and service boundaries
- Gateway routing and provider abstraction
- Catalog manifest schema and validation flow
- Memory model (Per-Agent, Shared, Public) and mount policy

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

## Terminology (quick glossary)
- **Runtime**: where agent processes run (macOS/Linux host, Windows via WSL2).
- **Diagnose**: generate a sanitized diagnostic artifact for troubleshooting.
- **Memory types**:
  - **Per-Agent**: memory private to one agent instance.
  - **Shared**: reusable memory across local agents (default read-only mount).
  - **Public**: template memory packages (read-only).
- **Priority levels**:
  - **P0**: must-have for Phase 1 acceptance.
  - **P1**: important but can follow after P0.

## Installation and testing

### Prerequisites
- Go toolchain (see `daemon/go.mod` for the required version)
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
- Full local flow from repo root (mandatory before pushing):
  - `./scripts/run-all-tests.sh`

Optional local flow from repo root:
  - `cd gateway && bun install && bun run check && cd ../daemon && go test ./...`

#### Merge Queue
- This repository supports GitHub Merge Queue for `main`.
- For merge operations, prefer queue-based merge from the GitHub UI.

#### CI coverage note (E2E execution policy)
- `End-to-End Tests` in `.github/workflows/ci.yml` run only on `push` to `main` (`github.event_name == 'push' && github.ref == 'refs/heads/main'`).
- Pull requests do not run E2E in `ci.yml`.
- Release flow also enforces mandatory end-to-end validation in `.github/workflows/release.yml` (`End-to-End Tests (Deployment)`).

## Local workflow notes

- Before making changes, read the skill instructions under `skills/` (especially `skills/pr-review/SKILL.md` and `skills/review-followup/SKILL.md`) to follow repository-specific conventions.

## Kanban workflow required secrets/env vars

For `.github/workflows/carrier-kanban-operations.yml`, configure these repository-level secrets/variables before running the workflow (see workflow file: [`.github/workflows/carrier-kanban-operations.yml`](./.github/workflows/carrier-kanban-operations.yml)):

- `CARRIER_PROJECT_ID` (required): GitHub Project (v2) ID used by the workflow.
- `CARRIER_PROJECTS_TOKEN` (required): token with project read/write permissions for field/view operations.
- `CARRIER_DISCUSSION_CATEGORY_ID` (optional): discussion category override for workflow-generated discussion posts.

Implementation note: the workflow falls back to the pre-authenticated `github` client from `@actions/github` when an explicit token is not provided, so repository-scoped operations still work with default workflow auth.

Without the required values above, Kanban operations workflow runs will fail early during setup.

## Install OpenClaw from release package (non-technical quick path)

> Status note: this project is still in Phase 1 scaffold, but release packages can already be used as the starting point for local installation flow.

### English

1. Open [**Releases**](https://github.com/Keith-CY/carrier/releases) for this repository.
2. Download the ZIP package for your OS/CPU (for example: `linux-x64`, `darwin-arm64`, `windows-x64`).
3. (Optional but recommended) Download the matching `.sha256` file and verify checksum:
   - Linux (GNU): `sha256sum -c carrier-*.sha256`
   - macOS: `shasum -a 256 carrier-*.zip`
   - Windows PowerShell: `Get-FileHash .\carrier-*.zip -Algorithm SHA256`
4. Extract the ZIP to a local folder.
5. Start the daemon from the extracted folder:
   - macOS/Linux: `./agentd`
   - Windows PowerShell: `.\agentd.exe`
6. Get your pairing code from the daemon terminal output (it is generated when daemon/gateway pairing flow is ready).
7. Use your chat provider flow (Telegram/Discord/Feishu) to pair and run commands:
   - `/pair <code>`
   - `/agents`
   - `/install openclaw`
8. Configure required OpenClaw environment (for example `OPENAI_API_KEY`) before start.
9. Start and verify:
   - `/start openclaw`
   - `/status openclaw`

If install/start fails, run `/diagnose openclaw` and use the generated artifact for support.

### 中文（简版）

1. 打开本仓库 [**Releases**](https://github.com/Keith-CY/carrier/releases) 页面。
2. 下载与你系统匹配的 ZIP（如 `linux-x64`、`darwin-arm64`、`windows-x64`）。
3. （可选）下载同名 `.sha256` 文件做校验：
   - Linux（GNU）：`sha256sum -c carrier-*.sha256`
   - macOS：`shasum -a 256 carrier-*.zip`
   - Windows PowerShell：`Get-FileHash .\carrier-*.zip -Algorithm SHA256`
4. 解压 ZIP 到本地目录。
5. 运行解压目录里的 daemon：
   - macOS/Linux：`./agentd`
   - Windows PowerShell：`.\agentd.exe`
6. 从 daemon 终端输出里获取配对码（pair code）。
7. 通过 Telegram/Discord/Feishu 配对后执行：
   - `/pair <code>`
   - `/agents`
   - `/install openclaw`
8. 启动前先配置 OpenClaw 必需环境变量（如 `OPENAI_API_KEY`）。
9. 启动并检查：
   - `/start openclaw`
   - `/status openclaw`

如果安装或启动失败，请执行 `/diagnose openclaw` 并提交诊断产物。
