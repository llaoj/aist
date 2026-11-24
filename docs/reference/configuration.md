# Configuration & Environment Variables

AIST intentionally keeps its configuration surface minimal — zero client config files (`config.yaml` is not used) and zero required server environment variables.

## Client Environment Variables

| Variable | Required | Default | Purpose |
|---|---|---|---|
| `AIST_SERVER` | No | Stored `server_url` or `https://aistrun.cc` | Overrides the server URL in `aist login`, token loading, and `run.sh` |
| `AIST_RELEASE_BASE_URL` | No | `${AIST_SERVER}/releases/client` | (`run.sh` only) Overrides client archive mirror URL for custom testing |
| `AIST_YES` | No | Unset | (`uninstall.sh` only) Set `AIST_YES=1` to confirm uninstallation in non-interactive shells |
| `LC_ALL` / `LC_MESSAGES` / `LANG` | No | OS locale | Inspected **once** when `~/.aist/preferences.json` does not exist yet to infer initial `zh` or `en` language |

## Server & Docker Compose Environment Variables

| Variable | Scope | Default | Purpose |
|---|---|---|---|
| `AIST_SERVER_ADDR` | `server` process | `:8090` | HTTP listen address inside the server process / container |
| `AIST_SERVER_VERSION` | `deploy/server/compose.yaml` | `latest` | Image tag pulled from `ghcr.io/llaoj/aist-server` |
| `AIST_SERVER_BIND` | `deploy/server/compose.yaml` | `127.0.0.1:8090` | Host `ip:port` mapped to container `:8090` |
| `AIST_SERVER_DATA` | `deploy/server/compose.yaml` | `./data` | Host directory mounted at `/var/lib/aist` |
| Custom `<ENCRYPTION_ENV>` | `server serve --encryptionEnv=<VAR>` | Disabled | Optional 32-character secret for PocketBase AES-256-GCM encryption of `Settings` and `upstream_providers.api_key` |

## Per-User / Invite-Code `settings` JSON Schema

Stored in `users.settings` and `invite_codes.settings` on the server:

```json
{
  "session": {
    "max_sessions": 10,
    "compaction": {
      "keep_tokens": 20000,
      "trigger_ratio": 0.85
    }
  },
  "blacklist": [
    "rm -rf /",
    "mkfs *",
    "shutdown"
  ]
}
```

| Field | Fallback When `<= 0` or Omitted | Description |
|---|---|---|
| `session.max_sessions` | `10` | Number of recent local sessions kept under `~/.aist/sessions/` |
| `session.compaction.keep_tokens` | `20000` | Approximate uncompacted tail token budget kept intact |
| `session.compaction.trigger_ratio` | `0.85` | Fraction of `(context_window - max_output_tokens)` that triggers compaction |
| `blacklist` | `[]` | Glob patterns enforced by both `<command_blacklist>` prompt block and client AST guard |

## Proxy Timeout Defaults (Client → Server)

| Timeout | Value |
|---|---|
| Client overall completion timeout (including retries) | `90s` |
| Server total proxy budget / per-candidate timeout | `85s` / `30s` |
