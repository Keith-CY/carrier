# Carrier Control Plane Backlog Issue Drafts

这些 issue 刻意放在 `Track 1-5` 之后，避免在 execution lifecycle、observability、RBAC、policy scope 稳定前继续扩散产品面。

可直接复制到 GitHub issue；每条都已经包含标题、背景、范围、验收标准和非目标。

---

## Issue 1

**Title**
Execution templates and launch presets

**Problem**
当前 `carrier orchestrate` 和 WebUI Quick Launch 仍以自由输入 goal 为主。对首次接触产品的用户，以及需要重复运行同类流程的平台团队，这个入口过于抽象，不利于复用，也不利于做 policy、audit 和 KPI 归因。

**Goal**
引入一层模板化 execution launch，先覆盖最值得演示和复用的 4 个模板：
- `pr-triage`
- `issue-investigation`
- `incident-diagnosis`
- `rollout-smoke-check`

**Scope**
- 定义 template schema：
  - `id`
  - `name`
  - `description`
  - `inputSchema`
  - `defaultGoalTemplate`
  - `defaultPolicyHints`
  - `defaultWorkerHints`
  - `defaultOutputSchema`
- Gateway API:
  - `GET /api/v1/templates`
  - `GET /api/v1/templates/:id`
  - `POST /api/v1/templates/:id/launch`
- CLI:
  - `carrier templates`
  - `carrier templates show <id>`
  - `carrier templates run <id>`
- WebUI:
  - Dashboard Quick Launch 增加 `Template` 模式
  - 每个模板有结构化输入表单
  - launch 后仍进入现有 execution detail
- 审计：
  - execution 写入 `templateId`
  - policy scope 能按 template 命中

**Acceptance Criteria**
- 至少 4 个模板可通过 CLI 和 WebUI 启动
- 模板 launch 后生成标准 execution，对 retry/rerun/clone 无破坏
- policy scope 能基于 `templateId` 生效
- Playwright 覆盖至少 1 个模板的 launch 主路径

**Non-Goals**
- 不做模板 marketplace
- 不做用户自定义模板编辑器
- 不做复杂模板版本管理

**Dependencies**
- Track 1
- Track 2
- Track 5

---

## Issue 2

**Title**
Trigger system for GitHub, webhook, and schedule launches

**Problem**
Carrier 目前能手动启动 execution，但还缺少进入真实工作流的自动入口。没有 trigger，execution 仍然像一个需要人工点击的 control plane，而不是一个能接入工程流程的执行面。

**Goal**
支持 3 类受控 trigger：
- GitHub trigger
- Webhook trigger
- Scheduled trigger

**Scope**
- 定义 trigger model：
  - `id`
  - `type`
  - `templateId`
  - `enabled`
  - `config`
  - `policyOverrides`
  - `createdBy`
- Gateway API:
  - `GET /api/v1/triggers`
  - `POST /api/v1/triggers`
  - `PATCH /api/v1/triggers/:id`
  - `DELETE /api/v1/triggers/:id`
  - `POST /api/v1/triggers/webhook/:id`
- GitHub first cut:
  - PR comment trigger
  - issue label trigger
- Schedule first cut:
  - cron expression
  - UTC-based execution
- Audit:
  - execution 写入 `triggerSource`
  - execution 写入 `initiator`
  - payload digest / trigger metadata 可追踪

**Acceptance Criteria**
- 能从 webhook 创建 execution
- 能从 GitHub comment 或 label 创建 execution
- 能按 schedule 定时创建 execution
- execution detail 可见 trigger source 和 initiator
- trigger 创建/修改/删除受 RBAC 控制

**Non-Goals**
- 不做复杂 workflow engine
- 不做 hosted GitHub App
- 不做 multi-step chained trigger

**Dependencies**
- Track 1
- Track 4
- Track 5

---

## Issue 3

**Title**
Execution evidence bundle and audit export

**Problem**
现在 execution、policy、artifact、approval 已经存在，但还不能把一次 execution 的关键证据打包导出。对于审计、复盘、合规和对外同步，这会明显限制产品价值。

**Goal**
提供 execution evidence bundle，把一次 execution 的关键治理信息导出成一个稳定包。

**Scope**
- Evidence bundle 内容：
  - execution metadata
  - plan snapshot
  - policy snapshot
  - governance resolution
  - authorization / approval trace
  - worker assignment and lease summary
  - results summary
  - artifact manifest
- Gateway API:
  - `GET /api/v1/orchestrator/executions/:id/evidence`
  - `GET /api/v1/audit/export?executionId=<id>`
- 导出格式：
  - `json`
  - `zip` bundle
- WebUI:
  - execution detail 增加 `Export Evidence`
- CLI:
  - `carrier executions evidence <id>`

