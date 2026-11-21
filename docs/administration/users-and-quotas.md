# Users, Invite Codes & Weekly Token Quotas

## Managing Users (`users` Collection)

`users` is an admin-managed PocketBase auth collection (all public CRUD API rules are locked to `nil`; clients interact solely via auth endpoints and `/v1/*`).

![User allowed models, weekly token quota, and command blacklist settings](/admin-governance.png)

| Field | Type | Description |
|---|---|---|
| `email` | email (unique) | Login identity |
| `password` | password | Bcrypt hash (8–71 bytes) |
| `verified` | bool | Email verification status |
| `name` | text(255) | Optional display name |
| `allowed_models` | relation → `upstream_models` | Models this user is permitted to route through |
| `weekly_token_quota` | int (`>= 0`) | Maximum total tokens per UTC week (resets **Monday `00:00 UTC`**); `0` means unlimited |
| `settings` | JSON (`1 MiB`) | Per-user `session` and `blacklist` policy |

Example `users.settings`:

```json
{
  "session": {
    "max_sessions": 10,
    "compaction": {
      "keep_tokens": 20000,
      "trigger_ratio": 0.85
    }
  },
  "blacklist": ["rm -rf /", "mkfs *", "shutdown", "reboot"]
}
```

### Revoking User Sessions

PocketBase auth tokens are stateless JWTs signed with `users.tokenKey + collectionSecret` (default 7-day duration):
- **Revoke a single user**: Change the user's password or email in `/_/` (rotates that user's `tokenKey`), or delete the user record.
- **Revoke all users**: In `/_/` → `users` → **Edit collection → Options → Token options**, click **Invalidate all previously issued tokens**.

## Invite Codes & New-User Templates (`invite_codes`)

A single `invite_codes` collection controls both **registration admission** and **initial account provisioning** when users sign up at `/register` (`POST /v1/register`):

| Field | Type | Description |
|---|---|---|
| `code` | text(4..64), unique | Invite code string (leave blank in Admin UI to auto-generate an 8-character uppercase alphanumeric code, or set a custom code like `TEAM-SRE-2026`) |
| `enabled` | bool | Toggle code availability anytime |
| `max_uses` | int (`>= 0`) | Total redemptions allowed (`0` = unlimited) |
| `used_count` | int (`>= 0`) | Atomically incremented inside the registration transaction |
| `expires_at` | date (UTC) | Optional expiration timestamp (empty = never expires) |
| `allowed_models` | relation → `upstream_models` | Copied onto newly registered `users.allowed_models` |
| `weekly_token_quota` | int (`>= 0`) | Copied onto newly registered `users.weekly_token_quota` |
| `settings` | JSON | Copied onto newly registered `users.settings` |
| `note` | text(255) | Admin memo (e.g. campaign or team name) |

Share prefilled registration URLs directly with teammates or testers:

```text
https://aist.example.com/register?code=TEAM-SRE-2026
```

## Daily Usage Metering & Weekly Quota Enforcement

Every successful `2xx` upstream completion extracts token counts from `usage` (falling back to `prompt_tokens + completion_tokens` if `total_tokens` is omitted) and atomically upserts `daily_model_usage`:

| Field | Description |
|---|---|
| `user` | Relation to `users` (cascade delete) |
| `provider` / `model` | Plain-text snapshots so deleting or renaming an upstream model never corrupts historical billing or active weekly quota totals |
| `date` | UTC date (`YYYY-MM-DD`) |
| `requests` | Successful completion count |
| `prompt_tokens` / `completion_tokens` / `total_tokens` | Cumulative token counts |

Before forwarding a proxy request, if `user.weekly_token_quota > 0`, the server sums `total_tokens` in `daily_model_usage` for that user from **Monday `00:00:00 UTC`** of the current week through today. Once reached, requests return HTTP `429` (`code: "quota_exceeded"`) with `retry_at` set to next Monday `00:00:00 UTC` (displayed in the user's local terminal timezone).
