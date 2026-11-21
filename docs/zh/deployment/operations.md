# 健康检查、备份升级与 Fly.io 部署

## 健康检查与版本查看

服务端提供免鉴权的存活与版本探针接口（容器内置 `HEALTHCHECK` 与负载均衡器可直接调用）：

```bash
curl -fsS https://aist.example.com/v1/healthz
# {"status":"ok","version":"v0.6.0"}
```

在宿主机直接查看运行中容器的服务端版本：

```bash
docker compose exec server server version
```

## 数据备份

服务端全部状态均位于 `/var/lib/aist/pb_data`（包含主库 `data.db`、日志库 `auxiliary.db` 及 `storage/`）。

推荐以下两种备份方式：

1. **PocketBase 后台在线热备（推荐）**：
   - 登录 `/_/` → **Settings → Backups**；
   - 点击 **Create backup** 立即生成一致性 `.zip` 快照，或开启定时 Cron 自动备份（支持直接同步至任意 S3 兼容对象存储）。
2. **宿主机数据卷冷备**：
   - 短暂停止容器（`docker compose stop server`）后打包备份宿主机挂载目录。

> [!WARNING]
> 若启动时启用了 `--encryptionEnv`，请务必在独立的密钥管理系统中妥善备份该 32 位密钥；更换或丢失密钥后将无法解密备份中的邮件配置与上游 API Key。

## 平滑升级与回滚

### 升级步骤

1. 先在后台 **Settings → Backups** 创建一次快照备份；
2. 更新镜像版本号并重建容器：
   ```bash
   AIST_SERVER_VERSION=0.6.0 docker compose up -d
   ```
3. 服务端启动时会在事务内自动执行增量迁移；
4. 客户端用户下次执行 `curl -fsSL https://<服务端地址>/run.sh | sh` 时会自动静默升级至配套版本。

### 版本回滚

停止新容器，将升级前的备份恢复至 `/var/lib/aist/pb_data`，再切回旧版镜像 Tag 启动即可。

## 在 Fly.io 部署（`deploy/server/fly.toml`）

仓库内置 `deploy/server/fly.toml`，无需本地安装 Docker 即可通过单实例 + 单持久化卷部署：

```bash
fly apps create my-aist-server
fly volumes create aist_data --app my-aist-server --region sin --size 1
fly deploy --config deploy/server/fly.toml --app my-aist-server

fly ssh console --app my-aist-server -C \
  "su-exec aist server superuser upsert admin@example.com '<password>'"
```

Fly.io 运维要点：
- **严格保持单 Machine 实例**：PocketBase 基于单机 SQLite，请勿水平扩容为多副本；
- **空闲自动休眠与唤醒**：默认开启空闲自动停止，请求到达时由 Fly Proxy 自动唤醒（冷启动约 2 秒；内存 ≤ 2 GB 且未开启 swap 时可将 `auto_stop_machines` 改为 `"suspend"` 获得亚秒级恢复）。

## 上线检查清单

- [ ] `GET /v1/healthz` 返回 `200` 且版本号与发布版本一致
- [ ] 后台 `Settings → Application → Application URL` 已设为对外 HTTPS 域名
- [ ] 后台 `Settings → Application` 已配置反向代理对应的 **Trusted proxy headers**
- [ ] 后台 `Settings → Rate limits` 已开启限流开关
- [ ] 后台 `Settings → Mail settings` 已测试通过 SMTP 发信（启用注册/OTP/找回密码时）
- [ ] 管理后台 `/_/` 或管理员 API 已做好访问来源限制
- [ ] 在干净机器执行 `curl -fsSL https://<服务端地址>/run.sh | sh` 完成安装、登录与首轮问答验证
