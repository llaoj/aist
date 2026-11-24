# CLI 命令与状态目录（`~/.aist`）

## 客户端命令总表（`aist`）

| 命令 | 是否需要已登录 | 说明 |
|---|---|---|
| `aist` | 是 | 启动多轮交互式排障会话 |
| `aist "<问题>"` | 是 | 带初始问题进入交互排障会话 |
| `<cmd> \| aist ["<问题>"]` | 是 | 读取管道输入后进入交互会话（若标准输出被重定向或无 TTY，则单次只读分析后退出） |
| `aist sessions` | 是 | 列出本机历史会话并选择恢复 |
| `aist login [server] [-e email]` | 否 | 登录指定服务端并将 Token 写入 `~/.aist/token.json` |
| `aist logout` | 否 | 删除 `~/.aist/token.json`（保留 `preferences.json`、历史会话、Memory 与备份） |
| `aist whoami` | 否 | 查看当前登录邮箱、服务端地址及本周 Token 用量与剩余额度（离线时自动降级展示本地状态） |
| `aist lang [zh\|en]` | 否 | 查看或切换本地界面与 AI 回复语言（`~/.aist/preferences.json`） |
| `aist version` | 否 | 输出版本号、Commit 与构建时间（不初始化 `~/.aist`） |
| `aist completion <shell>` | 否 | 生成 `bash` / `zsh` / `fish` / `powershell` 自动补全脚本 |

## 服务端命令总表（`server`）

| 命令 | 说明 |
|---|---|
| `server serve [--encryptionEnv=<变量名>] [--dev]` | 启动 PocketBase 与 AIST 服务端（默认数据目录 `./pb_data`，容器内位于 `/var/lib/aist/pb_data`） |
| `server superuser upsert <email> <password>` | 创建或重置超级管理员账号密码 |
| `server version` | 输出服务端构建版本（不打开数据目录） |

## 客户端状态目录结构（`~/.aist`）

所有客户端可写状态固定收敛在 `~/.aist` 下（目录权限 `0700`，文件权限 `0600`）：

```text
~/.aist/
├── fileedit/
│   └── backups/<镜像原路径>/
│       ├── meta.json
│       └── <UTC时间戳>-<sha256前8位>.bak
├── sessions/
│   └── ses_<时间戳>-<8位hex>/
│       ├── session.json
│       └── messages.jsonl
├── memory/
│   └── <名称>.md
├── preferences.json
└── token.json
```

远端 SSH 主机上的文件修改备份保存在 `~/.aist/fileedit/backups/@remote/<target>/<镜像原路径>/`。

## 原子持久化与并发约定

- **原子替换写入（`storage.WriteFile` / `ReplaceFile`）**：`preferences.json`、`session.json`、`memory/*.md` 与文件备份均采用“同目录临时文件 → `fsync` → 原子 `rename`”落盘，避免崩溃留下半写文件。
- **追加式会话事件流**：`messages.jsonl` 使用 `O_APPEND` 单次追加并 `Sync()`；若异常断电导致最后一行不完整，恢复会话时会自动忽略尾部残行。
- **单写者约定**：客户端不设跨进程文件锁，请避免在两个终端同时操作同一个历史会话或并发编辑同一目标文件（`file_edit` 内置 SHA-256 CAS 校验拦截并发覆盖）。

## 内置资源与安全常量上限

| 项目 | 默认值 |
|---|---|
| 管道标准输入读取上限 | `1.6 MB`（送入模型上限 `240,000` 字符） |
| 单轮用户输入 / 单条工具结果截断 | `8,000` 字符（保留前 60% + 后 40%） |
| 单条 Shell 命令客户端内存缓冲 | `1 MiB`（前 `512 KB` + 后 `512 KB`） |
| 单条 Shell 命令默认执行超时 | `30 分钟`（输出结束后 `5 秒` 退出宽限） |
| 连续无人工确认的自动工具调用上限 | `15 轮`（经用户确认即清零） |
| `file_edit` 单文件及新内容上限 | `1 MiB` |
| `file_edit` 每个原文件保留备份数 | `10` 个去重历史版本 |
| 本机 Memory 总预算 / 单次保存上限 | 总计 `32 KiB` / 单次写入 `8 KiB` |
| 本机保留历史会话数（`max_sessions`） | 默认 `10` 个（服务端账号配置可调） |
