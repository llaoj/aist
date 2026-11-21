# 用户管理、邀请码与周 Token 限额

## 用户账号管理（`users` 表）

`users` 表仅允许超级管理员在 `/_/` 后台直接读写（所有公开 CRUD API 规则均为关闭状态，客户端仅通过认证接口与 `/v1/*` 交互）。

![用户可用模型、周 Token 限额与命令黑名单配置](/admin-governance.png)

| 字段 | 类型 | 说明 |
|---|---|---|
| `email` | email（唯一） | 登录邮箱身份 |
| `password` | password | Bcrypt 哈希存储（8–71 字节） |
| `verified` | bool | 邮箱是否已完成激活验证 |
| `name` | text(255) | 可选展示名称 |
| `allowed_models` | relation → `upstream_models` | 该账号允许调度的上游模型范围 |
| `weekly_token_quota` | int（`>= 0`） | 每个 UTC 自然周允许消耗的总 Token 上限（每周一 `00:00 UTC` 自动重置）；`0` 表示不限额 |
| `settings` | JSON（`1 MiB`） | 账号级 `session` 与 `blacklist` 策略 |

`users.settings` 配置示例：

```json
{
  "session": {
    "max_sessions": 10,
    "compaction": {
      "keep_tokens": 20000,
      "trigger_ratio": 0.85
    }
  },
  "blacklist": ["rm -rf /", "mkfs *", "shutdown", "reboot"]
}
```

### 强制吊销登录态

PocketBase Auth Token 为无状态 JWT（默认有效期 7 天）：
- **使单个用户立即下线**：在后台修改该用户的密码或邮箱（自动轮换该用户的 `tokenKey`），或直接删除用户；
- **使全员立即重新登录**：在 `/_/` → `users` → **Edit collection → Options → Token options** 中点击 **Invalidate all previously issued tokens**。

## 邀请码与新用户默认模板（`invite_codes` 表）

单张 `invite_codes` 表同时管理**准入配额**与**新用户初始权限模板**，用户在 `/register` 自助注册时在数据库事务内一次性完成核销与赋权：

| 字段 | 类型 | 说明 |
|---|---|---|
| `code` | text(4..64)，唯一 | 邀请码（在后台新建时留空会自动生成 8 位大写字母数字码，也可手动指定如 `TEAM-SRE-2026`） |
| `enabled` | bool | 启用开关，随时可暂停某个邀请码 |
| `max_uses` | int（`>= 0`） | 最大可用次数（`0` 表示不限次数） |
| `used_count` | int（`>= 0`） | 已注册消耗次数（并发注册时在事务内原子累加，绝不超发） |
| `expires_at` | date（UTC） | 过期时间（留空表示不过期） |
| `allowed_models` | relation → `upstream_models` | 注册成功后自动赋给新用户的可用模型 |
| `weekly_token_quota` | int（`>= 0`） | 注册成功后自动赋给新用户的每周 Token 限额 |
| `settings` | JSON | 注册成功后自动赋给新用户的黑名单与会话策略 |
| `note` | text(255) | 渠道或团队备注 |

你可以直接把带预填参数的注册链接发给团队成员或体验用户：

```text
https://aist.example.com/register?code=TEAM-SRE-2026
```

## 按日用量统计与自然周限额拦截

每次上游返回 `2xx` 响应后，服务端会提取 `usage` 中的 Token 消耗，按“**用户 × Provider × 模型 × UTC 日期**”原子累加写入 `daily_model_usage` 表：

| 字段 | 说明 |
|---|---|
| `user` | 关联 `users`（删除用户时级联清理） |
| `provider` / `model` | 保存纯文本名称快照，删除或调整上游模型配置不会丢失历史账单与当周已用额度 |
| `date` | UTC 日期（`YYYY-MM-DD`） |
| `requests` | 当日成功请求次数 |
| `prompt_tokens` / `completion_tokens` / `total_tokens` | 输入、输出与总 Token 汇总 |

代理转发前，若用户的 `weekly_token_quota > 0`，服务端会实时汇总该用户从**本周一 `00:00:00 UTC`** 起累计的 `total_tokens`。一旦达到或超过限额，立即返回 `429 quota_exceeded` 及恢复时间 `retry_at`（客户端自动换算为终端本地时区展示）。
