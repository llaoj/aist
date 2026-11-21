# 5-Minute Self-Hosting

The AIST server runs as a single stateless binary backed by embedded SQLite (PocketBase) and bundles matching client binaries inside its ~42 MB container image. **No external database, Redis, object storage, or required environment variables are needed.**

## 1. Start the Server with Docker Compose

Create a `compose.yaml` file:

```yaml
services:
  server:
    image: ghcr.io/llaoj/aist-server:latest
    restart: unless-stopped
    ports:
      - "127.0.0.1:8090:8090"
    volumes:
      - ./data:/var/lib/aist
```

Bring it up:

```bash
docker compose up -d
```

## 2. Create the Superuser Account

Create your administrator credentials directly via CLI (or use the one-time installer link printed in `docker compose logs server` on first boot):

```bash
docker compose exec server su-exec aist server superuser upsert admin@example.com '<strong-password>'
```

Open `http://127.0.0.1:8090/_/` (or your reverse-proxied HTTPS URL) and sign in.

## 3. Set Application URL & Configure Upstream Models

1. **Set Application URL**: Go to **Settings → Application**, set **Application URL** to your external server address (for example `https://aist.example.com` or `http://127.0.0.1:8090` for local testing), and click **Save**. Both the landing page install snippet and `/run.sh` automatically inject this URL.
2. **Add an Upstream Provider** (`upstream_providers` collection):
   - `name`: e.g. `deepseek`
   - `base_url`: OpenAI-compatible base URL without `/chat/completions`, e.g. `https://api.deepseek.com/v1`
   - `api_key`: paste your upstream API key in plain text (masked as `••••••••` in API responses)
   - `enabled`: `true`
3. **Add an Upstream Model** (`upstream_models` collection):
   - `provider`: select `deepseek`
   - `name`: upstream model ID, e.g. `deepseek-chat`
   - `display_name`: e.g. `DeepSeek Chat`
   - `priority`: `10` (smaller numbers are tried first)
   - `context_window`: `128000`
   - `max_output_tokens`: `8192`
   - `enabled`: `true`

## 4. Create a User (or Invite Code) & Connect the Client

Either create a user directly in the `users` collection (assigning `allowed_models`), or create an invite code in `invite_codes` and register at `/register`.

Then install and connect the matching `aist` client directly from your server:

```bash
curl -fsSL http://127.0.0.1:8090/run.sh | sh
```

Verify server health anytime:

```bash
curl -fsS http://127.0.0.1:8090/v1/healthz
```

> [!IMPORTANT]
> Before exposing your server to the public internet, put it behind an HTTPS reverse proxy, restrict `/_/` admin access, and configure SMTP if you enable self-registration or OTP login. See [Docker Compose Deployment](../deployment/docker-compose.md) and [HTTPS Reverse Proxy](../deployment/reverse-proxy.md).
