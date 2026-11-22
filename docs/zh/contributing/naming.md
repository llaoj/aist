# 代码与领域命名规范

本文是 AIST 的统一命名依据。代码、测试、CLI、文档和提交信息中的领域名称都应遵循这里的定义。

本项目不保留历史命名兼容。名称不合适时直接修改调用方、测试和文档，不增加过渡别名，并通过 `make naming-check` 自动守卫。

## 1. 领域词汇

| 概念 | 统一名称 | 说明 | 避免使用 |
|---|---|---|---|
| 持久化会话 | `session.Session` | 会话数据本身 | `Conversation` 作为类型名 |
| 会话管理器 | `session.Manager` | 追加消息、压缩、标题、持久化编排 | `session.Service` |
| 交互式会话 | `interactive.Session` | 面向用户的 LLM/工具循环 | `Agent`、`Runner` 作为同义类型 |
| 一轮用户请求 | `Turn` | 一条真实 user 消息及其后的 assistant/tool 消息 | `Round`、`Question` |
| 消息 | `Message` | 持久化的一条消息 | `Entry` 表示消息 |
| 工具调用 | `ToolCall` | 模型请求执行工具 | `Command` 泛指工具 |
| 工具结果 | `ToolResult` | 工具执行后写回会话的内容 | `Output` 泛指工具结果 |
| 工具结果状态 | `ToolOutcome` | `success` / `failed` / `skipped` / `rejected` | `ResultStatus`、`ToolStatus` |
| 工具后续调用 | `FollowUp` | 工具结果后额外发起的一次模型调用 | `Analysis` 泛指所有后续调用 |
| Shell 命令 | `ShellCommand` | 待执行或已解析的 shell 命令 | `Invocation` 表示命令数据 |
| 查询命令 | `CommandTypeQuery` | 只读、不改动系统状态的命令；界面显示「查询命令」 | 只读命令（作为类别名） |
| 修改命令 | `CommandTypeModify` | 会改变系统状态的命令；界面显示「修改命令」 | 写命令、状态变更命令 |
| 执行主机 | `Target` | 本地或 SSH 主机 | 文件写入目标、配置路径 |
| Memory 条目 | `memory.Entry` | 一条环境知识 | `memory.Memory` |
| Memory 变更 | `Change` | append / replace 的统一操作 | `Save` 仅偏追加含义 |
| 账号配置 | `config.Settings` | 服务端下发的运行时配置快照 | `config.Config`、`config.ProviderConfig`、`remote.ServerConfig` |
| Memory 注入提供方 | `memory.CatalogProvider` | 生成 Memory prompt block | `memory.Provider` |
| Prompt 集合 | `prompt.Set` | 会话使用的系统/后续/压缩提示词 | `prompt.Provider` |
| Memory 快照存储 | `memory.SnapshotStore` | 已加载 Memory 的内存快照 | `memory.Store` |
| 逻辑模型客户端 | `llm.ServerModel` | 客户端唯一模型入口，调用 server | `llm.Manager`、`llm.ModelManager`、`CloudModel` |
| 终端渲染器 | `Renderer` | 终端展示端口 | `Display` 表示端口 |
| 终端打印 | `PrintX` | 直接写 stdout | `DisplayX` |

术语分工：`user` 只表示登录身份（PocketBase `users`、`user_id`、`email`）；`account` 表示该身份的配置、凭证与配额用量聚合（`AccountProvider`、`remote.AccountConfig`、`remote.AccountUsage`、`config.Settings`）。不要用 `ServerConfig` 表示下发给客户端的账号配置，以免与服务端运行时配置 `server.Config` 混淆。

## 2. Go 命名规则

