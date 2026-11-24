# HTTP 接口契约与错误码

## 自定义服务端接口（`/v1/*` 与静态分发）

| 方法 | 路径 | 鉴权 | 成功响应 | 错误状态码 |
|---|---|---|---|---|
| `GET` | `/v1/healthz` | 无 | `200 {"status":"ok","version":"..."}` | — |
| `POST` | `/v1/register` | 无（要求 `application/json`，带限流） | `201 {"email":"...","verified":false,"verification_sent":bool}` | `400`、`415`、`429`、`500` |
| `GET` | `/v1/account/usage` | `Bearer <auth_token>` | `200 {"week_start":"...","reset_at":"...","requests":int,"prompt_tokens":int,"completion_tokens":int,"used_tokens":int,"weekly_token_quota":int,"remaining_tokens"?:int}` | `401`、`500` |
| `POST` | `/v1/proxy/chat/completions` | `Bearer <auth_token>` | 上游 `2xx` OpenAI 兼容响应体 | `400`、`401`、`403`、`429`、`502`、`503` |
| `GET` | `/`、`/register` | 无 | 内嵌落地页与注册页（动态注入 `Settings.Meta.AppURL`） | — |
| `GET` | `/run.sh`、`/uninstall.sh` | 无 | 客户端安装升级脚本与卸载脚本 | — |
| `GET` | `/releases/client/*` | 无 | 同版本客户端压缩包与 `checksums.txt` | `404` |

## PocketBase 原生用户认证接口

| 方法 | 路径 | 鉴权 | 用途 |
|---|---|---|---|
| `GET` | `/api/collections/users/auth-methods` | 无 | 获取服务端启用的密码 / OTP / MFA 登录方式 |
| `POST` | `/api/collections/users/auth-with-password` | 无 | 邮箱密码登录（开启 MFA 时返回 `401 {"mfaId":"..."}`） |
| `POST` | `/api/collections/users/request-otp` | 无 | 发送一次性邮件验证码（`200 {"otpId":"..."}`） |
| `POST` | `/api/collections/users/auth-with-otp` | 无 | 提交验证码（及可选 `mfaId`）完成登录 |
| `POST` | `/api/collections/users/auth-refresh` | `Bearer <auth_token>` | 滑动续期 Token 并获取最新 `meta.config` |
| `POST` | `/api/collections/users/request-verification` | 无 | 重新发送邮箱激活链接（`204`） |
| `POST` | `/api/collections/users/request-password-reset` | 无 | 发送密码重置链接（`204`） |

### `authResponse` 与 `meta.config` 客户端投影

```json
{
  "token": "<jwt>",
  "record": {
    "id": "u1234567890abcd",
    "email": "alice@example.com"
  },
  "meta": {
    "config": {
      "schema_version": 1,
      "limits": {
        "context_window": 128000,
        "max_output_tokens": 8192
      },
      "session": {
        "max_sessions": 10,
        "compaction": {
          "keep_tokens": 20000,
          "trigger_ratio": 0.85
        }
      },
      "blacklist": ["rm -rf /"]
    }
  }
}
```

## 代理接口统一结构化错误码

`POST /v1/proxy/chat/completions` 在返回错误时使用统一 JSON 结构，客户端据此向用户展示清晰的中英文排障提示（将 `retry_at` 自动转换为本机时区），对确定性错误不盲目重试：

```json
{
  "error": {
    "code": "quota_exceeded",
    "message": "weekly token quota exceeded",
    "retry_at": "2026-11-23T00:00:00Z"
  }
}
```

| HTTP 状态码 | `error.code` | 客户端是否重试 | 含义与提示说明 |
|---|---|---|---|
| `400` | `invalid_request` | 否 | 请求格式不合法、超出模型上下文窗口或触发上游安全拦截 |
| `401` | *(PocketBase 鉴权)* | 自动刷新重试一次 | 登录凭证失效或已被轮换吊销 |
| `403` | `no_available_model` | 否 | 账号未分配启用模型，或名下模型均因 `auth` 鉴权错误阻塞 |
| `429` | `quota_exceeded` | 否 | 本周 Token 限额已用完，`retry_at` 为下周一 `00:00 UTC` |
| `503` | `no_available_model` | 否 | 名下可用模型均处于额度或限流冷却期，`retry_at` 为最早预计恢复时间 |
| `502` | `upstream_unavailable` | 否 | 服务端已在预算内依次尝试全部可用候选模型，但上游均返回 `5xx` 或超时 |

## `POST /v1/register` 错误码

```json
{
  "code": "invite_code_exhausted",
  "message": "This invite code has reached its maximum uses."
}
```

可能的 `code` 值：`invalid_content_type`（`415`）、`invalid_json`、`invite_code_required`、`invalid_invite_code`、`invite_code_expired`、`invite_code_exhausted`、`email_already_registered`、`validation_failed`、`registration_failed`。
