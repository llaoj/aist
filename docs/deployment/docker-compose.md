# Docker Compose Production Deployment

## System Requirements

- Docker Engine + Docker Compose v2
- Single host persistent volume (PocketBase uses embedded SQLite in WAL mode — **always run a single server replica**)
- An HTTPS reverse proxy (Caddy, Nginx, Traefik, or Cloudflare Tunnel)

## Container Layout

Inside `ghcr.io/llaoj/aist-server`:

| Item | Container Path / Default | Notes |
|---|---|---|
| Binary | `/usr/local/bin/server` | Runs as non-root `aist` user via `su-exec` after fixing `/var/lib/aist` ownership |
| Data root | `/var/lib/aist` | Actual SQLite files live in `/var/lib/aist/pb_data` (`data.db`, `auxiliary.db`, `storage/`) |
| Bundled clients | `/usr/share/aist/client` | Served publicly at `/releases/client/` (`latest.txt`, `latest.json`, `<version>/*.tar.gz`, `checksums.txt`) |
| Listen address | `:8090` (`AIST_SERVER_ADDR`) | Bind to `127.0.0.1:8090` on the host when fronted by a local reverse proxy |

## Compose Configuration (`deploy/server/compose.yaml`)

```yaml
services:
  server:
    image: ghcr.io/llaoj/aist-server:${AIST_SERVER_VERSION:-latest}
    restart: unless-stopped
    ports:
      - "${AIST_SERVER_BIND:-127.0.0.1:8090}:8090"
    volumes:
      - ${AIST_SERVER_DATA:-./data}:/var/lib/aist
```

Pin a specific release tag in production:

```bash
AIST_SERVER_VERSION=0.6.0 AIST_SERVER_DATA=/var/lib/aist docker compose up -d
```

### Optional At-Rest Secret Encryption (`--encryptionEnv`)

By default, PocketBase settings and `upstream_providers.api_key` are stored in SQLite (and always masked as `••••••••` in admin API responses). To encrypt both PocketBase settings and provider API keys at rest with AES-256-GCM, pass PocketBase's native `--encryptionEnv` flag and a **32-character** key:

```yaml
services:
  server:
    image: ghcr.io/llaoj/aist-server:0.6.0
    command: ["serve", "--encryptionEnv=PB_ENCRYPTION_KEY"]
    environment:
      - PB_ENCRYPTION_KEY=<32-character-random-secret>
    restart: unless-stopped
    ports:
      - "127.0.0.1:8090:8090"
    volumes:
      - /var/lib/aist:/var/lib/aist
```

## Essential Post-Boot Setup

### 1. Create Superuser

```bash
docker compose exec server su-exec aist server superuser upsert admin@example.com '<password>'
```

### 2. Set Application URL

In `/_/` → **Settings → Application**:
- Set **Application URL** to your external HTTPS origin (e.g. `https://aist.example.com`).
- Landing page install snippets (`/`), `/run.sh`, verification emails, and password reset links all read this URL directly.

### 3. Configure SMTP & Require Verified Emails (Strongly Recommended)

When opening invite-code registration (`/register`), enabling OTP/MFA login, or supporting password resets:

1. Open `/_/` → **Settings → Mail settings**, enable **Use SMTP mail server**, configure your SMTP host/port/credentials, and send a test email.
2. To prevent unverified email addresses from consuming model quota after invite-code registration:
   - Open the **`users`** collection settings (**Edit collection → Options → Password auth** or API rules), or set `verified = true` requirement as appropriate for your workflow.
   - When `SMTP.Enabled` is `true`, `POST /v1/register` automatically sends PocketBase's verification email and instructs new users on Step 2 of `/register` to activate their inbox before signing in.

### 4. Enable Rate Limiting

`aist` automatically seeds a rate-limit rule `POST /v1/register` (`5` requests per `600` seconds per IP) alongside PocketBase's built-in auth rules. Turn on the master switch in `/_/` → **Settings → Rate limits → Enable** in public-facing deployments.
