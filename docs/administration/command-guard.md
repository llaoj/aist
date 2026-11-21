# Command Blacklist & Full-Screen TUI Guard

AIST enforces two layers of deterministic command safety (`internal/commandguard`) **before** showing any execution confirmation prompt, plus system-prompt injection so the model avoids proposing forbidden commands in the first place.

## 1. Account Command Blacklist (`settings.blacklist`)

Administrators define glob-style blacklist rules in `users.settings.blacklist` (or on `invite_codes.settings.blacklist` for newly registered users):

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

### Dual Enforcement: Prompt + Shell AST

1. **System Prompt Injection**: Up to `4 KiB` of deduplicated blacklist rules are injected into `<command_blacklist>` in the system prompt alongside explicit anti-bypass rules.
2. **Shell AST Runtime Inspection**: Every `shell_execute` command is parsed into a POSIX/Bash syntax tree (`mvdan.cc/sh/v3/syntax`) rather than naive prefix matching. The guard recursively inspects:
   - Pipelines (`|`), logical chains (`&&`, `||`, `;`), subshells `(...)`, command substitutions `$(...)` and backticks `` `...` ``, and loops/conditionals (`if`, `for`, `while`, `case`);
   - Wrapper binaries: `sudo`, `env`, `timeout`, `nice`, `nohup`, `stdbuf`, `command`, `builtin`, `exec`, and bare `xargs`;
   - Static inline scripts passed to `sh -c`, `bash -c`, `zsh -c`, or `eval`.

When a subcommand matches a blacklist pattern, execution is rejected immediately without prompting (`✗ Command rejected: This command matches blacklist rule '...'`) and recorded as `Status: skipped` in the tool audit trail.

## 2. Built-in Full-Screen TUI Guard

Even inside an interactive PTY, full-screen terminal UIs trap cursor addressing and break the request/response troubleshooting loop. Using the same shell AST walker, AIST automatically intercepts:

| Category | Blocked Programs | Guided Alternative Returned to User & Model |
|---|---|---|
| **Full-Screen Editors** | `vi`, `vim`, `nvim`, `view`, `vimdiff`, `nano`, `pico`, `emacs`, `micro`, `helix`, `hx`, `kak`, `joe`, `mcedit`, `ed`, `ex`, `visudo`, `vipw`, `vigr`, `crontab -e`, `git commit`/`rebase -i` without non-interactive flags | Use the structured `file_edit` tool (supports remote `target` and `sudo` with diff preview & backups) |
| **Pagers & Interactive Monitors** | `less`, `more`, `most`, `pg`, `htop`, `btop`, `atop`, `glances`, `nmon`, `iotop`, `iftop`, `nethogs`, `ncdu`, `tig`, `lazygit`, `lazydocker`, `k9s`, `mc`, `ranger`, `yazi`, `nnn`, `lf`, `watch`, `top` (without `-b` or `-l <N>`), interactive `tmux` / `screen` | Re-run with non-paging / batch flags (`--no-pager`, `top -b -n 1` on Linux, `top -l 1` on macOS) |

## Scope & Limitations

> [!WARNING]
> AST inspection is a **cooperative safety guardrail against accidental or hallucinated destructive commands**, not a kernel sandbox:
> - Dynamic command names (`$CMD`), dynamically constructed `eval`/`sh -c` strings, or scripts written inside external files cannot be statically resolved by the parser — human confirmation at the terminal remains your primary authorization boundary.
> - Because `aist` executes on the client machine, a user who modifies their own client binary could bypass client-side checks. Enforce OS/SSH least-privilege (`sudoers`, bastion RBAC) on target hosts accordingly.
