# 核心模块设计详解

本文面向参与核心代码演进的贡献者，浓缩梳理客户端与服务端各关键模块的内部机制与协作契约。

## 1. 启动链路与组合根（`internal/cmd` → `internal/app`）

1. **CLI 前置阶段（`internal/cmd/root.go`）**：
   - 无状态命令（`version`、`help`、`completion`、`__complete`）完全跳过 `~/.aist` 初始化；
   - 其余命令先调用 `state.EnsureRoot("")`（确保目录 `0700`）与 `preferences.Store.LoadOrInit()`（确保 `preferences.json` 存在；`aist lang <zh|en>` 带参写入时跳过前置读取以支持损坏文件自愈）；
   - 免登录子命令（`login`、`logout`、`whoami`、`lang`）到此直接执行，不触发联网刷新。
2. **应用装配（`internal/app/app.go`）**：
   - 读取本地 `preferences.json` 语言偏好，执行 `account.Startup(ctx)`（调用 `auth-refresh` 续期并获取最新 `meta.config`），转换为不可变的 `config.Settings` 快照，一次性装配 `i18n`、`llm.ServerModel`、`memory.Manager`、`prompt.Builder`、`commandguard.Guard`、`fileedit.Editor`、`executor.CommandRouter` 与 `session.Repository`。

## 2. 提示词分层与动态注入（`internal/prompt` & `internal/tools`）

- **英文指令底座 + 本地语言后缀**：所有基础提示词模板、4 个工具的 JSON Schema 及回写状态头均使用英文编写以保证跨模型指令遵循稳定性；`prompt.Builder` 根据本地 `preferences.json` 仅在交互模式和管道模式末尾追加显式回复语言指令（压缩提示词保持纯英文）。
- **双占位符按需注入**：
  - `{{MEMORY}}`：注入 `32 KiB` 预算内的 `<memory_catalog>` 目录（仅含名称、类型、摘要、更新时间，正文按需走 `memory_search`）；
  - `{{BLACKLIST}}`：非空时注入 `4 KiB` 预算内的 `<command_blacklist>` 与反绕过规则，为空时零额外 Token 开销。

## 3. 会话持久化、Token 估算与上下文压缩（`internal/session`）

- **语言无关持久化**：`messages.jsonl` 只保存语言无关的 `ToolOutcome`（`success` / `failed` / `skipped` / `rejected`）与原始参数，`aist sessions` 恢复时通过 `tools.DecodeInvocation` 按当前语言实时重渲染。
- **消息链自愈校验**：发往模型前，`ModelMessages()` 自动校验 `assistant.tool_calls` 与 `role=tool` 配对（补齐中断缺失的占位结果、丢弃孤立/重复工具消息、合并相邻 `user` 消息），确保严格符合 OpenAI 工具调用协议。
- **锚点增量启发式估算**：不引入庞大词表依赖，按自然语言与结构化文本（符号占比 ≥ 25% 或工具消息）分类赋权估算，并以最近一次上游返回的 `usage.total_tokens` 作为增量锚点（`TokenAnchor`）。
- **整轮边界压缩（`planner.go`）**：按真实 `user` 消息切分 `Turn`，保护开头 `system` 消息与尾部 `keep_tokens`（最新一轮始终完整保留），将中间历史压缩为 `<compressed_history priority="high">` 合成消息并写入 `compaction` 事件。

## 4. 工具结果头置格式与服务端防丢键截断（`internal/interactive` → `internal/server/audit.go`）

- 客户端 `renderToolResult` 固定将短小的关键元数据行（`Status`、`Target`、`Exit Code`、`Error`、`Command` / `Path` / `Sudo`、`Detail`）排在最前，`[Output]` 大段输出排在最后。
- 服务端 `recordToolAudit` 仅提取最后一条 `assistant` 消息之后的新增 `role=tool` 结果，按 UTF-8 字符边界做字段前缀截断（`result` 6000 B、`command`/`old`/`new` 4000 B），并通过 `limitAuditAttrs` 将 JSON 转义膨胀后的总大小严格压在 `14 KB` 以内，避免触发 PocketBase 默认 `16 KB` 上限按字母序丢弃 `type`、`tool`、`userIP` 等尾部关键字段。

## 5. 服务端代理调度流水线（`internal/server/proxy.go` & `model_availability.go`）

1. 复用 PocketBase 原生 `apis.RequireAuth("users")` 中间件完成鉴权并注入 `e.Auth`；
2. 在候选重试循环之外执行一次 `recordToolAudit`（避免 Fallback 重复记审计）；
3. 汇总该用户本周一 `00:00 UTC` 以来的 `daily_model_usage.total_tokens` 校验 `weekly_token_quota`；
4. 读取用户 `allowed_models` 中已启用且所属 Provider 已启用的模型，按 `(priority ASC, provider ASC, model ASC)` 排序并跳过 `auth` 阻塞及仍在 `unavailable_until` 冷却期内的模型；
5. 按单模型 `max_output_tokens` 裁剪请求，依次尝试上游：成功则自动清除到期冷却状态并原子累加 `daily_model_usage`；失败则按错误分类更新冷却退避或直接返回结构化错误码。
