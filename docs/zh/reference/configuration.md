# 配置项与环境变量总表

AIST 坚持极简配置面设计：客户端无业务配置文件，服务端无必填环境变量。

## 客户端环境变量

| 变量名 | 是否必填 | 默认值 | 说明 |
|---|---|---|---|
| `AIST_SERVER` | 否 | `token.json` 中已存地址或 `https://aistrun.cc` | 覆盖 `aist login`、运行期请求与 `run.sh` 连接的服务端地址 |
| `AIST_RELEASE_BASE_URL` | 否 | `${AIST_SERVER}/releases/client` | 仅 `run.sh` 使用，用于本地或自定义镜像源测试 |
| `AIST_YES` | 否 | 空 | 仅 `uninstall.sh` 使用，设为 `1` 时在非交互环境免确认卸载 |
| `LC_ALL` / `LC_MESSAGES` / `LANG` | 否 | 系统默认 | 仅在 `~/.aist/preferences.json` 尚未生成时读取一次，用于首次自动推断默认语言（`zh` / `en`） |

## 服务端与 Compose 环境变量

| 变量名 | 作用范围 | 默认值 | 说明 |
|---|---|---|---|
| `AIST_SERVER_ADDR` | `server` 进程 | `:8090` | 服务端 HTTP 监听地址（容器内固定为 `:8090`） |
| `AIST_SERVER_VERSION` | `deploy/server/compose.yaml` | `latest` | `ghcr.io/llaoj/aist-server` 镜像标签 |
| `AIST_SERVER_BIND` | `deploy/server/compose.yaml` | `127.0.0.1:8090` | 宿主机绑定地址与端口 |
| `AIST_SERVER_DATA` | `deploy/server/compose.yaml` | `./data` | 挂载到容器 `/var/lib/aist` 的宿主机持久化目录 |
| 自定义 `<变量名>` | `server serve --encryptionEnv=<变量名>` | 不启用 | 可选 32 字符密钥，开启 PocketBase `Settings` 与 `upstream_providers.api_key` 的 AES-256-GCM 落盘加密 |

## 用户与邀请码 `settings` JSON 字段

存放在服务端 `users.settings` 与 `invite_codes.settings` 中：

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

| 字段 | 省略或 `<= 0` 时的回退默认值 | 说明 |
|---|---|---|
| `session.max_sessions` | `10` | 客户端本地保留的最近会话数量 |
| `session.compaction.keep_tokens` | `20000` | 触发上下文压缩时尾部保留的原始消息 Token 预算 |
| `session.compaction.trigger_ratio` | `0.85` | 当估算上下文达到 `(context_window - max_output_tokens) × trigger_ratio` 时触发压缩 |
| `blacklist` | `[]` | 同时注入系统提示词并在客户端通过 Shell AST 拦截的命令通配规则 |

## 客户端代理请求超时默认值

| 超时项 | 默认值 |
|---|---|
| 单次模型调用总超时 | `10 分钟` |
| 等待响应头超时 | `5 分钟` |
| 响应体读取超时 | `15 分钟` |
