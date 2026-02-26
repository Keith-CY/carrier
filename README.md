# Carrier

Scaffold for the Agent Installation Platform.

## Source of truth

- Current architecture index: `docs/current-architecture.md`
- Runtime model ADR: `docs/phase1-runtime-adr.md`
- Canonical product scope: `docs/Agent_Installation_Platform_PRD.md`

## Current scope
- Runtime: local host (macOS/Linux), WSL2 (Windows)
- Runtime exclusions: Docker is out of scope for Phase 1
- Full lifecycle target: OpenClaw
- Candidate-only agents: Pi Mono, NanoClaw, Pico Claw
- Memory model: Per-Agent, Shared, Public
- Gateway providers: Telegram, Discord, Feishu

## Read order (single source of truth)
For product scope/priority decisions, use this order:
1. `docs/current-architecture.md` (canonical index and task entry points)
2. `docs/Agent_Installation_Platform_PRD.md` (source of truth for scope and requirements)
3. `docs/phase1-runtime-adr.md` (canonical runtime-model ADR for Phase 1)
4. `docs/Agent_Installation_Platform_Implementation_Plan.md` (historical execution plan context)
5. `README.md` (quick orientation and current implementation status)

## Task-first docs quick links
- `docs/task-first-quickstart.md`
- `docs/carrier-cli.md`
- `docs/runbooks/pairing-lifecycle.md`
- `docs/ci/first-response-playbook.md`
- `docs/runbooks/go-live-rollback.md`

## Contributor docs quick links
- `CONTRIBUTING.md`
- `docs/command-contract.md`
- `docs/daemon-api-contract.md`
- `docs/daemon-lifecycle-runtime.md`
- `docs/e2e-parity-taxonomy.md`

## Quick Navigation

