# HTTPS 反向代理与后台防护

AIST 服务端不在应用层重复造 IP 白名单。生产环境中建议通过 Caddy、Nginx 或 Cloudflare 终止 TLS，并对管理后台接口做来源限制，同时确保客户端认证与代理接口正常可达。

## 必须对客户端保持开放的路由

| 路由路径 | 用途 |
|---|---|
| `GET /`、`/register`、`/logo.svg`、`/css/*`、`/js/*` | 落地页与邀请码自助注册页 |
| `GET /run.sh`、`/uninstall.sh`、`/releases/client/*` | 客户端一键安装/升级脚本及同版本二进制压缩包 |
| `GET /v1/healthz` | 存活与版本健康检查 |
| `POST /v1/register` | 邀请码自助注册接口 |
| `POST /v1/proxy/chat/completions` | LLM 模型代理转发（需放宽反代读超时至 `15m`） |
| `/api/collections/users/auth-methods` | 登录方式探测 |
| `/api/collections/users/auth-with-password` | 邮箱密码登录 |
| `/api/collections/users/request-otp`、`/auth-with-otp` | 邮件验证码 / MFA 登录 |
| `/api/collections/users/auth-refresh` | Token 刷新与账号配置拉取 |
| `/api/collections/users/request-verification`、`/confirm-verification` | 邮箱激活验证 |
| `/api/collections/users/request-password-reset`、`/confirm-password-reset` | 密码重置 |

> [!NOTE]
> PocketBase 内置的邮箱激活与密码重置确认页（`/_/#/auth/confirm-verification/...` 与 `/_/#/auth/confirm-password-reset/...`）挂载在 `/_/` 前端路由下。若需要面向公网用户开放邮件激活/改密，同时保护管理员接口，可针对 `/api/collections/_superusers/*`、`/api/settings*`、`/api/logs*`、`/api/backups*` 限制管理网段 IP。

## Caddy 配置示例（`Caddyfile`）

```txt
aist.example.com {
    # 可选：仅允许办公网 / 跳板机 IP 访问超级管理员后台 API
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

## Nginx 配置示例

请务必将 `proxy_read_timeout` 调大到 `900s`（15 分钟），防止长思考推理模型在响应完成前被 Nginx 默认 60 秒超时断开：

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

接入反向代理或 Cloudflare 后，请同步在 `/_/` → **Settings → Application** 中配置 **Trusted proxy headers**（如 `X-Real-IP` 或 `CF-Connecting-IP`），以便限流规则与工具审计日志中的 `userIP` 记录真实客户端 IP，而不是 `127.0.0.1`。
