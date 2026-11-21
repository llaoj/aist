# Login & Account Registration

## Self-Registration with an Invite Code

When an administrator issues an invite code, users can create an account at:

```text
https://<server>/register
https://<server>/register?code=<INVITE_CODE>
```

1. Submit your invite code, email address, and password (8–71 characters).
2. Your account automatically inherits the invite code's configured `allowed_models`, `weekly_token_quota`, and `settings` (blacklist and session policy).
3. If the server has SMTP configured, PocketBase sends a verification email — click the link in that email (or use **Resend verification email** on Step 2 of `/register`) if the server requires verified accounts.
4. Forgot your password? Visit `https://<server>/register#reset` to request a password reset link.

## Signing In from the CLI (`aist login`)

```bash
aist login                              # uses remembered server or https://aistrun.cc
aist login https://aist.example.com     # specify server URL
aist login -e alice@example.com 127.0.0.1:8090
```

### Server Address Resolution Order

1. Positional `[server]` argument passed to `aist login`
2. `AIST_SERVER` environment variable
3. Previously remembered `server_url` in `~/.aist/token.json`
4. Default `https://aistrun.cc`

Address normalization:
- Bare hosts default to `https://` (`aist.example.com` → `https://aist.example.com`).
- Loopback hosts (`localhost`, `127.*`, `[::1]`) default to `http://` (`127.0.0.1:8090` → `http://127.0.0.1:8090`).

> [!TIP]
> Before prompting for credentials, `aist login` records the target `server_url` if no active login exists yet. If you cancel `run.sh`'s initial login prompt with `Ctrl+C`, running `aist login` later remembers your server automatically.

### Supported Server Auth Modes

`aist login` queries `GET /api/collections/users/auth-methods` and adapts automatically to the server's `users` collection settings:

| Server Configuration | CLI Login Flow |
|---|---|
| **Password auth** | Prompts for hidden password → signs in |
| **OTP-only auth** | Sends one-time code to email → prompts for verification code |
| **Password + MFA (OTP)** | Prompts for password → sends one-time code → completes 2FA |

Passwords are always read interactively with echo disabled — there is intentionally no `--password` flag or password environment variable so secrets never leak into shell history or `ps` output.

## Inspecting & Clearing Credentials

```bash
aist whoami   # display current email and server URL locally (offline, no network call)
aist logout   # delete ~/.aist/token.json (preserves ~/.aist/preferences.json, sessions, and Memory)
```

### Token Refresh Lifecycle

- Credentials live in `~/.aist/token.json` (`0600`) as a PocketBase JWT (7-day default sliding duration).
- On every `aist` session startup, the client calls `POST /api/collections/users/auth-refresh` to refresh the token and pull the latest account configuration snapshot (`limits`, `session`, `blacklist`) into memory.
- During long interactive sessions, tokens within 60 seconds of expiry are refreshed automatically before proxy calls, and a `401` proxy response triggers one automatic refresh-and-retry.