**Acceptance Criteria**
- 任意 completed/failed/cancelled execution 都可导出 evidence bundle
- artifact manifest 与实际 artifact API 一致
- approval 和 policy trace 在 bundle 中可见
- export 行为本身写入 audit log

**Non-Goals**
- 不做长期归档系统
- 不做外部 SIEM 直连
- 不做 PDF 报告生成

**Dependencies**
- Track 2
- Track 5

---

## Issue 4

**Title**
Provider governance v2 and cost attribution

**Problem**
当前已经能看到 requested/resolved provider 的一部分信息，但还不能系统展示 provider/model 层面的成功率、失败率和成本归因。这会限制平台团队做 provider 策略和成本治理。

**Goal**
把 provider governance 从“能解析”提升到“可分析、可归因、可治理”。

**Scope**
- execution 持久化补齐：
  - requested provider/model
  - resolved provider/model
  - resolution source
  - estimated cost / usage
- observability 增加 provider 维度聚合：
  - success/failure by provider
  - success/failure by model
  - latency by provider
  - estimated cost by provider/model
- WebUI:
  - provider resolution trace panel
  - provider metrics cards/table
- CLI:
  - `carrier executions show` 增加 provider trace
  - `carrier providers usage` 或等价统计入口

**Acceptance Criteria**
- execution detail 能完整显示 requested/resolved provider/model
- `/api/v1/orchestrator/metrics` 或相关新接口能返回 provider aggregates
- WebUI Observability 页面可见 provider failures 和 estimated usage/cost
- provider binding / resolution drift 有稳定展示

**Non-Goals**
- 不做真实账单结算系统
- 不做 provider-side token accounting 精确对账
- 不做跨组织成本分摊

**Dependencies**
- Track 4
- Track 5

---

## Issue 5

**Title**
Documentation and website repositioning around execution control plane

**Problem**
产品能力已经从“agent 安装器”明显走到“execution control plane”，但文档和对外叙事还没有完全收拢。继续沿旧叙事会让用户误判产品价值，也让 roadmap 的治理能力显得分散。

**Goal**
重写 README、quickstart、网站首页和 demo 路径，让产品定位统一到：
`self-hosted execution control plane for agentic workflows`

**Scope**
- 重写 README 首页结构：
  - what Carrier is
  - create execution
  - preview plan
  - approve
  - inspect
  - retry / rerun
- 重写 quickstart：
  - 首次体验以 execution 为中心，不以 install 为中心
- WebUI 一级导航和截图更新：
  - `Executions`
  - `Workers`
  - `Policies`
  - `Providers`
  - `Hosts`
  - `Observability`
- 增加一套 demo script：
  - create execution
  - blocked by policy
  - approve
  - export evidence

**Acceptance Criteria**
- 新用户 10-15 分钟内能跑通一次 governed execution
- README 第一屏不再以安装 agent 为主叙事
- 网站和 CLI 文档对 execution / policy / observability 的术语保持一致
- 至少有 1 个完整 demo runbook 可复现

**Non-Goals**
- 不做品牌重设计
- 不做多语言站点
- 不做营销站完整重构

**Dependencies**
- Track 1
- Track 2
- 建议等待 Issue 3 有初版后一起更新 demo

---

## Issue 6

**Title**
Approval workflow v2 with separate infrastructure and policy approvals

**Problem**
当前 approval 已经存在，但语义仍偏扁平。随着 policy scope、RBAC、templates 和 triggers 增加，单一 approval 状态会不足以表达“是谁、因为什么、在哪一步批准了什么”。

**Goal**
把 approval 拆成更清晰的工作流对象，至少区分：
- infrastructure approval
- policy approval
- future human-review approval slot

**Scope**
- execution authorization model 扩展：
  - `requiredApprovals`
  - `grantedApprovals`
  - `approvalState`
  - `approvalHistory`
- Gateway API:
  - `POST /api/v1/orchestrator/executions/:id/approve`
  - `POST /api/v1/orchestrator/executions/:id/reject`
  - 或在现有 authorize API 上扩展 typed approval payload
- WebUI:
  - pending approvals panel
  - approval history timeline
  - reject with reason
- CLI:
  - `carrier executions approve <id> --kind policy`
  - `carrier executions approve <id> --kind infrastructure`
  - `carrier executions reject <id> --kind policy --reason ...`

**Acceptance Criteria**
- execution detail 能明确显示当前缺哪类 approval
- approver 角色只能执行自己有权限的 approval kind
- approval/reject 都会写入 audit log
- policy-gated execution 与 infra-gated execution 可被区分展示

**Non-Goals**
- 不做多级企业审批流引擎
- 不做邮件审批
- 不做外部工单系统集成

**Dependencies**
- Track 5
- 建议在 templates 和 triggers 初步上线后推进

