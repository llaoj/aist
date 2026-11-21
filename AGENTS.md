# AGENTS.md

本文件记录本仓库的协作约定，供开发者与 AI 助手遵循。

## 网络代理

网络访问失败时（如 `go mod`、`gh`、`curl` 超时），可临时启用本机 VPN 代理：

```sh
export https_proxy=http://127.0.0.1:26800 http_proxy=http://127.0.0.1:26800 all_proxy=socks5://127.0.0.1:26800
```

仅在当前 shell 生效，不要把代理地址写入代码或提交。

## 构建、测试与检查

- 构建全部客户端与服务端产物：`make build`（依次调用 `./scripts/build-client.sh` 与 `./scripts/build-server.sh`）
- 单独构建：`make build-client` / `make build-server`
- 全部 Go 测试：`go test ./...`（或 `make test`）
- 端到端测试：`make test-e2e`（全部：`make test-e2e-all`）
- 文档站构建与死链检查：`npm --prefix docs run build`
- 格式化：`make format`（`gofmt -w -s .`）
- 命名检查：`make naming-check`
- 提交前至少保证：`gofmt -l` 无输出、`go vet ./...` 与 `go test ./...` 通过。

## 敏感信息与密钥

- 绝不在终端输出、日志、提交信息、文档或对话上下文中打印 token、密码、API Key、secret。
- 不读取 `.env.local`、`.env` 等可能含密钥的文件到上下文；需要时只在 shell 中 source 并直接使用。
- 生成 token 后直接通过管道写入目标 secret，例如：
  `flyctl tokens create deploy -a <app> -x 8760h | gh secret set FLY_API_TOKEN --repo <owner>/<repo>`
- 禁止 `echo $TOKEN`、`printenv`、`env`、`set -x` 等可能暴露密钥的写法。
- 文档和示例只使用占位符，如 `<password>`、`<token>`。
- 使用最小权限、可轮换、有有效期的 token；泄露后立即轮换。
- 命令输出可能包含敏感值时，先重定向或脱敏，不要把原始输出放进上下文。

## 代码约定

- 无特殊情况不写注释；注释只解释“为什么”，不复述“是什么”。
- 遵循目标文件既有的风格、导入分组与命名。
- 新增依赖前先确认仓库已使用该库。

## 消除重复实现（重要）

- **同一逻辑不在多个包中重复实现。** 一旦发现两处（或更多）功能相同的函数/常量/工具逻辑，应抽取到共享包，而不是各自拷贝一份。
- 通用的小工具（字符串处理、路径处理、格式化等）统一放在 `internal/util`，以导出函数供各包复用。例如：
  - `util.FirstNonEmpty`：返回首个非空白字符串。
  - `util.ExpandHome`：展开 `~` / `~/` 前缀。
- 抽取到共享包后，删除各包内的本地副本，并同步修改所有调用点；随后执行 `gofmt`、`go vet`、`go test` 验证。
- 判定“重复”：函数签名与行为一致（或可安全合并为超集行为）。若两处实现的行为、边界、错误语义确有不同（例如不同的截断规则），不要强行合并，保持独立并在必要时说明差异。
- 测试辅助函数（如 `_test.go` 内的构造器）若在多个包中重复，且需要共享，放到专门的可导入测试辅助包；不复制粘贴。注意仅被测试使用的工具不要混入生产代码包。

## 包边界

- 服务端逻辑在 `cmd/server` + `internal/server`，客户端在 `internal/*` 与 `cmd/aist`。
- 客户端与服务端的配置边界、接口契约见 `docs/zh/reference/api-and-errors.md`、`docs/zh/reference/configuration.md`。
- 会话、Memory、状态目录、安全边界与部署分别见 `docs/zh/user-guide/sessions-and-memory.md`、`docs/zh/reference/cli-and-state.md`、`docs/zh/security/overview.md`、`docs/zh/deployment/docker-compose.md`、`docs/zh/contributing/architecture.md`。
- 工具审计日志（客户端工具结果格式、服务端采集与日志结构）见 `docs/zh/administration/audit-logs.md`。

## 命名规范

完整命名规范见 `docs/zh/contributing/naming.md`（英文版 `docs/contributing/naming.md`），它是统一依据。代码、测试、CLI、文档和提交信息都应遵循。本仓库不保留历史命名兼容：名称不合适时直接改调用方、测试和文档，不加过渡别名。要点：

- **领域词汇**以 `docs/zh/contributing/naming.md §1` 的词汇表为准（如 `session.Manager`、`interactive.Session`、`Turn`、`Message`、`ToolCall`、`ToolResult`、`ToolOutcome`、`Target`、`memory.Entry`、`config.Settings`、`llm.ServerModel`、`Renderer`）。不得引入同义词（如 `Conversation`、`Agent`、`Round`、`Session.Service`、`llm.Manager`、`memory.Provider`）。
- **Go 标识符**：导出用 `PascalCase`，包内私有用 `camelCase`；缩写统一大写（`ID`、`URL`、`API`、`LLM`、`SSH`、`JSON`、`CLI`、`HTTP`）；不用下划线（测试名除外）；避免包名与类型名重复（`state.Root`、`config.Settings`、`memory.Entry`）。
- **构造函数**用 `NewX`；仅当包内只有一个主要类型时才允许裸 `New`，否则禁止。
- **转换函数**：转入目标领域用 `toX`，转出用 `fromX`；导出可用 `ToX`，但同文件保持一致。
- **输出动词**：纯转换返回字符串用 `Render`；直接写终端用 `Print`；交互输入用 `Prompt`；写文件用 `Write`/`Save`；追加日志/事件用 `Append`/`Record`；应用变更用 `Apply`。不要用 `Display` 表示打印。
- **布尔值**正向、具体（如 `needsFollowUp`、`skipConfirm`），避免 `handled`、`stop`、`ok` 等裸布尔。
- **错误**：哨兵用 `ErrX`，错误类型用 `XError`，包装用 `fmt.Errorf("...: %w", err)`；不用动态字符串表示可比较的错误条件。
- **枚举**统一 `<Type><Value>`（如 `CommandTypeQuery`、`ToolOutcomeSuccess`）；零值必须明确、安全，未知用 `Unknown`，同一枚举常量前缀一致。
- **结构体/接口**：持久化端口 `Repository`、内存快照 `Snapshot`、应用编排 `Manager`、适配器 `FileLoader`/`FileWriter`/`SSHRunner`、请求结果 `<Domain>Request`/`<Domain>Result`、选项 `<Type>Options`；同包最多一个裸 `Options`。
- **文件与包名**：包名小写单词、不下划线；文件名小写、多词用下划线（`model_manager.go`）并匹配主要内容；测试文件名与被测文件同名加 `_test.go`；Shell 脚本放 `scripts/` 用 kebab-case。
- **查询方法**不使用 `Get` 前缀（用 `LookupProvider`、`EffectiveContextWindow` 等）。
- **禁止出现 `docs/zh/contributing/naming.md §4` 列出的旧名称**；提交前运行 `make naming-check`，并确保 `gofmt`、`go vet ./...`、`go test ./...` 通过。
- **领域包边界**：领域包不依赖 UI、CLI、环境变量和全局状态；适配器可依赖领域包；领域包间只通过端口和明确的领域类型交互，禁止为命名方便制造循环依赖。
