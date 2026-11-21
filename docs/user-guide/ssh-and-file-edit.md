# Multi-Host SSH & Safe File Editing

## Multi-Host SSH Troubleshooting (`target`)

Both `shell_execute` and `file_edit` accept a `target` parameter so AIST can inspect and repair remote Linux/macOS servers from your workstation or bastion host without installing `aist` on every target node.

```text
User: Check why redis on cache-01 (192.0.2.21) is evicting keys, and compare its maxmemory config with cache-02 (192.0.2.22).
```

### How Targets Work Safely

- **Local vs Remote**: Empty `target`, `local`, or `localhost` runs locally (`sh -c`); any other valid host/alias (e.g. `web1`, `ubuntu@192.0.2.10`, `[2001:db8::1]`) executes via your system `ssh` binary (`ssh -t <target> <command>` when a TTY is present).
- **Zero credential scraping**: AIST **never** scans `~/.ssh/config`, `~/.ssh/known_hosts`, `/etc/hosts`, or inventory files to guess hosts. Targets come strictly from your prompt, saved [Environment Memory](./sessions-and-memory.md), or live command outputs you ran (e.g. `kubectl get nodes`).
- **Strict target validation**: Rejects leading `-` (preventing SSH option injection), `..`, whitespace, control characters, and lengths over 255 bytes.
- **Reuses your native SSH setup**: `ProxyJump`, SSH agent forwarding, hardware keys, and `~/.ssh/config` host aliases work out of the box. If SSH returns exit code `255`, AIST reports `SSH connection failed; verify access with ssh <target>` and stops rather than guessing credentials.

## Safe File Editing (`file_edit`)

Instead of fragile `sed -i` / `cat <<EOF` shell one-liners or full-screen editors (which are blocked in `shell_execute`), AIST uses the structured `file_edit` tool for creating and modifying regular text files locally and over SSH.

### End-to-End Safety Workflow

1. **Read & Diff Preview**:
   - Reads the target file (locally, over non-PTY SSH pipe for world/user-readable remote files, or over interactive PTY `sudo` when elevated privileges are needed).
   - Verifies `old_content` matches **exactly once** in the file (or is empty when creating a new file) and generates a colorized unified diff.
2. **Explicit Confirmation**:
   - Displays the `reason`, `[target] /path/to/file (sudo)`, colorized diff, and a state-modification warning before touching disk.
3. **SHA-256 CAS Check, Local Backup & Atomic Write**:
   - Computes the SHA-256 hash of the current file right before writing (`Compare-And-Swap`): if another process or editor changed the file while you were reviewing the diff, `file_edit` aborts instead of overwriting concurrent edits.
   - Backs up the original file locally under `~/.aist/fileedit/backups/<mirrored-path>/` (remote files are mirrored under `~/.aist/fileedit/backups/@remote/<host>/...`), keeping up to **10 deduplicated versions** (`0600`) alongside `meta.json`.
   - Writes through a temporary file, `fsync`s, preserves existing permissions/ownership (or `0600` for newly created files), and atomically renames into place.

### Built-in File Guardrails

| Guardrail | Policy |
|---|---|
| Path format | Must be absolute (`/etc/nginx/nginx.conf`) or `~/` |
| Pseudo-filesystems | Rejects `/proc/`, `/sys/`, `/dev/` |
| File types & size | Regular files only (rejects directories, symlinks, devices); max `1 MiB` |
| Private keys | Blocks private key paths (`id_rsa`, `*.pem`, `*.key`, `ssh_host_*_key`, `*private_key*`) and `-----BEGIN ... PRIVATE KEY-----` content |
| Privilege escalation | Automatically enables `sudo` when modifying a readable file whose parent/inode requires root, or pass `sudo=true` explicitly for root-only `0600` files |

### Restoring from a Local Backup

Inspect `~/.aist/fileedit/backups/<mirrored-path>/meta.json` (or `@remote/<host>/<path>/meta.json`) to locate the timestamped `.bak` snapshot and copy it back to `original_path` with its recorded `mode`.
