# Carrier Kanban 使用优化方案（Repo 配套）

该仓库已有看板自动同步：`.github/workflows/carrier-kanban-automation.yml`

## 一、已完成的自动化优化
- Issue/PR 变化会同步到 GitHub Project。
- 扩展了同步事件：
  - `opened`, `reopened`, `closed`, `edited`, `labeled`, `unlabeled`, `assigned`, `unassigned`
- 自动推断并回写字段（按你已有字段名）：
  - `Priority`（优先级）
  - `Type`（类型）
  - `Iteration`（迭代/里程碑）
  - `Estimate Hours`（工时）
  - `Blocked Reason`（阻塞说明）
  - `Status`（状态）
- 不再硬编码 Option ID，改为按字段名/选项名动态匹配，容错更强。

## 二、建议的 Project 字段（与自动化对齐）
请在 GitHub Project 中确认以下字段存在：

- 单选字段 `Status`
  - 选项：`To Do`、`In Progress`、`Blocked`、`Done`
- 单选字段 `Priority`
  - 选项：`P0`、`P1`、`P2`、`P3`
- 单选字段 `Type`
  - 选项：`Bug`、`Feature`、`Infra`、`Docs`、`Refactor`、`Release`
- 单选字段 `Iteration`
  - 选项按当前 sprint/里程碑命名
- 数值字段 `Estimate Hours`
- 文本字段 `Blocked Reason`
- 日期字段 `Due Date`（项目里建议按 `YYYY-MM-DD` 维护）

## 三、建议视图（可直接在 Project 创建）

### 1. 看板总览视图
- 分组：`Status`
- 显示字段：`Title, Assignees, Priority, Estimate Hours, Iteration, Due Date, Blocked Reason`
- 说明：用于追踪流程与瓶颈。
- WIP 建议：`In Progress` 每人最多 2～3 项。

### 2. 个人负载视图
- 分组：`Assignees`
- 排序：`Status`, `Priority`, `Iteration`
- 显示字段：`Title, Status, Estimate Hours, Type, Priority`
- 说明：每人工作量一眼可见。

### 3. 迭代视图
- 分组：`Iteration`
- 过滤：`Iteration != Backlog`
- 显示字段：`Title, Status, Priority, Assignees, Estimate Hours, Blocked Reason`
- 说明：按迭代追踪进度。

### 4. 风险视图
- 分组：`Status`
- 过滤：`Status = Blocked`
- 显示字段：`Title, Assignees, Blocked Reason, Iteration, Priority`
- 说明：专门盯住阻塞。

### 5. 交付进度视图（趋势）
- 过滤：`Status in Done/To Do/In Progress/Blocked`
- 排序：`Due Date`
- 显示字段：`Title, Assignees, Status, Due Date, Estimate Hours`
- 说明：用于预警即将到期。

## 四、统计看板（用于周会）
在 Project 侧使用筛选+保存视图并导出统计（或结合 GraphQL 导出）：

- 完成率：`Done / (To Do + In Progress + Blocked + Done)`
- 人均在手量：每位 `Assignees` 下 `In Progress + Blocked` 数量
- 人均在手工时：每位这些任务 `Estimate Hours` 求和
- 逾期率：`Due Date < 今日` 且 `Status != Done`
- 阻塞率：`Blocked` 占比
- 交付风险数：`Iteration` 下 `In Progress + Blocked + 即将到期`

## 五、使用约定（用于让自动化有效）
- 新建 issue/PR 时，优先打上 `P0`~`P3`、`Bug`/`Feature`/`Infra` 之一。
- 用 label 表示 sprint（如 `sprint-2026-02-W07`）或在 milestone 填写迭代名。
- 在 issue 描述中可写（供估时识别）：`estimate: 3`。
- 阻塞时加 `blocked` 标签并补 `Blocked Reason` 内容。
