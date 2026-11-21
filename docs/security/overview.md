# Security Architecture

AIST separates **secret custody and policy governance** (on the server) from **interactive host execution and local environment context** (on the client).

## End-to-End Data Flow & Secret Boundaries

| Asset / Data | Stored Where | Sent to AIST Server? | Sent to Upstream LLM? |
|---|---|---|---|
| Upstream Provider API Keys (`api_key`) | Server SQLite (`upstream_providers`) | Owned by server (masked `••••••••` in Admin UI) | Sent in `Authorization` header to that provider only |
| User Login Token | Client `~/.aist/token.json` (`0600`) | Sent as `Authorization: Bearer <jwt>` | **Never** |
| Client Language Preference | Client `~/.aist/preferences.json` (`0600`) | **Never** | Appended as reply-language instruction in prompt |
| Local Environment Memory | Client `~/.aist/memory/*.md` (`0600`) | Only `<memory_catalog>` summary & `memory_search` results inside active prompt messages | Included in active prompt messages (never persisted in server DB) |
| File Edit Backups | Client `~/.aist/fileedit/backups/` (`0600`) | **Never** | **Never** |
| Conversation Messages & Truncated Tool Outputs | Client `~/.aist/sessions/<id>/` (`0600`) | Transitively proxied via `/v1/proxy/chat/completions` (`shell_execute`/`file_edit` recorded in `_logs` up to `MaxDays`) | Forwarded to selected candidate model |
| Interactive Terminal Secrets (`sudo` / SSH / MFA prompts) | Entered directly into local/SSH PTY | **Never** (unless echoed by command stdout) | **Never** |

## Server-Side Security Controls

- **Zero Client Key Exposure**: `OnRecordAuthRequest("users")` strips `allowed_models`, `weekly_token_quota`, and raw `settings` from user records, projecting only non-secret runtime limits and blacklist rules into `meta.config`.
- **Optional AES-256-GCM Encryption at Rest**: Passing `--encryptionEnv=<ENV_VAR>` encrypts both PocketBase `Settings` (SMTP credentials, S3 backup keys) and `upstream_providers.api_key` using PocketBase's native `security.Encrypt` / `security.Decrypt`. Even without `--encryptionEnv`, `provider_hook.go` masks `api_key` on all API reads so keys cannot be scraped from browser DevTools.
- **Locked Collection Rules**: `users`, `upstream_providers`, `upstream_models`, `daily_model_usage`, and `invite_codes` all enforce `nil` (superuser-only) collection rules.
- **HTTP Hardening**: Built-in `Content-Security-Policy`, `X-Content-Type-Options: nosniff`, `X-Frame-Options: SAMEORIGIN`, `Referrer-Policy: same-origin`, request body size limits, and registration rate limiting (`POST /v1/register`).

## Client-Side Security Controls

- **Strict Filesystem Permissions**: `state.Paths.Ensure()` enforces `0700` on `~/.aist` and all subdirectories (`sessions`, `memory`, `fileedit`, `backups`), and `0600` on all state files. Startup aborts rather than running with degraded directory permissions.
- **Human Confirmation Gate**: Every `shell_execute` and `file_edit` call requires explicit terminal confirmation, with a second warning prompt for `modify` commands and file writes.
- **Private Key & Secret Scrubbing**:
  - `file_edit` refuses private key filenames (`id_rsa`, `*.pem`, `*.key`, `ssh_host_*_key`, `*private_key*`), pseudo-filesystems (`/proc`, `/sys`, `/dev`), symlinks, and PEM private key headers.
  - `memory_save` strips `<private>...</private>` blocks and rejects private keys, password/token assignments, Bearer tokens, `AKIA...`, and `sk-...` strings.
- **Checksum-Verified Upgrades**: `run.sh` fails closed whenever `checksums.txt` cannot be fetched or SHA-256 verification mismatches.

## Reporting Vulnerabilities

Please **do not** open public GitHub issues for security vulnerabilities. Report privately via GitHub Security Advisories or email `security@aistrun.cc` (initial acknowledgment within 3 business days).
