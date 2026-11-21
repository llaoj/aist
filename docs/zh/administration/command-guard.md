# 命令黑名单与全屏交互程序拦截

在弹出终端确认菜单之前，AIST 会通过 `internal/commandguard` 执行两道确定性的本地安全检查，并同步将黑名单注入系统提示词，从源头与执行端双重防止高危误操作或全屏卡死。

## 1. 账号级命令黑名单（`settings.blacklist`）

管理员可在 `users.settings.blacklist`（或 `invite_codes.settings.blacklist` 新用户模板）中按通配符配置禁止执行的命令规则：

```json
{
  "blacklist": [
    "rm -rf /",
    "rm -rf /*",
    "mkfs*",
    "dd if=* of=/dev/*",
    "shutdown*",
    "reboot*"
  ]
}
```

### 提示词约束 + Shell AST 语法树双重拦截

1. **系统提示词注入**：清洗去重后的黑名单（预算上限 `4 KiB`）会注入系统提示词的 `<command_blacklist>` 块，要求模型不得提议或变相绕过这些命令。
2. **Shell AST 深度语法树检查**：客户端不只做简单的字符串前缀匹配，而是将整条命令解析为 Shell 抽象语法树（AST），递归拆解并检查：
   - 管道（`|`）、逻辑串联（`&&`、`||`、`;`）、子 Shell `(...)`、命令替换 `$(...)`、反引号 `` `...` `` 以及 `if`/`for`/`while`/`case` 控制流内部的子命令；
   - 常见命令包装器：`sudo`、`env`、`timeout`、`nice`、`nohup`、`stdbuf`、`command`、`builtin`、`exec` 与裸 `xargs`；
   - 传给 `sh -c`、`bash -c`、`zsh -c` 或 `eval` 的静态内联脚本。

一旦任意子命令命中黑名单，客户端会直接拒绝执行（不弹确认框），并将拦截记录写回会话（随后随下一次代理请求记入服务端工具审计日志）。

## 2. 内置全屏交互程序（TUI）拦截

即使在真实 PTY 中，进入全屏接管屏幕缓冲区的编辑器或监控器也会破坏多轮自动排障体验。AIST 复用同一套 AST 解析器内置拦截两类全屏程序：

| 拦截类别 | 典型程序 | 提示与替代方案 |
|---|---|---|
| **全屏编辑器** | `vi`、`vim`、`nvim`、`nano`、`emacs`、`micro`、`helix`、`visudo`、`crontab -e` 等 | 拦截并引导模型改用 `file_edit` 工具（支持远端 `target`、`sudo`、彩色 Diff 确认与本地自动备份） |
| **分页器与全屏监控** | `less`、`more`、`htop`、`btop`、`glances`、`iotop`、`iftop`、`ncdu`、`k9s`、`lazygit`、`watch`、未加 `-b` / `-l <N>` 的 `top`、交互式 `tmux`/`screen` | 拦截并引导改用非分页或单次采样参数（如 `--no-pager`、Linux `top -b -n 1`、macOS `top -l 1`） |

## 能力边界说明

> [!WARNING]
> 命令黑名单与全屏拦截定位为**防范模型幻觉与运维误操作的安全护栏**，而非操作系统级沙箱：
> - 动态拼接的变量命令（如 `$CMD`）或外部脚本文件内部的子命令无法在运行前静态穷举，终端人工确认仍是核心防线；
> - 命令在本机客户端执行，请同时在目标服务器上遵循最小权限原则配置 SSH 账号与 `sudoers`。
