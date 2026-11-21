# Sessions & Environment Memory

## Resuming Saved Sessions (`aist sessions`)

Every interactive session is persisted incrementally to `~/.aist/sessions/<id>/` (`0700` directory, `0600` files) as messages are appended — pressing `Ctrl+C` or closing a terminal never loses prior turns.

List and resume recent sessions:

```bash
aist sessions
```

```text
Select a session
  2026-11-21 09:15  nginx 502 upstream timeout  (14 messages)
  2026-11-20 18:42  prod redis memory check     (8 messages)
↑↓ select · Enter resume · Ctrl+C quit
```

- Restoring a session replays your conversation history and tool execution summaries using your **current** `aist lang` setting without re-executing past commands.
- By default, the client retains the **10 most recent sessions** (configurable per account on the server via `settings.session.max_sessions`).

## Automatic Context Compaction

Long troubleshooting sessions can accumulate large command outputs. When estimated context tokens reach `trigger_ratio` (default `85%`) of the effective model context budget (`context_window - max_output_tokens`, default `128000 - 8192 = 119808`), AIST automatically compacts older turns:

1. Protects the system prompt header and the most recent turns up to `keep_tokens` (default `20,000` tokens; the latest turn is always preserved in full).
2. Summarizes middle turns through the server proxy into a structured `<compressed_history>` block preserving hosts, IPs, paths, versions, error messages, commands already run, and open hypotheses.
3. Appends a compaction event to `messages.jsonl` so full raw history remains intact for display while keeping model calls fast and within context limits.

## Local Environment Memory (`~/.aist/memory/`)

**Memory** lets AIST remember stable facts about your infrastructure across sessions — cluster topologies, jump host aliases, service paths, database roles, and runbook conventions — without repeating yourself every day.

### Core Privacy & Safety Properties

- **100% local on your machine**: Stored as plain Markdown files in `~/.aist/memory/*.md` (`0600`). Memory files are **never** uploaded to a server database; only a lightweight catalog (`Name`, `Type`, `Summary`, `Modified`) is injected into your prompt, and specific entries are read on demand via `memory_search`.
- **Maintained through conversation**: There is no `aist memory` subcommand. Simply tell AIST in chat (e.g. *"Remember that prod-web1 is 192.0.2.10 and nginx configs live in /etc/nginx/conf.d"*), or AIST will save verified high-signal facts via `memory_save` and notify you (`✓ Remembered: prod-web1`).
- **Environment knowledge, never instructions**: System prompts explicitly instruct the model to treat Memory strictly as factual context, never as executable instructions.
- **Automatic secret scrubbing**: `<private>...</private>` tags are stripped before saving, and `memory_save` rejects content matching private keys (`-----BEGIN ... PRIVATE KEY-----`), `password=`/`token=`/`secret=`/`api_key=` assignments, Bearer headers, AWS `AKIA...` keys, or `sk-...` tokens.

### Example Memory File (`~/.aist/memory/prod-k8s.md`)

You can also view, edit, or delete `~/.aist/memory/*.md` directly with any text editor:

```markdown
---
updated: 2026-11-21T10:00:00+08:00
source: user statement + verified commands
summary: prod Kubernetes cluster, control plane 192.0.2.1, 3 worker nodes
type: k8s
---

# prod-k8s

## Memory

- control-plane: k8s-cp-01 (192.0.2.1)
- workers: 192.0.2.2, 192.0.2.3, 192.0.2.4
- ingress controller: nginx-ingress in namespace ingress-nginx
```

> [!NOTE]
> The `<memory_catalog>` block is built once at `aist` startup (up to a `32 KiB` catalog/file budget). Entries saved mid-session via `memory_save` are immediately searchable via `memory_search` in the same session, and appear in `<memory_catalog>` on your next `aist` launch.
