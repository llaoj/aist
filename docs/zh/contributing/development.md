# 本地开发、构建与测试

## 开发环境要求

- **Go**：以 `go.mod` 声明版本为准（`1.27+`）
- **Git** 与标准 Shell（`bash` / `sh`）
- **Node.js 20+**：仅在本地预览或构建 `docs/` 文档站时需要（服务端与客户端构建均零 Node 依赖）

## 仓库目录结构

```text
cmd/
  aist/                 # 客户端 CLI 入口
  server/               # 服务端入口（基于 PocketBase Go 框架）
internal/
  app/                  # 组合根：账号配置快照、运行时依赖装配、会话创建
  auth/                 # 客户端登录态管理与滑动刷新
  buildinfo/            # 构建版本、Commit 与时间注入
  cmd/                  # Cobra 子命令：login/logout/whoami/lang/sessions/version
  commandguard/         # 基于 Shell AST 的命令黑名单匹配与全屏 TUI 程序拦截
  config/               # 服务端下发的账号配置模型（Settings）、归一化与校验
  executor/             # 本地与 SSH 命令执行器、PTY 窗口同步、输出缓冲
  fileedit/             # 结构化文件编辑（唯一匹配替换、SHA-256 CAS、本地多版本备份）
  i18n/                 # 中英文终端文案字典
  interactive/          # 面向用户的交互主循环、确认决策、历史重放
  llm/                  # OpenAI 兼容客户端与逻辑模型入口 ServerModel ("auto")
  memory/               # 本机环境记忆领域模型、Markdown 编解码、目录注入与检索
  preferences/          # ~/.aist/preferences.json 本地偏好读写与首次语言推断
  prompt/               # 系统提示词组装、Memory 目录与命令黑名单注入
  remote/               # PocketBase 认证与 /v1/proxy HTTP 客户端
  scriptexec/           # file_edit 读写辅助脚本执行通道
  server/               # 服务端路由、邀请码注册、代理容灾、用量限额、工具审计、后台品牌扩展
  session/              # 会话领域模型、JSONL 持久化、启发式 Token 估算、上下文压缩规划
  state/                # ~/.aist 目录布局、0700/0600 权限保证与镜像备份仓库
  storage/              # 原子写（WriteFile / ReplaceFile）原语
  target/               # 本地与远端 SSH Target 格式安全校验
  termio/               # 终端控制辅助工具
  token/                # ~/.aist/token.json 凭证读写
  tools/                # 内置工具 JSON Schema、参数解析与语言无关的 ToolDisplay
  ui/                   # 终端交互选择器、Markdown 渲染与加载状态
  util/                 # 跨包通用小工具（FirstNonEmpty、ExpandHome、TruncatePrefix 等）
migrations/             # PocketBase 表结构与初始 Settings 迁移
frontend/               # 内嵌进 server 二进制的公开静态站点与安装/卸载脚本
deploy/server/          # Dockerfile、compose.yaml、docker-entrypoint.sh、fly.toml
scripts/                # 跨平台构建、E2E 测试套件与命名规范检查脚本
docs/                   # VitePress 中英双语文档站
```

## 构建与本地启动

```bash
make build          # 构建全平台客户端（7 个目标）与服务端发布包至 dist/
make build-client   # 仅构建客户端发布包
make build-server   # 仅构建服务端发布包
make dev            # 开发模式启动服务端（默认监听 :8090，若存在 .env.local 会自动加载）
```

本地指定版本号测试构建：

```bash
VERSION=v0.6.0 COMMIT=$(git rev-parse --short HEAD) BUILD_DATE=$(date -u +%F) make build-client
```

## 代码检查与测试套件

每次提交或发起 PR 前，请确保以下四项检查全部通过：

```bash
make format         # gofmt -w -s .（保证 gofmt -l 无输出）
go vet ./...
make naming-check   # ./scripts/naming-check.sh（见代码与领域命名规范）
go test ./...       # 或 make test
```

端到端自动化集成测试：

```bash
make test-e2e       # 运行 file_edit E2E 与多主机/SSH/确认流程 E2E
make test-e2e-all   # 运行全部单元测试 + 所有自动化 E2E 套件
```

*（涉及真实终端 `sudo` 密码输入、硬件 MFA 与实时 `Ctrl+C` 信号的场景保留为终端人工验收项。）*

## 本地预览与构建文档站（`docs/`）

```bash
npm --prefix docs ci
npm --prefix docs run dev     # 本地热重载预览
npm --prefix docs run build   # 静态产物构建 + 全站死链严格检查
```

## 贡献者许可协议（CLA）

所有代码与文档贡献均适用本项目的 [CLA 贡献者许可协议](https://github.com/llaoj/aist/blob/main/CLA.md)。首次提交 PR 时，机器人会在评论区引导你回复一行声明完成签署。
