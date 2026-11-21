# 5 分钟私有化部署

AIST 服务端是内置 SQLite（PocketBase）的单体静态二进制，约 42 MB 的容器镜像里已经打包好公开站点与全平台同版本客户端安装包——**不需要外部数据库、不需要 Redis、不需要对象存储，也不强制配置任何环境变量**。

## 1. 使用 Docker Compose 启动服务端

新建 `compose.yaml`：

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

启动容器：

```bash
docker compose up -d
```

## 2. 创建超级管理员账号

通过命令行直接创建或更新管理员（也可以使用首次启动日志 `docker compose logs server` 中输出的一次性初始化链接）：

```bash
docker compose exec server su-exec aist server superuser upsert admin@example.com '<strong-password>'
```

浏览器访问 `http://127.0.0.1:8090/_/`（或反代后的 HTTPS 域名）登录管理后台。

## 3. 配置 Application URL 与上游模型

1. **设置对外地址**：进入 **Settings → Application**，将 **Application URL** 修改为服务端真实访问地址（例如 `https://aist.example.com`，本地测试保持 `http://127.0.0.1:8090`）并保存。首页安装命令、`/run.sh` 脚本与验证邮件链接都会自动使用该地址。
2. **添加上游 Provider**（左侧 `upstream_providers` 表）：
   - `name`：如 `deepseek`
   - `base_url`：OpenAI 兼容接口根地址（不带 `/chat/completions`），如 `https://api.deepseek.com/v1`
   - `api_key`：直接粘贴上游 API Key 明文（后台接口回显自动掩码为 `••••••••`）
   - `enabled`：勾选启用
3. **添加上游模型**（左侧 `upstream_models` 表）：
   - `provider`：选择刚创建的 `deepseek`
   - `name`：上游真实模型名，如 `deepseek-chat`
   - `display_name`：展示别名，如 `DeepSeek Chat`
   - `priority`：`10`（数值越小越优先尝试）
   - `context_window`：`128000`
   - `max_output_tokens`：`8192`
   - `enabled`：勾选启用

## 4. 创建用户并一键安装客户端

你可以在 `users` 表中直接新建账号并勾选 `allowed_models`，也可以在 `invite_codes` 表中创建邀请码后通过 `/register` 页面自助注册。

随后在目标机器上直接从你的服务端拉取配套客户端并登录：

```bash
curl -fsSL http://127.0.0.1:8090/run.sh | sh
```

随时检查服务端健康状态与版本：

```bash
curl -fsS http://127.0.0.1:8090/v1/healthz
```

> [!IMPORTANT]
> 面向公网或团队正式投产前，请配置 HTTPS 反向代理、限制 `/_/` 管理后台访问来源，并在开启自助注册或邮箱验证码登录时配置好 SMTP。详见 [Docker Compose 生产部署](../deployment/docker-compose.md) 与 [HTTPS 反向代理与后台防护](../deployment/reverse-proxy.md)。
