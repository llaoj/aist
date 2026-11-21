# 多主机 SSH 与安全文件编辑

## 多主机 SSH 协同排障（`target`）

`shell_execute` 与 `file_edit` 原生支持 `target` 参数。你只需要在本地电脑或跳板机安装一个 `aist`，就能在同一轮对话中跨多台远端 Linux / macOS 主机排查故障与修改配置，无需在每台业务机器上部署客户端。

```text
用户：检查 cache-01 (192.0.2.21) 的 Redis 为什么在频繁驱逐 key，并和 cache-02 (192.0.2.22) 的 maxmemory 配置做对比。
```

### Target 安全机制

- **本机与远端无缝切换**：`target` 为空、`local` 或 `localhost` 时在本机执行；填写主机名、SSH 别名或 `user@host`（如 `web1`、`ubuntu@192.0.2.10`）时，自动调用系统 `ssh` 执行（有终端时使用 `ssh -t <target> <command>` 分配远端 PTY）。
- **绝不私自扫描主机配置**：AIST **不会**主动读取或爬取你的 `~/.ssh/config`、`known_hosts`、`/etc/hosts` 来猜测主机列表。所有远端目标只能来自你的明确指令、已保存的[本机环境记忆](./sessions-and-memory.md)，或你在本轮实际运行过的命令输出（如 `kubectl get nodes`）。
- **防参数注入校验**：严格拒绝以 `-` 开头的值（防止 SSH 选项注入）、`..`、空白与控制字符，长度上限 255 字节。
- **完全复用本机 SSH 体系**：`~/.ssh/config` 中的别名、`ProxyJump` 跳板机、SSH Agent 转发与硬件密钥均开箱即用。若连接返回退出码 `255`，客户端会明确提示使用 `ssh <target>` 检查连通性并停止，不会盲目猜测账号或改写 SSH 配置。

## 安全文件编辑（`file_edit`）

AIST 在命令层拦截了 `vim` / `nano` 等全屏编辑器，并提供专门的 `file_edit` 工具用于在本机或远端 SSH 主机上新建、修改常规文本配置文件，避免脆弱的 `sed -i` 拼接误伤文件。

### 五步安全写入闭环

1. **读取原文件并生成预览**：
   - 普通可读远端文件优先通过无 PTY 的 SSH 管道静默读取，让你在输密码前就能先看到 Diff；仅 root 可读文件则通过交互式 PTY `sudo` 读取。
   - 严格校验 `old_content` 在原文件中**恰好唯一匹配一次**（新建文件时 `old_content` 为空），生成彩色 Diff。
2. **终端可视化确认**：
   - 清晰展示修改原因、`[target] /path/to/file (sudo)`、彩色增删 Diff 以及状态修改警告。
3. **SHA-256 CAS 并发校验**：
   - 真正落盘前再次核对目标文件当前内容的 SHA-256 摘要；若在你审阅 Diff 期间文件被其他进程或同事改过，立即中止写入，拒绝静默覆盖他人变更。
4. **自动本地多版本备份**：
   - 覆盖已有文件前，先将原文件备份到客户端本机 `~/.aist/fileedit/backups/<镜像路径>/`（远端文件位于 `~/.aist/fileedit/backups/@remote/<host>/...`），每个文件自动去重保留最近 **10 个历史版本**（权限 `0600`）并记录 `meta.json`。
5. **保留权限原子替换**：
   - 先写临时文件并 `fsync`，继承原文件权限与属主（新建文件默认 `0600`），最后原子 `rename` 替换。

### 内置文件安全红线

| 限制项 | 规则说明 |
|---|---|
| 路径格式 | 必须为绝对路径（如 `/etc/nginx/nginx.conf`）或 `~/` 开头 |
| 特殊文件系统 | 禁止访问 `/proc/`、`/sys/`、`/dev/` |
| 文件类型与体积 | 仅限普通文件（拒绝目录、符号链接、设备文件），单文件及新内容上限 `1 MiB` |
| 私钥保护 | 拒绝读写私钥路径（`id_rsa`、`*.pem`、`*.key`、`ssh_host_*_key`、含 `private_key` 路径）及包含 `-----BEGIN ... PRIVATE KEY-----` 的内容 |
| `sudo` 提权 | 对当前用户可读但目录/文件属主需要 root 才能替换的配置文件，会自动启用 `sudo`；完全不可读的文件由模型显式传 `sudo=true` |

### 从本地备份恢复文件

如需回滚某次文件修改，在 `~/.aist/fileedit/backups/<镜像路径>/meta.json`（远端文件在 `@remote/<host>/...` 下）按时间查看 `versions` 列表，将对应的 `.bak` 文件复制回 `original_path` 并按 `mode` 恢复权限即可。
