# CLI Commands & Local State (`~/.aist`)

## Client Commands (`aist`)

| Command | Requires Login? | Description |
|---|---|---|
| `aist` | Yes | Start interactive troubleshooting session |
| `aist "<question>"` | Yes | Start session with an initial question (or analyze stdin when piped) |
| `<cmd> \| aist ["<question>"]` | Yes | Read-only pipe analysis mode (exits after printing findings) |
| `aist sessions` | Yes | List recent local sessions and resume the selected conversation |
| `aist login [server] [-e email]` | No | Authenticate against `[server]` and store credentials in `~/.aist/token.json` |
| `aist logout` | No | Remove `~/.aist/token.json` (preserves `preferences.json`, `sessions/`, `memory/`, and `fileedit/`) |
| `aist whoami` | No | Display stored login email, server URL, and weekly token usage/quota (gracefully degrades when offline) |
| `aist lang [zh\|en]` | No | Show or switch local display and AI reply language (`~/.aist/preferences.json`) |
| `aist version` | No | Print version, commit hash, and build date (does not touch `~/.aist`) |
| `aist completion <shell>` | No | Generate shell completion scripts (`bash`, `zsh`, `fish`, `powershell`) |

## Server Commands (`server`)

| Command | Description |
|---|---|
| `server serve [--encryptionEnv=<VAR>] [--dev]` | Start PocketBase + AIST HTTP server (default data dir `./pb_data`, `/var/lib/aist/pb_data` in Docker) |
| `server superuser upsert <email> <password>` | Create or update a PocketBase superuser account |
| `server version` | Print server build version without opening `pb_data` |

## Local State Directory Layout (`~/.aist`)

All writable client state lives under `~/.aist` (`0700` directories, `0600` files):

```text
~/.aist/
├── fileedit/
│   └── backups/<mirrored-path>/
│       ├── meta.json
│       └── <UTC-timestamp>-<sha256-prefix8>.bak
├── sessions/
│   └── ses_<timestamp>-<hex8>/
│       ├── session.json
│       └── messages.jsonl
├── memory/
│   └── <name>.md
├── preferences.json
└── token.json
```

Remote SSH file backups are stored under `~/.aist/fileedit/backups/@remote/<target>/<mirrored-path>/`.

## Persistence & Concurrency Semantics

- **Atomic replacement (`storage.WriteFile` / `ReplaceFile`)**: `preferences.json`, `session.json`, `memory/*.md`, and `fileedit` backups are written to a temporary sibling file, `fsync`ed, and atomically renamed over the target path.
- **Append-only session log**: `messages.jsonl` uses `O_APPEND` + `Sync()` per message; a truncated trailing line caused by power loss is ignored on recovery.
- **Single-writer contract**: `aist` is an interactive CLI without cross-process file locks — avoid writing to the exact same session or target file from two concurrent `aist` terminals (`file_edit` uses SHA-256 CAS to detect and abort on concurrent content modifications).

## Built-in Runtime Limits

| Limit | Default |
|---|---|
| Pipe stdin read ceiling | `1.6 MB` (`240,000` chars sent to model) |
| Per-turn user message / tool result | `8,000` chars (60% head + 40% tail) |
| Buffered shell stdout/stderr in memory | `1 MiB` (first `512 KB` + last `512 KB`) |
| Shell execution timeout | `30m` (`5s` post-EOF exit grace) |
| Max follow-up tool rounds per turn | `25` |
| `file_edit` max file / new content size | `1 MiB` |
| `file_edit` backup retention per file | `10` deduplicated versions |
| Local Memory budget (`~/.aist/memory/`) | `32 KiB` total / `8 KiB` per `memory_save` |
| Retained sessions (`max_sessions`) | `10` (server-configurable per account) |
