# Docker Compose 生产部署

## 运行环境要求

- Docker 与 Docker Compose v2
- 单机持久化存储卷（PocketBase 使用内置 SQLite，**请始终保持单实例运行，切勿多副本挂载同一数据盘**）
- 前置 HTTPS 反向代理（推荐 Caddy 或 Nginx）

## 容器内部布局

`ghcr.io/llaoj/aist-server` 镜像约定：

| 项目 | 容器内固定路径 / 默认值 | 说明 |
|---|---|---|
| 服务端程序 | `/usr/local/bin/server` | 启动入口自动修正数据目录属主后，通过 `su-exec` 降权为非 root 用户 `aist` 运行 |
| 数据根目录 | `/var/lib/aist` | SQLite 数据实际位于 `/var/lib/aist/pb_data`（含 `data.db`、`auxiliary.db`、`storage/`） |
| 内置客户端包 | `/usr/share/aist/client` | 通过 `/releases/client/` 对外提供同版本 7 平台客户端压缩包与 `checksums.txt` |
| 监听端口 | `:8090`（`AIST_SERVER_ADDR`） | 宿主机建议绑定在 `127.0.0.1:8090`，由本机反向代理统一对外暴露 HTTPS |

## 标准 Compose 配置（`deploy/server/compose.yaml`）

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

生产环境建议固定具体镜像版本号：

```bash
AIST_SERVER_VERSION=0.6.0 AIST_SERVER_DATA=/var/lib/aist docker compose up -d
```

### 可选：开启数据库密钥落盘加密（`--encryptionEnv`）

默认情况下，无需配置任何密钥即可启动，上游 Provider 的 `api_key` 在后台接口中始终掩码为 `••••••••`。若合规要求对落盘的 PocketBase Settings 及 `upstream_providers.api_key` 进行 AES-256-GCM 加密，可按 PocketBase 原生方式传入 `--encryptionEnv` 并提供 **32 字符**密钥：

```yaml
services:
  server:
    image: ghcr.io/llaoj/aist-server:0.6.0
    command: ["serve", "--encryptionEnv=PB_ENCRYPTION_KEY"]
    environment:
      - PB_ENCRYPTION_KEY=<32位随机密钥>
    restart: unless-stopped
    ports:
      - "127.0.0.1:8090:8090"
    volumes:
      - /var/lib/aist:/var/lib/aist
```

## 首次启动必配清单

### 1. 创建超级管理员

```bash
docker compose exec server su-exec aist server superuser upsert admin@example.com '<password>'
```

### 2. 设置 Application URL

登录 `/_/` → **Settings → Application**：
- 将 **Application URL** 设置为服务端真实的对外 HTTPS 地址（如 `https://aist.example.com`）并保存；
- 首页安装命令、`/run.sh` 脚本内注入的服务端地址、邮箱验证链接及密码重置链接均直接读取该配置。

### 3. 配置 SMTP 邮件与邮箱验证（强烈建议）

当你需要启用邀请码自助注册（`/register`）、邮箱验证码登录（OTP/MFA）或忘记密码重置时：

1. 进入 `/_/` → **Settings → Mail settings**，勾选 **Use SMTP mail server**，填好发信服务器与凭据并发送测试邮件验证连通；
2. 若希望防止未验证邮箱占用模型额度，强烈建议配合要求已验证账号（`verified = true`）：开启 SMTP 后，`POST /v1/register` 会在创建账号后自动发送 PocketBase 原生激活邮件，并在注册成功页引导用户先激活邮箱再登录。

### 4. 开启内置接口限流开关

系统迁移已自动预置 `POST /v1/register` 限流规则（单 IP 每 600 秒最多 5 次）以及 PocketBase 默认认证限流规则。面向公网部署时，请在 `/_/` → **Settings → Rate limits** 中勾选启用总开关并保存。
