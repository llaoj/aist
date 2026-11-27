# Interactive, One-Shot & Pipe Modes

`aist` provides three natural entry points depending on how you work in the terminal.

## 1. Interactive Mode

Run `aist` with no piped stdin to enter a persistent multi-turn session:

```bash
aist
```

Or kick off an interactive session with an opening question:

```bash
aist "nginx is returning 502 on api.example.com — check upstream service health"
```

Wait-free autonomous loop within each turn:
1. AIST inspects relevant state by proposing `shell_execute`, `memory_search`, `memory_save`, or `file_edit` tool calls.
2. For `shell_execute` and `file_edit`, you choose one of three options in the confirmation prompt:
   - **Run / Yes**: execute the command or apply the file diff and return the result to the model;
   - **Skip this command / No**: skip this single step and continue remaining steps;
   - **Reject all and let me add details**: abort all remaining confirmation-gated steps in the current batch and immediately return to your input prompt so you can steer the investigation.
3. User-confirmed steps (`Run` or `Skip`) never hit an artificial step wall; AIST only pauses if **15 consecutive unattended tool rounds** (`DefaultMaxAutoToolCalls = 15`, such as silent memory lookups or guard rejections) occur without human confirmation, so you can type `"continue"` or refine instructions.
4. Press `Esc` anytime while the model is thinking to cancel the in-flight request without losing session history; press `Ctrl+C` at any time (while thinking, at the prompt, or in a confirmation menu) to exit.

> [!NOTE]
> Note that `aist "question"` on an interactive terminal enters interactive mode after answering your initial question so you can immediately follow up. Press `Ctrl+C` whenever you are done.

## 2. Pipe Mode

Like `less` or `fzf`, you can pipe command output, logs, stack traces, or config diffs directly into `aist`:

```bash
kubectl describe pod api-7d9f8b-xk2lp -n prod | aist "why is this pod CrashLoopBackOff?"
journalctl -u postgresql --since "30 min ago" | aist
git diff HEAD~1 | aist "spot any risky production config changes"
```

Key characteristics of Pipe Mode:
- **Interactive in a terminal (`/dev/tty`)**: When stdout is a terminal and `/dev/tty` is available, `aist` drains the piped input first and reattaches to `/dev/tty` for a full interactive session — running immediately when a question is provided, or waiting at the prompt for your question when omitted (`cmd | aist`) so no tokens are wasted guessing your intent.
- **Explicit question required when redirected**: When stdout is redirected (e.g. `journalctl -n 200 | aist "summarize the root cause" > report.md`) or no TTY exists (cron/CI), an explicit question argument is required; `aist` then omits tools, prints a clean read-only response once, and exits.
- **Generous input budget**: Reads up to **1.6 MB** from stdin and sends up to **240,000 characters** (preserving head and tail if truncated) alongside your local Memory catalog.

## 3. Command Execution Environment & PTY Support

When you approve a `shell_execute` command in interactive mode:

- **Real interactive PTY**: When attached to a terminal, commands run inside a PTY (`ssh -t` for remote hosts) synchronized with your terminal window size. Interactive prompts such as `sudo` passwords, SSH passphrases, hardware MFA touches, and `[y/N]` confirmations work natively — **never paste passwords or MFA codes into chat**.
- **Non-paging environment**: `aist` injects `PAGER=cat`, `GIT_PAGER=cat`, `SYSTEMD_PAGER=cat`, `MANPAGER=cat` and `EDITOR=false`, `VISUAL=false`, `GIT_EDITOR=false`, `SUDO_EDITOR=false` so `systemctl status` or `git log` never hang waiting on a pager.
- **Timeouts & output bounds**: Commands have a 30-minute execution timeout (5-second post-EOF exit grace) and cap buffered memory to the first 512 KB + last 512 KB (`1 MiB` total), passing up to 8,000 characters (60% head + 40% tail) back to the model.
