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

## Repository Map

| Path | Purpose |
|------|---------|
| [`daemon/`](./daemon/) | Go daemon — lifecycle management, catalog loading, Base Agent triage interfaces |
| [`gateway/`](./gateway/) | TypeScript gateway — provider routing, session management, download tokens |
| [`catalog/`](./catalog/) | OpenClaw manifest and candidate agent list |
| [`docs/`](./docs/) | Product requirements, implementation plan, and architecture docs |
| [`ARCHITECTURE.md`](./ARCHITECTURE.md) | System design overview and component interaction diagrams |
| [`CONTRIBUTING.md`](./CONTRIBUTING.md) | Contributor workflow, branching policy, and review conventions |
| [`skills/`](./skills/) | Automation and review helper skills (PR review, NBS follow-up) |
| [`scripts/`](./scripts/) | Development and CI helper scripts |

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

See `CONTRIBUTING.md` for command examples and required process. For security vulnerability reporting, see [`SECURITY.md`](./SECURITY.md).

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
   - Note: `PAIR_CODE` has a short TTL (currently 10 minutes). If pairing fails due to expiration, restart the daemon or request a fresh code and retry `/pair <code>`.
7. Use your chat provider flow (Telegram/Discord/Feishu) to pair and run commands:
   - `/pair <code>`
   - `/agents`
   - If `/agents` returns an empty list, verify the daemon is running and successfully paired, then retry after a few seconds. If still empty, re-run `/pair <code>` with a fresh code.
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
   - 说明：`PAIR_CODE` 有较短有效期（当前为 10 分钟）。若因过期导致配对失败，请重启 daemon 或重新获取新配对码后再执行 `/pair <code>`。
7. 通过 Telegram/Discord/Feishu 配对后执行：
   - `/pair <code>`
   - `/agents`
   - 若 `/agents` 返回空列表，请先确认 daemon 正在运行且已成功配对，等待数秒后重试；若仍为空，请用新的配对码重新执行 `/pair <code>`。
   - `/install openclaw`
8. 启动前先配置 OpenClaw 必需环境变量（如 `OPENAI_API_KEY`）。
9. 启动并检查：
   - `/start openclaw`
   - `/status openclaw`

如果安装或启动失败，请执行 `/diagnose openclaw` 并提交诊断产物。

### Flow verification checklist (download → pair → install/start)

Use this as a quick pass/fail checklist when validating the README flow:

1. **Download**
   - Can locate matching ZIP + `.sha256` in Releases.
2. **Checksum**
   - Verification command is available for your OS and returns success.
3. **Daemon start**
   - `agentd` starts and prints a usable `PAIR_CODE`.
4. **Pair**
   - `/pair <code>` returns success before TTL expires.
5. **Install path**
   - `/agents` includes `openclaw`, then `/install openclaw` starts normally.
6. **Start/Status**
   - `/start openclaw` succeeds; `/status openclaw` returns healthy/running.
7. **Fallback path**
   - On failure, `/diagnose openclaw` generates a support artifact.

### 流程验收清单（下载 → 配对 → 安装/启动）

可用下面清单快速确认 README 流程是否可执行：

1. **下载**：能在 Releases 找到匹配 ZIP 与 `.sha256`。
2. **校验**：对应系统校验命令可用且结果成功。
3. **启动 daemon**：`agentd` 启动后可看到可用 `PAIR_CODE`。
4. **配对**：`/pair <code>` 在有效期内返回成功。
5. **安装链路**：`/agents` 包含 `openclaw`，`/install openclaw` 能正常开始。
6. **启动与状态**：`/start openclaw` 成功，`/status openclaw` 显示 healthy/running。
7. **兜底诊断**：失败场景可通过 `/diagnose openclaw` 产出诊断文件。

## 三平台 Bot 管理（超详细步骤，可直接照做）

本节用于“只使用聊天软件操作”的场景。  
你不需要理解系统内部原理，只要按步骤发送命令。

### 先准备（缺一不可）

在开始前，请向项目管理员索取以下信息：

1. Telegram 机器人入口（用户名或邀请链接）
2. Discord 机器人入口（服务器+频道，或私聊入口）
3. 飞书机器人入口（会话入口）
4. 一次性配对码 `PAIR_CODE`（示例：`AB12CD34`）

如果缺少以上任意一项，本流程无法完成。

### 固定规则（所有平台都一样）

1. 命令必须发给机器人账号（Bot），不能发给真人账号
2. 每个平台第一次都要先执行：`/pair <PAIR_CODE>`
3. 配对成功后，才能执行 `/agents`、`/install openclaw` 等命令
4. 如果提示配对码无效或过期，向管理员申请新的 `PAIR_CODE`

### Telegram（逐步执行）

1. 打开 Telegram
2. 搜索管理员给你的机器人账号
3. 进入机器人聊天窗口
4. 发送：`/pair <PAIR_CODE>`
5. 等待回复，确认包含 `paired` 或“已配对”
6. 发送：`/agents`
7. 确认回复中包含 `openclaw`
8. 依次发送：
   - `/install openclaw`
   - `/start openclaw`
   - `/status openclaw`
9. 确认状态回复包含 `healthy`（或“健康”）

### Discord（逐步执行）

1. 打开 Discord
2. 进入管理员提供的服务器和频道（或机器人私聊窗口）
3. 确认消息对象是机器人账号
4. 发送：`/pair <PAIR_CODE>`
5. 等待回复，确认包含 `paired` 或“已配对”
6. 发送：`/agents`
7. 确认回复中包含 `openclaw`
8. 依次发送：
   - `/install openclaw`
   - `/start openclaw`
   - `/status openclaw`