- [Architecture](#architecture)
- [Repository Map](#repository-map)
- [Installation and Testing](#installation-and-testing)
- [Carrier CLI Flow](#carrier-cli-flow-recommended-bootstrap--onboard--add)
- [EC2 Binary + TUI Validation](#ec2-binary--tui-validation)
- [Install OpenClaw from Release](#install-openclaw-from-release-package-non-technical-quick-path)
- [三平台 Bot 管理（中文）](#三平台-bot-管理超详细步骤可直接照做)
- [Three-Platform Bot Management (English)](#three-platform-bot-management-detailed-step-by-step)

## Architecture

For a detailed overview of the system design and component interactions, see [`ARCHITECTURE.md`](./ARCHITECTURE.md).

Key topics covered:
- Top-level dependency boundaries (`webui -> gateway -> daemon -> shared`, `webui -> gateway -> baseagent -> shared`)
- Daemon lifecycle state machine and service boundaries
- Gateway routing and provider abstraction
- Catalog manifest schema and validation flow
- Memory model (Per-Agent, Shared, Public) and mount policy

## Repository Map

| Path | Purpose |
|------|---------|
| [`shared/`](./shared/) | Shared Go module — cross-module config and redaction primitives |
| [`baseagent/`](./baseagent/) | Base Agent Go module — triage policy/runtime, independently testable |
| [`daemon/`](./daemon/) | Go daemon — lifecycle scheduling, host runtime management, daemon API |
| [`gateway/`](./gateway/) | Go gateway module — ingress, session/rate-limit, webhook/message routing |
| [`webui/`](./webui/) | Local WebUI static app and handler package |
| [`catalog/`](./catalog/) | OpenClaw manifest and candidate agent list |
| [`docs/`](./docs/) | Product requirements, implementation plan, and architecture docs |
| [`ARCHITECTURE.md`](./ARCHITECTURE.md) | System design overview and component interaction diagrams |
| [`CONTRIBUTING.md`](./CONTRIBUTING.md) | Contributor workflow, branching policy, and review conventions |
| [`skills/`](./skills/) | Automation and review helper skills (PR review, NBS follow-up) |
| [`scripts/`](./scripts/) | Development and CI helper scripts |
| [`tests/`](./tests/) | Cross-module integration and end-to-end tests |

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
- Telegram/Discord/Feishu payload parsers now normalize provider events/interactions into one gateway command DTO shape.

## Development flow
- Start work from `origin/main` using a new `codex/*` branch and worktree.
- Submit all feature updates via Pull Request.
- Use GitHub Actions as test source of truth.
- For non-blocking review suggestions, use `NBS:` lines (one suggestion per line). Post-merge automation creates follow-up issues from those lines.

See `CONTRIBUTING.md` for command examples and required process. For security vulnerability reporting, see [`SECURITY.md`](./SECURITY.md). For install/upgrade integrity verification, see [`docs/security-install-integrity.md`](./docs/security-install-integrity.md).

## Terminology Mapping

| Term | Mapping |
|---|---|
| Runtime | Local host (macOS/Linux) or WSL2 (Windows), no Docker path in Phase 1 |
| Diagnose | Sanitized diagnostic artifact generation with optional remote diagnosis consent flow |
| Per-Agent Memory | Memory private to one agent instance |
| Shared Memory | Reusable memory across local agents (default read-only mount) |
| Public Memory | Template memory packages (read-only) |
| P0 | Must-have for Phase 1 acceptance |
| P1 | Important priority after P0 |

## Installation and testing

### Carrier CLI flow (recommended: bootstrap → onboard → install)

Carrier CLI is now the recommended first path:

1. `carrier` (bootstrap)
   - If no onboarding config exists, it starts onboarding automatically.
   - If already onboarded, it starts/reuses daemon + gateway and exits.
2. `carrier onboard` (interactive TUI/CLI)
   - This flow is Telegram-first today.
   - Explicit terminal aliases are also supported: `carrier onboard --tui` and `carrier onboard --cli`.
   - For Discord/Feishu onboarding, use `carrier onboard --webui` (or provider/manual setup) and run install flow through WebUI where needed.
3. `carrier install <agent_id>` (managed or direct)
   - `openclaw`, `picoclaw`, and `zeroclaw`: managed-agent flow with provider/channel setup, then install/start plus instance tracking.
   - Other agent IDs: direct install + start for the daemon.
   - `carrier add <agent_id>` remains supported as an alias.
4. `carrier install <agent_id> --webui` (browser-assisted for all managed flows)

For full command coverage, see [`docs/carrier-cli.md`](./docs/carrier-cli.md).

Notes:

- Chat `/install` and `/onboard` command names still exist, but onboarding/install in chat mode are intentionally blocked (`E_INSTALL_GUI_ONLY` / `E_ONBOARD_GUI_ONLY`) to protect credentials; use Carrier CLI/TUI (`carrier install`, `carrier onboard`) or WebUI instead.
- Release/package flows below are still valid and useful for non-CLI deployment scenarios.

### Onboarding Runbook (Directly Executable)

1. Run bootstrap first (one-time setup)
   - Command: `carrier`
   - Expected result:
     - If not initialized, onboarding guidance starts automatically.
     - If already initialized, daemon and gateway are started/reused and the command exits.
2. Enter onboarding explicitly (recommended)
   - Command: `carrier onboard`
   - Complete the prompts:
     - Select communication channel (Telegram/Discord/Feishu).
     - Enter channel credentials.
     - Select and configure model provider credentials.
3. Use WebUI onboarding if a browser-assisted flow is preferred
   - Command: `carrier onboard --webui`
   - Expected result:
     - Local browser opens the WebUI onboarding flow.
     - Gateway health endpoint is reachable at `http://127.0.0.1:8787/healthz`.
4. Install an agent after onboarding for validation
   - Command: `carrier add openclaw`
   - Expected result:
     - Install/start logs are printed.
     - `carrier list` includes the new instance.
5. Quick troubleshooting
   - `carrier gateway` startup fails: check whether `CARRIER_GATEWAY_PORT` is already in use.
   - Command timeout: verify `CARRIER_DAEMON_BASE_URL` is reachable.
   - Channel pairing fails: re-run `carrier onboard` and verify channel token/secret values.

### Prerequisites
- Go toolchain (see `daemon/go.mod` for the required version)
- Bun (required for WebUI TypeScript build and utility scripts)

#### Toolchain quick check

Run the following to verify required tools are installed and print their versions:

```bash
echo "gh:  $(gh --version | head -1)"
echo "go:  $(go version)"
echo "bun: $(bun --version)"
```

If any command fails, install the missing tool before running automation scripts. See [CONTRIBUTING.md](./CONTRIBUTING.md) for detailed CI troubleshooting.

### Install
- Carrier/Daemon/Gateway/BaseAgent/Shared (Go): Go modules are loaded automatically when building or testing; no additional install command needed.
- WebUI assets: source files are TypeScript in `webui/src/*.ts`; build output is generated to `webui/static/*.js` via `bash scripts/build-webui.sh`.

### Run tests / checks
- Daemon tests:
  - `cd daemon`
  - `go test ./...`
  - `go test ./internal/manifest -run TestLoadFileAcceptsCatalogManifest -count=1`
- Gateway tests:
  - `cd gateway`
  - `go test ./...`
- Base Agent tests:
  - `cd baseagent`
  - `go test ./...`
- Shared module tests:
  - `cd shared`
  - `go test ./...`
- Coverage gate from repo root:
  - `make coverage-gate`
  - Strict all-module 100% mode: `COVERAGE_STRICT_100=1 make coverage-gate`
- Full local flow from repo root (mandatory before pushing):
  - `./scripts/run-all-tests.sh`

Optional local flow from repo root:
  - `cd daemon && go test ./...`

### Gateway runtime server (Go)

- Start gateway runtime HTTP server:
  - `go run ./cmd/carrier gateway`
- Health route:
  - `GET /healthz`
- Command ingress route:
  - `POST /command` with JSON body `{ "input": "<provider> <chat_id> <request_id> <command> [...args]" }`
  - If `CARRIER_GATEWAY_API_TOKEN` is set, send `Authorization: Bearer <gateway_api_token>`
  - For authenticated commands (non-`/pair`), provide session token via `x-session-token` header or `sessionToken` field in JSON body
- Artifact download route:
  - `GET /downloads/<token>/<filename>`

Runtime environment variables:
- `CARRIER_DAEMON_BASE_URL` (default: `http://127.0.0.1:9090`)
- `CARRIER_DAEMON_TIMEOUT_MS` (default: `30000`)
- `CARRIER_COMMAND_TIMEOUT` (default: `5m`, daemon lifecycle install/upgrade command timeout)
- `CARRIER_GATEWAY_HOST` (default: `127.0.0.1`)
- `CARRIER_GATEWAY_PORT` (default: `8787`)
- `CARRIER_GATEWAY_API_TOKEN` (optional on loopback; required for non-loopback bind)
- `CARRIER_MAX_COMMAND_BODY_BYTES` (default: `65536`)
- `CARRIER_REMOTE_CONTROL_PLANE_ENABLED` (default: `true`)
- `CARRIER_REMOTE_CHAT_ENABLED` (default: `true`, effective only when `CARRIER_REMOTE_CONTROL_PLANE_ENABLED=true`)
- `CARRIER_PROVIDER_BINDING_ENABLED` (default: `true`, effective only when `CARRIER_REMOTE_CONTROL_PLANE_ENABLED=true`)

Remote rollout metrics:
- `GET /api/v1/remote/metrics` returns aggregate remote-operation metrics plus `rollout` status:
  - `rollout.state`: `healthy | canary | hold`
  - `rollout.canPromote`: rollout gate signal for full promotion
  - `rollout.reasons`: threshold-based reasons when state is not fully healthy

SSH trust behavior note:
- Remote SSH uses `StrictHostKeyChecking=accept-new` (TOFU) by default for first connection convenience.
- For production environments, pre-populate `known_hosts` and/or use controlled SSH config to avoid implicit first-seen trust.

### Managed PicoClaw Secret Persistence

- Managed instance record files (`~/.carrier/agents/*.json`) do not store channel/provider secret values.
- Shared provider credentials are loaded from the credential store (`CARRIER_CREDENTIAL_STORE`, with keychain/file fallback logic in code).
- For OAuth provider flows (for example `openai-codex`), tokens are persisted in `~/.picoclaw/auth.json`; generated `config.json` keeps auth metadata/reference fields and avoids duplicating OAuth token values in model entries.
- API-key providers keep a single provider-level credential entry in generated PicoClaw config instead of duplicating the same key across multiple config nodes.

#### Merge Queue
- This repository supports GitHub Merge Queue for `main`.
- For merge operations, prefer queue-based merge from the GitHub UI.

#### CI coverage note (E2E execution policy)
- `Provider Parity (core/failure)` matrix in `.github/workflows/ci.yml` runs on PR and push, and uploads per-scenario log artifacts (`provider-parity-core`, `provider-parity-failure`) for drift diagnosis.
- `End-to-End Tests` in `.github/workflows/ci.yml` run only on `push` to `main` (`github.event_name == 'push' && github.ref == 'refs/heads/main'`).
- Pull requests do not run E2E in `ci.yml`.
- Docs-only changes run `.github/workflows/docs-consistency.yml`, which executes `scripts/check-doc-command-sync.sh`.
- Release flow also enforces mandatory end-to-end validation in `.github/workflows/release.yml` (`End-to-End Tests (Deployment)`).

## Local workflow notes

- Before making changes, read the skill instructions under `skills/` (especially `skills/pr-review/SKILL.md` and `skills/review-followup/SKILL.md`) to follow repository-specific conventions.

## Runbooks

- Pairing lifecycle and troubleshooting matrix: `docs/runbooks/pairing-lifecycle.md`
- Go-live checklist and rollback: `docs/runbooks/go-live-rollback.md`
- Post-merge smoke checklist (includes remote control plane flows): `docs/runbooks/post-merge-smoke-checklist.md`
- CI first-response playbook: `docs/ci/first-response-playbook.md`

## Kanban workflow env vars

For `.github/workflows/carrier-kanban-operations.yml` (see [`.github/workflows/carrier-kanban-operations.yml`](./.github/workflows/carrier-kanban-operations.yml)):

- `CARRIER_PROJECTS_TOKEN` (required): token with project read/write permissions for field/view operations.
- `CARRIER_PROJECT_ID` (optional override): target project node ID override.
- `CARRIER_DISCUSSION_CATEGORY_ID` (optional): discussion category override for workflow-generated discussion posts.

Target project resolution order:
1. workflow dispatch input `project_id`
2. `CARRIER_PROJECT_ID`
3. repository workflow/config default

Implementation notes:
- `carrier-kanban-operations.yml` can fall back to the pre-authenticated `github` client from `@actions/github` for repository-scoped calls when an explicit token is not provided.
- Kanban workflows skip with a warning when `CARRIER_PROJECTS_TOKEN` is missing or project access is unavailable, so unrelated CI checks are not blocked by Kanban configuration gaps.

## EC2 Binary + TUI Validation

- For each push to `main`, release workflow publishes a **pre-release** with tag `main-<full_commit_sha>`.
- Binary package naming on that tag:
  - `carrier-main-<full_commit_sha>-linux-x64.zip`
  - `carrier-main-<full_commit_sha>-windows-x64.zip`
- Release assets can be downloaded directly from:
  - `https://github.com/Keith-CY/carrier/releases/download/main-<full_commit_sha>/carrier-main-<full_commit_sha>-<label>.zip`
- Quick install (`curl ... | bash`) is available via:
  - `curl -fsSL https://raw.githubusercontent.com/Keith-CY/carrier/main/scripts/install.sh | bash`
  - Installer behavior: resolves `main` HEAD SHA, downloads `carrier-main-<full_commit_sha>-<label>.zip`, verifies `.sha256`, installs `carrier`.
- Do not infer latest by taking the first `/releases` entry; resolve `main` HEAD SHA and use `main-<full_commit_sha>`.
- Use TUI/CLI flow for onboarding/install on EC2:
  - `carrier onboard`
  - `carrier install openclaw` (or `carrier add openclaw`)
- Chat `/install` and `/onboard` are intentionally blocked in gateway chat mode (`E_INSTALL_GUI_ONLY` / `E_ONBOARD_GUI_ONLY`) for credential safety.
- End-to-end no-rebuild scripts:
  - Linux: `scripts/ec2-binary-tui-linux.sh` (no args defaults to latest `main` push release)
  - Windows: `scripts/ec2-binary-tui-windows.ps1` (no args defaults to latest `main` push release)
- Detailed runbook: `docs/runbooks/ec2-binary-tui-validation.md`

## Install OpenClaw from release package (non-technical quick path)

> Status note: this project is still in Phase 1 scaffold, but release packages can already be used as the starting point for local installation flow.

### English

1. Open [**Releases**](https://github.com/Keith-CY/carrier/releases) for this repository. Artifacts follow the naming pattern `carrier-<commit-sha>-<os>-<arch>.zip` (e.g., `carrier-1feeeb7-linux-x64.zip`, `carrier-1feeeb7-darwin-arm64.zip`).
2. Download the ZIP package for your OS/CPU (for example: `linux-x64`, `darwin-arm64`, `windows-x64`).
3. (Optional but recommended) Download the matching `.sha256` file and verify checksum:
   - Linux (GNU): `sha256sum -c carrier-*.sha256`
   - macOS: `shasum -a 256 carrier-*.zip`
   - Windows PowerShell: `Get-FileHash .\carrier-*.zip -Algorithm SHA256`
4. Extract the ZIP to a local folder.
5. Start the daemon from the extracted folder:
   - macOS/Linux: `./carrier daemon`
   - Windows PowerShell: `.\carrier.exe daemon`
6. Get your pairing code from the daemon terminal output (format: `pair-<hex>`). If needed, you can also issue one via API: `curl -s -X POST http://127.0.0.1:9090/api/v1/pairing/codes`.
   - Note: `PAIR_CODE` has a short TTL (currently 5 minutes). If pairing fails due to expiration, request a fresh code and retry `/pair <code>`.
7. Use your chat provider flow (Telegram/Discord/Feishu) to pair and run commands:
   - `/pair <code>`
   - `/agents`
   - If `/agents` returns an empty list, verify the daemon is running and successfully paired, then retry after a few seconds. If still empty, re-run `/pair <code>` with a fresh code.
8. Install OpenClaw through Carrier CLI/TUI or WebUI (chat `/install` is intentionally blocked):
   - `carrier install openclaw` (or `carrier add openclaw`)
9. Configure required OpenClaw environment (for example `OPENAI_API_KEY`) before start.
10. Start and verify:
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
   - macOS/Linux：`./carrier daemon`
   - Windows PowerShell：`.\carrier.exe daemon`
6. 从 daemon 终端输出里获取配对码（格式：`pair-<hex>`）。如需手动申请，也可调用 API：`curl -s -X POST http://127.0.0.1:9090/api/v1/pairing/codes`。
   - 说明：`PAIR_CODE` 有较短有效期（当前为 5 分钟）。若因过期导致配对失败，请重新获取新配对码后再执行 `/pair <code>`。
7. 通过 Telegram/Discord/Feishu 配对后执行：
   - `/pair <code>`
   - `/agents`
   - 若 `/agents` 返回空列表，请先确认 daemon 正在运行且已成功配对，等待数秒后重试；若仍为空，请用新的配对码重新执行 `/pair <code>`。
8. 通过 Carrier CLI/WebUI 安装 OpenClaw（chat `/install` 已被有意禁用）：
   - `carrier add openclaw`
9. 启动前先配置 OpenClaw 必需环境变量（如 `OPENAI_API_KEY`）。
10. 启动并检查：
   - `/start openclaw`
   - `/status openclaw`

如果安装或启动失败，请执行 `/diagnose openclaw` 并提交诊断产物。

### Flow verification checklist (download → pair → add/start)

Use this as a quick pass/fail checklist when validating the README flow:

1. **Download**
   - Can locate matching ZIP + `.sha256` in [Releases](https://github.com/Keith-CY/carrier/releases).
   - Artifact naming is expected as `carrier-<commit-sha>-<os>-<arch>.zip` with matching checksum file `carrier-<commit-sha>-<os>-<arch>.zip.sha256`.
2. **Checksum**
   - Verification command is available for your OS and returns success.
3. **Daemon start**
   - `carrier daemon` starts and prints a usable `PAIR_CODE`.
4. **Pair**
   - `/pair <code>` returns success before TTL expires.
5. **Add path**
   - `carrier add openclaw` completes successfully.
6. **Start/Status**
   - `/start openclaw` succeeds; `/status openclaw` returns healthy/running.
7. **Fallback path**
   - On failure, `/diagnose openclaw` generates a support artifact.

### 流程验收清单（下载 → 配对 → add/启动）

可用下面清单快速确认 README 流程是否可执行：

1. **下载**：能在 [Releases](https://github.com/Keith-CY/carrier/releases) 找到匹配 ZIP 与 `.sha256`。
   - 产物命名建议为 `carrier-<commit-sha>-<os>-<arch>.zip`，对应校验文件为 `carrier-<commit-sha>-<os>-<arch>.zip.sha256`。
2. **校验**：对应系统校验命令可用且结果成功。
3. **启动 daemon**：`carrier daemon` 启动后可看到可用 `PAIR_CODE`。
4. **配对**：`/pair <code>` 在有效期内返回成功。
5. **安装链路**：`carrier add openclaw` 能成功完成。
6. **启动与状态**：`/start openclaw` 成功，`/status openclaw` 显示 healthy/running。
7. **兜底诊断**：失败场景可通过 `/diagnose openclaw` 产出诊断文件。

## 三平台 Bot 管理（超详细步骤，可直接照做）

本节用于“聊天侧运维命令（start/stop/status 等）”的场景。  
安装必须通过 Carrier TUI/WebUI 完成，聊天里的 `/install` 已禁用。

### 先准备（缺一不可）

在开始前，请向项目管理员索取以下信息：

1. Telegram 机器人入口（用户名或邀请链接）
2. Discord 机器人入口（服务器+频道，或私聊入口）
3. 飞书机器人入口（会话入口）
4. 一次性配对码 `PAIR_CODE`（示例：`pair-4e72e19a9f2a`）

如果缺少以上任意一项，本流程无法完成。

### 固定规则（所有平台都一样）

1. 命令必须发给机器人账号（Bot），不能发给真人账号
2. 每个平台第一次都要先执行：`/pair <PAIR_CODE>`
3. 配对成功后，才能执行 `/agents`、`/start openclaw` 等管理命令
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
   - `/start openclaw`
   - `/status openclaw`
9. 确认状态回复包含 `healthy`（或“健康”）

### 常用命令（可复制）

```text
/pair <PAIR_CODE>
/agents
# 安装请在终端执行：carrier add openclaw
/start openclaw
/status openclaw
/logs openclaw 200
/diagnose openclaw
/stop openclaw
```


### 预期响应示例

以下是通用响应示例（不含真实 ID 或令牌）：

**配对成功 (`/pair`)：**
```
✅ 配对成功。你现在可以使用 /agents、/start、/status 等管理命令。
```

**代理列表 (`/agents`)：**
```
可用代理:
- openclaw (v0.1.0) — 已停止
```

**健康状态 (`/status openclaw`)：**
```
代理: openclaw
版本: 0.1.0
状态: running
健康: healthy
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
   - 处理：先在终端执行 `carrier add openclaw`，再执行 `/start openclaw`
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

This section covers chat-side management commands (`/start`, `/stop`, `/status`, etc.).  
Installation must be done in Carrier TUI/WebUI because chat `/install` is intentionally blocked.

### Prerequisites (all required)

Before starting, ask your project administrator for:

1. Telegram bot entry (username or invite link)
2. Discord bot entry (server + channel, or DM entry)
3. Feishu bot entry (chat entry)
4. One-time pairing code `PAIR_CODE` (example: `pair-4e72e19a9f2a`)

If any of the items above is missing, this flow cannot be completed.

### Fixed rules (same for all platforms)

1. Send commands to a bot account, not to a human account
2. On each platform, run pairing first: `/pair <PAIR_CODE>`
3. Only after pairing succeeds, run `/agents`, `/start openclaw`, and other management commands
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
   - `/start openclaw`
   - `/status openclaw`
9. Confirm status response includes `healthy`

### Common commands (copy-paste)

```text
/pair <PAIR_CODE>
/agents
# Install via terminal: carrier add openclaw
/start openclaw
/status openclaw
/logs openclaw 200
/diagnose openclaw
/stop openclaw
```


### Expected response examples

Below are generic response examples (no real IDs or tokens):

**Pairing success (`/pair`):**
```
✅ Paired successfully. You can now use /agents, /start, /status, and other management commands.
```

**Agent listing (`/agents`):**
```
Available agents:
- openclaw (v0.1.0) — stopped
```

**Healthy status (`/status openclaw`):**
```
Agent: openclaw
Version: 0.1.0
Status: running
Health: healthy
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
   - Action: run `carrier add openclaw` in terminal first, then run `/start openclaw`
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
