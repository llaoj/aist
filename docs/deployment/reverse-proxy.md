# HTTPS Reverse Proxy & Admin Access Hardening

AIST does not implement an application-layer IP allowlist. In production, terminate TLS at a reverse proxy and restrict `/_/` (the PocketBase superuser console) to your corporate VPN, bastion IP, or Cloudflare Zero Trust policy while keeping client authentication and proxy routes reachable.

## Routes That Must Remain Reachable to Clients

| Route Pattern | Purpose |
|---|---|
| `GET /`, `/register`, `/logo.svg`, `/css/*`, `/js/*` | Public landing & invite-code registration pages |
| `GET /run.sh`, `/uninstall.sh`, `/releases/client/*` | Client bootstrap script and version-matched binary archives |
| `GET /v1/healthz` | Liveness / version health check |
| `POST /v1/register` | Invite-code registration API |
| `POST /v1/proxy/chat/completions` | Authenticated LLM proxy (allow long read timeouts up to `15m`) |
| `/api/collections/users/auth-methods` | Login method discovery |
| `/api/collections/users/auth-with-password` | Password sign-in |
| `/api/collections/users/request-otp`, `/auth-with-otp` | OTP / MFA sign-in |
| `/api/collections/users/auth-refresh` | Token refresh & account config sync |
| `/api/collections/users/request-verification`, `/confirm-verification` | Email verification |
| `/api/collections/users/request-password-reset`, `/confirm-password-reset` | Password reset |

> [!NOTE]
> If you use PocketBase's built-in email verification or password reset links (`/_/#/auth/confirm-verification/...` and `/_/#/auth/confirm-password-reset/...`), note that those confirmation SPA views live under `/_/`. If you restrict `/_/` by IP, either restrict `/api/collections/_superusers/*` instead or keep `/_/` protected via Cloudflare Access / VPN according to who registers.

## Caddy Example (`Caddyfile`)

```txt
aist.example.com {
    # Optional: restrict superuser API to internal admin CIDRs
    @superuser_api path /api/collections/_superusers/* /api/settings* /api/logs* /api/backups*
    handle @superuser_api {
        @trusted remote_ip 198.51.100.0/24 203.0.113.10
        handle @trusted {
            reverse_proxy 127.0.0.1:8090
        }
        respond "Forbidden" 403
    }

    reverse_proxy 127.0.0.1:8090 {
        transport http {
            read_timeout 15m
        }
    }
}
```

## Nginx Example

Pay special attention to `proxy_read_timeout` (`900s`) so long reasoning models do not get cut off by Nginx's default 60-second timeout, and configure `real_ip_header` / Trusted Proxy headers in PocketBase (`Settings → Application → Trustedessing proxy headers`) if rate-limiting by client IP:

```nginx
server {
    listen 443 ssl http2;
    server_name aist.example.com;

    ssl_certificate     /etc/ssl/certs/aist.example.com.pem;
    ssl_certificate_key /etc/ssl/private/aist.example.com.key;

    client_max_body_size 32m;

    location / {
        proxy_pass http://127.0.0.1:8090;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 900s;
        proxy_send_timeout 900s;
    }
}
```

When running behind Nginx or Cloudflare, configure **Trusted proxy headers** (e.g. `X-Real-IP` or `CF-Connecting-IP`) in `/_/` → **Settings → Application** so PocketBase rate limits and `userIP` audit logs record real client IPs instead of `127.0.0.1`.