9. 确认状态回复包含 `healthy`（或“健康”）

### 飞书（逐步执行）

1. 打开飞书
2. 进入管理员提供的机器人会话
3. 确认不是同事聊天窗口，而是机器人会话
4. 发送：`/pair <PAIR_CODE>`
5. 等待回复，确认包含 `paired` 或“已配对”
6. 发送：`/agents`
7. 确认回复中包含 `openclaw`
8. 依次发送：
   - `/install openclaw`
   - `/start openclaw`
   - `/status openclaw`
9. 确认状态回复包含 `healthy`（或“健康”）

### 常用命令（可复制）

```text
/pair <PAIR_CODE>
/agents
/install openclaw
/start openclaw
/status openclaw
/logs openclaw 200
/diagnose openclaw
/stop openclaw
```

### 验收标准（三平台都要通过）

以下 3 条都满足，表示配置成功：

1. Telegram 里 `/status openclaw` 返回 `healthy`
2. Discord 里 `/status openclaw` 返回 `healthy`
3. 飞书里 `/status openclaw` 返回 `healthy`

### 报错处理（按文字处理）

1. `pairing code is invalid or expired`
   - 处理：配对码错误或过期，向管理员申请新码，再执行 `/pair <PAIR_CODE>`
2. `chat is not paired; run /pair <code> first`
   - 处理：当前聊天窗口还未配对，先执行 `/pair <PAIR_CODE>`
3. `E_NOT_INSTALLED`
   - 处理：先执行 `/install openclaw`，再执行 `/start openclaw`
4. `E_ALREADY_RUNNING`
   - 处理：说明已经在运行，直接执行 `/status openclaw` 即可
5. 没有任何回复
   - 处理：通常是入口错误（发给了非机器人账号）或机器人未接通后端，请联系管理员

### 管理员信息模板（建议放在团队文档）

```text
Telegram Bot: <填写用户名或链接>
Discord 入口: <填写服务器/频道或私聊入口>
飞书入口: <填写机器人会话入口>
PAIR_CODE 获取方式: <填写由谁提供、有效期多久>
支持联系人: <填写姓名与联系方式>
```

## Three-Platform Bot Management (Detailed Step-by-Step)

This section is for users who operate only through chat apps.  
Follow the steps exactly. No internal system knowledge is required.

### Prerequisites (all required)

Before starting, ask your project administrator for:

1. Telegram bot entry (username or invite link)
2. Discord bot entry (server + channel, or DM entry)
3. Feishu bot entry (chat entry)
4. One-time pairing code `PAIR_CODE` (example: `AB12CD34`)

If any of the items above is missing, this flow cannot be completed.

### Fixed rules (same for all platforms)

1. Send commands to a bot account, not to a human account
2. On each platform, run pairing first: `/pair <PAIR_CODE>`
3. Only after pairing succeeds, run `/agents`, `/install openclaw`, and other commands
4. If pairing code is invalid or expired, request a new `PAIR_CODE` from admin

### Telegram (step by step)

1. Open Telegram
2. Search for the bot account provided by admin
3. Open the bot chat window
4. Send: `/pair <PAIR_CODE>`
5. Wait for reply and confirm it includes `paired`
6. Send: `/agents`
7. Confirm response includes `openclaw`
8. Send in order:
   - `/install openclaw`
   - `/start openclaw`
   - `/status openclaw`
9. Confirm status response includes `healthy`

### Discord (step by step)

1. Open Discord
2. Enter the server/channel provided by admin (or bot DM)
3. Confirm your message target is the bot account
4. Send: `/pair <PAIR_CODE>`
5. Wait for reply and confirm it includes `paired`
6. Send: `/agents`
7. Confirm response includes `openclaw`
8. Send in order:
   - `/install openclaw`
   - `/start openclaw`
   - `/status openclaw`
9. Confirm status response includes `healthy`

### Feishu (step by step)

1. Open Feishu
2. Enter the bot chat provided by admin
3. Confirm this is a bot chat, not a human chat
4. Send: `/pair <PAIR_CODE>`
5. Wait for reply and confirm it includes `paired`
6. Send: `/agents`
7. Confirm response includes `openclaw`
8. Send in order:
   - `/install openclaw`
   - `/start openclaw`
   - `/status openclaw`
9. Confirm status response includes `healthy`

### Common commands (copy-paste)

```text
/pair <PAIR_CODE>
/agents
/install openclaw
/start openclaw
/status openclaw
/logs openclaw 200
/diagnose openclaw
/stop openclaw
```

### Acceptance criteria (all 3 platforms)

Configuration is successful only when all conditions are met:

1. In Telegram, `/status openclaw` returns `healthy`
2. In Discord, `/status openclaw` returns `healthy`
3. In Feishu, `/status openclaw` returns `healthy`

### Error handling (match exact message)

1. `pairing code is invalid or expired`
   - Action: pairing code is wrong or expired; request a new code and run `/pair <PAIR_CODE>` again
2. `chat is not paired; run /pair <code> first`
   - Action: current chat is not paired; run `/pair <PAIR_CODE>` first
3. `E_NOT_INSTALLED`
   - Action: run `/install openclaw` first, then run `/start openclaw`
4. `E_ALREADY_RUNNING`
   - Action: bot is already running; run `/status openclaw` directly
5. No reply at all
   - Action: usually wrong entry (not a bot account) or bot backend not connected; contact admin

### Admin info template (recommended)

```text
Telegram Bot: <username or invite link>
Discord Entry: <server/channel or DM entry>
Feishu Entry: <bot chat entry>
PAIR_CODE Process: <who provides it and expiration policy>
Support Contact: <name and contact method>
```
