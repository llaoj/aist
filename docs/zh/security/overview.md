# 数据流向、密钥托管与信任边界

AIST 在架构上将**模型密钥托管与团队治理策略**收敛在服务端，将**主机交互执行与本机环境记忆**保留在客户端，明确隔离各类敏感数据的流向。

## 核心资产与数据流向矩阵

| 数据 / 凭据类型 | 持久化位置 | 是否发往 AIST 服务端 | 是否发往上游大模型 |
|---|---|---|---|
| 上游 Provider API Key（`api_key`） | 服务端 SQLite（`upstream_providers`） | 仅保存在服务端（后台接口回显掩码 `••••••••`） | 仅在转发至对应服务商时置于 `Authorization` 头 |
| 客户端登录凭证 | 客户端 `~/.aist/token.json`（`0600`） | 作为 `Bearer <jwt>` 鉴权头发送 | **绝不发送** |
| 客户端语言偏好 | 客户端 `~/.aist/preferences.json`（`0600`） | **绝不发送** | 仅作为系统提示词末尾的回复语言要求 |
| 本机环境记忆（Memory） | 客户端 `~/.aist/memory/*.md`（`0600`） | 服务端不落库，仅随当前会话请求流转目录摘要与按需检索片段 | 随当前会话上下文转发给选中的候选模型 |
| 文件编辑历史备份 | 客户端 `~/.aist/fileedit/backups/`（`0600`） | **绝不发送** | **绝不发送** |
| 会话消息与截断后的工具输出 | 客户端 `~/.aist/sessions/<id>/`（`0600`） | 经 `/v1/proxy/chat/completions` 代理（其中 `shell_execute`/`file_edit` 记入 `_logs` 审计，受 `MaxDays` 控制） | 转发给选中的候选模型 |
| 终端交互口令（`sudo` / SSH / MFA） | 用户在本地或远端 PTY 原位输入 | **绝不发送**（正常密码提示关闭回显） | **绝不发送** |

## 服务端安全机制

- **零密钥泄露投影**：认证 Hook（`OnRecordAuthRequest("users")`）自动剥离用户记录中的 `allowed_models`、`weekly_token_quota` 与原始 `settings`，仅向客户端返回脱敏后的 `meta.config` 快照。
- **可选落盘加密与强制掩码**：支持原生传入 `--encryptionEnv=<环境变量名>` 对 PocketBase `Settings`（SMTP 密码、备份密钥）及 `upstream_providers.api_key` 启用 AES-256-GCM 落盘加密；无论是否开启加密，管理接口读取 `upstream_providers` 时均将非空 `api_key` 掩码为 `••••••••`，杜绝浏览器抓包泄露明文。
- **全表最小权限锁死**：`users`、`upstream_providers`、`upstream_models`、`daily_model_usage`、`invite_codes` 全部关闭公开集合读写规则（`nil`），仅允许超级管理员后台或受控服务端路由访问。
- **Web 安全响应头与限流**：内嵌站点启用严格的 `Content-Security-Policy`、`X-Content-Type-Options: nosniff`、`X-Frame-Options: SAMEORIGIN`、`Referrer-Policy: same-origin`、请求体体积限制以及注册接口限流规则。

## 客户端安全机制

- **严格目录与文件权限**：启动时强制确保 `~/.aist` 及各子目录权限为 `0700`，所有状态文件权限为 `0600`；目录创建或权限收紧失败时直接中止启动，不做静默降级。
- **人机确认防线**：每一次 `shell_execute` 与 `file_edit` 均须用户在终端明确确认，「修改命令」与文件写入还会额外触发二次风险确认提示。
- **私钥与凭据主动拦截**：
  - `file_edit` 硬性拒绝操作私钥文件名（`id_rsa`、`*.pem`、`*.key`、`ssh_host_*_key`、含 `private_key` 路径）、特殊文件系统（`/proc`、`/sys`、`/dev`）、符号链接以及包含私钥头部的内容；
  - `memory_save` 自动清理 `<private>...</private>` 标签，并拒绝保存包含私钥、密码/Token 赋值、`Bearer` 令牌、`AKIA...` 或 `sk-...` 的文本。
- **安装包完整性强校验**：`run.sh` 强制核对 `checksums.txt` 的 SHA-256，校验失败立即中止安装（Fail-Closed）。

## 漏洞披露与联系

请**不要**在公开 GitHub Issue 中提交安全漏洞。请通过 GitHub Security Advisories 私密报告或发送邮件至 `security@aistrun.cc`（我们会在 3 个工作日内确认响应）。