- **通用**：导出用 `PascalCase`，包内私有用 `camelCase`；缩写统一大写（`ID`、`URL`、`API`、`LLM`、`SSH`、`JSON`、`YAML`、`CLI`、`HTTP`）；不用下划线（测试名称除外）；避免包名与类型名重复（`state.Root`、`config.Settings`、`memory.Entry`、`preferences.Data`）。
- **构造函数**：使用 `NewX`（`NewManager`、`NewStore`、`NewEditor`）；仅当包内只有一个主要类型时才允许裸 `New`，多类型包禁止裸 `New`。
- **转换函数**：转入目标领域类型用 `toX`（导出可用 `ToX`），转出用 `fromX`，同文件保持一致。
- **输出动词**：纯转换返回字符串用 `Render`；直接写终端用 `Print`；交互输入用 `Prompt`；写文件用 `Write` / `Save`；追加日志/事件用 `Append` / `Record`；应用变更用 `Apply`。不要用 `Display` 表示“打印到终端”（`Display` 仅作名词如 `ToolDisplay`）。
- **布尔值**：使用正向、具体的名称（`needsFollowUp`、`skipConfirm`、`IsSupportedLanguage`），避免跨函数返回裸 `handled`、`asked`、`stop`、`state`、`ok`。
- **错误**：哨兵错误 `ErrX`，错误类型 `XError`，包装错误 `fmt.Errorf("...: %w", err)`；不用动态字符串表示可比较错误条件。
- **枚举**：统一 `<Type><Value>`（`CommandTypeUnknown`、`CommandTypeQuery`、`CommandTypeModify`）；零值必须明确安全，未知用 `Unknown`。
- **查询方法**：不使用 `Get` 前缀（用 `LookupProvider`、`EffectiveContextWindow`、`EffectiveMaxOutputTokens`）。
- **结构体与接口**：持久化端口 `Repository`、内存快照 `Snapshot`、应用编排 `Manager`、适配器 `FileLoader` / `FileWriter` / `SSHRunner`、请求/结果 `<Domain>Request` / `<Domain>Result`、选项 `<Type>Options`（同一包最多一个裸 `Options`）。
- **包边界**：领域包不依赖 UI、CLI、环境变量和全局状态；适配器可依赖领域包；禁止为命名方便创建循环依赖。
- **文件与包名**：包名小写单词无下划线（`session`、`fileedit`、`commandguard`、`preferences`）；文件名小写多词用下划线（`account_config.go`）；测试文件名同名加 `_test.go`；Shell 脚本放 `scripts/` 用 kebab-case（`naming-check.sh`）。

## 3. 包内规范要点

- `internal/config`：`Settings` 是服务端下发的账号配置聚合根，只保留 `limits`、`session` 和 `blacklist`（客户端语言偏好归属 `internal/preferences`）。
- `internal/llm`：`ServerModel` 是客户端唯一模型入口（固定请求 `auto`），`CompletionRequest` / `CompletionResponse` 表示调用输入输出。
- `internal/interactive`：`Session` 为交互编排器，`Turn` 为一轮请求，`FollowUp` 为工具后模型调用，`Decision` 使用 `DecisionApprove` / `DecisionSkip` / `DecisionRejectRemaining`。

## 4. 禁止出现的旧名称（`make naming-check`）

提交前必须通过命名检查，禁止以下废弃标识符重新出现：

```text
analyzeCommandOutput
needsAnalysis
AnalysisComplete
ContinueAnalysis
processQuestion
RunRestored
RunWithPipe
QueryCommand
ModifyCommand
ResultSuccess
ResultFailed
ResultSkipped
ResultRejected
ToolStatusSuccess
ToolStatusFailed
ToolStatusSkipped
ToolStatusRejected
SaveMode
SaveRequest
SaveResult
SaveAppend
SaveReplace
SaveDelete
Summarizer
SummaryRequest
SummaryResult
session.Service
memory.Service
memory.Provider
config.Provider
prompt.Provider
llm.Manager
CompactBlankLines
DisplayWelcome
DisplayShellCommand
EndRegion
clientConfig
clientLimits
buildClientConfig
remote.ServerConfig
CloudModel
CloudModelOptions
NewCloudModel
cmd/cloud
internal/cloud
deploy/cloud
scripts/build-cloud.sh
release-cloud
aist-cloud
AIST_CLOUD_*
```

## 5. 评审清单

- 函数名是否直接描述行为？类型名是否与 §1 领域词汇表一致？
- 枚举零值是否安全？布尔值是否正向、具体？
- 输出动词是否符合 `Render` / `Print` / `Prompt` / `Save` 规则？
- 是否修改了所有调用方、测试和文档？
- 是否运行了 `gofmt`、`go vet ./...`、`make naming-check` 和 `go test ./...`？
