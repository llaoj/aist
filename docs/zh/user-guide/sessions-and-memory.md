# 会话恢复与本机环境记忆

## 历史会话持久化与恢复（`aist sessions`）

一旦产生真实对话轮次（交互提问或管道分析），每一条消息在产生时即实时追加写入 `~/.aist/sessions/<id>/`（目录权限 `0700`，文件权限 `0600`），按 `Ctrl+C` 退出或终端意外断开都不会丢失已有排障记录（打开 `aist` 未发送任何消息直接退出不会生成空会话记录）。

快速恢复最近一次会话并直接开始对话：

```bash
aist -c
aist -c "继续查一下刚才的 nginx 日志"
```

需要从更早的历史记录中选择时，随时列出并恢复最近会话：

```bash
aist sessions
```

```text
选择会话
  2026-11-21 09:15  排查 /etc/nginx/conf.d/api.conf 的 502 超时  (14 messages)
  2026-11-20 18:42  检查线上 redis 内存占用                     (8 messages)
↑↓ 选择 · Enter 恢复 · Ctrl+C 退出
```

- 恢复会话会原样重放完整的历史对话、命令输出与文件修改 Diff（界面标签自动跟随当前 `aist lang` 语言渲染），**不会**重新执行历史命令；
- 管道分析（`cmd | aist`）同样会保存为历史会话，恢复后可直接切换进交互模式继续追问或执行排障命令；
- 默认保留最近 **10 个会话**（管理员可在服务端账号 `settings.session.max_sessions` 中按用户调整保留数量）。

## 自动上下文压缩（Compaction）

长时间排障产生大量命令输出时，一旦估算上下文达到有效预算（`context_window - max_output_tokens`，默认 `128000 - 8192 = 119808`）的触发阈值（默认 `trigger_ratio = 0.85`），客户端会自动压缩早期对话：

1. 完整保留开头的系统提示词，以及尾部最近约 `20,000` Token 的完整轮次（最新一轮无论多大始终完整保留）；
2. 将中间较早的历史轮次通过服务端代理提炼为结构化 `<compressed_history>` 摘要，精准保留主机名、IP、端口、路径、版本号、报错原文、已执行过的命令与待验证假设；
3. 压缩记录追加到 `messages.jsonl`，原始消息依然完整保留供界面回看，同时确保后续模型请求不超长、响应更敏捷。

## 本机环境记忆（Memory）

**Memory** 用于让 AIST 跨会话记住你所负责基础设施的长期事实——如服务器别名、集群节点角色、日志目录、配置路径与常用拓扑——无需每次开新会话都重复交代背景。

### 隐私与安全设计

- **100% 仅保存在客户端本机**：以纯文本 Markdown 文件存放在 `~/.aist/memory/*.md`（权限 `0600`），**不会同步或入库到服务端数据库**。启动时仅把精简目录（名称、分类、一行摘要、更新时间）注入提示词，具体内容由模型按需调用 `memory_search` 检索。
- **纯自然语言对话维护**：不设 `aist memory` 子命令，你只需在对话中告诉它（如*“记住 prod-web1 是 192.0.2.10，Nginx 配置在 /etc/nginx/conf.d”*），或由模型在排障确认事实后调用 `memory_save` 自动写入并提示 `✓ 已记住: prod-web1`。
- **只作环境事实，不作指令执行**：系统提示词明确约束模型将 Memory 视为静态环境知识，防止提示词注入。
- **敏感凭据自动拦截**：写入前自动剥离 `<private>...</private>` 片段，并拒绝包含私钥（`-----BEGIN ... PRIVATE KEY-----`）、密码/口令/Token 赋值、`Authorization: Bearer`、`AKIA...` 或 `sk-...` 等敏感秘密的内容。

### Memory 文件结构示例（`~/.aist/memory/prod-k8s.md`）

你随时可以用普通编辑器查看、修改或直接删除 `~/.aist/memory/*.md` 文件：

```markdown
---
updated: 2026-11-21T10:00:00+08:00
source: user statement + verified commands
summary: 生产 K8s 集群，控制面 192.0.2.1，3 台工作节点
type: k8s
---

# prod-k8s

## Memory

- control-plane: k8s-cp-01 (192.0.2.1)
- workers: 192.0.2.2, 192.0.2.3, 192.0.2.4
- ingress: 命名空间 ingress-nginx 下的 nginx-ingress
```

> [!NOTE]
> 系统提示词中的 `<memory_catalog>` 目录在 `aist` 启动时构建一次（总预算上限 `32 KiB`）。会话中途新保存的记忆在当次会话中即可立刻被 `memory_search` 搜到，并在下次启动 `aist` 时自动进入系统目录块。
