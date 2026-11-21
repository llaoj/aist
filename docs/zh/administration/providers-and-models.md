# 模型路由、优先级与自动冷却恢复

客户端在调用 `POST /v1/proxy/chat/completions` 时固定只传逻辑模型 `"model": "auto"`。具体的 Provider 选择、优先级排序、`max_tokens` 裁剪、上游报错识别、自动冷却退避与恢复全部在服务端集中完成。

![管理后台模型优先级与自动冷却状态](/admin-models.png)

## 核心数据表

### `upstream_providers`（上游服务商）

| 字段 | 类型 | 说明 |
|---|---|---|
| `name` | text（唯一） | 服务商标识（如 `deepseek`、`siliconflow`、`vllm`） |
| `base_url` | text | OpenAI 兼容接口根地址（不带 `/chat/completions`，如 `https://api.deepseek.com/v1`） |
| `api_key` | text | 上游 API Key；后台接口回显固定掩码为 `••••••••`，启用 `--encryptionEnv` 时自动 AES-256-GCM 加密落盘 |
| `enabled` | bool | 关闭后其下关联的所有模型立即跳过 |

### `upstream_models`（上游模型与运行状态）

| 字段 | 类型 | 说明 |
|---|---|---|
| `provider` | relation → `upstream_providers` | 所属服务商（级联删除） |
| `name` | text(128) | 转发给上游的真实模型 ID（`unique(provider, name)`） |
| `display_name` | text(128) | 后台选择模型时展示的易读名称 |
| `priority` | int | **升序优先级**：数值越小越先尝试（`10` 优先于 `20` 优先于 `100`；同优先级按 `provider` 与 `name` 字典序兜底） |
| `context_window` | int | 上下文窗口长度；下发给用户的有效上下文上限取候选模型中的最小正值（默认 `128000`） |
| `max_output_tokens` | int | 最大输出 Token；转发时自动将请求 `max_tokens` 限制在该模型上限内（默认 `8192`） |
| `enabled` | bool | 模型启用开关 |
| `unavailable_until` | date（UTC） | 自动冷却截止时间；正常或因鉴权错误永久阻塞时为空 |
| `unavailable_reason` | select | 不可用原因枚举：`quota` / `rate_limit` / `auth` |
| `last_error` | text(1024) | 最近一次标记不可用时的脱敏错误摘要 |

## 推荐的多级模型梯度编排

利用 `priority` 字段，可以把内网算力、平台每日免费额度、体验赠金与主力付费 API 串成高可用容灾链：

| `priority` | 典型梯队 | 运行效果 |
|---|---|---|
| `10` | 内网本地模型 / 每日免费额度模型 | 每次请求最先尝试；当日额度耗尽后自动进入冷却，到期自动探测恢复 |
| `20` | 新户赠金 / 备用云厂商模型 | 当第一梯队冷却或限流时无缝承接流量 |
| `100` | 主力付费模型（如 DeepSeek 官方） | 保底兜底，确保终端排障不中断 |

## 上游错误分类与自动冷却策略

当某个候选模型返回非 `2xx` 响应时，服务端结合 HTTP 状态码与响应体中的 `error.code` / `error.type` / 错误文本精确分类（已适配 DeepSeek、硅基流动、智谱及主流 OpenAI 兼容平台）：

| 错误类别 | 判定条件 | 是否继续 Fallback | 冷却与恢复策略 |
|---|---|---|---|
| **额度 / 余额耗尽**（`quota`） | HTTP `402`，或带 `insufficient_quota` / `insufficient_balance` 等余额不足特征的 `429` | **是** → 自动切下一个模型 | 标记 `unavailable_reason = "quota"`，首次冷却 **6 小时**；冷却到期重试若仍失败，退避延长至 **24 小时** |
| **并发 / 速率限流**（`rate_limit`） | 非额度耗尽类的 HTTP `429` | **是** → 自动切下一个模型 | 标记 `unavailable_reason = "rate_limit"`，优先遵循上游 `Retry-After` 头（无头时首次冷却 **30 秒**，连续失败退避至 **2 分钟**，上限 24 小时） |
| **Key 无效 / 无权限**（`auth`） | HTTP `401` 或 `403` | **是** → 自动切下一个模型 | 标记 `unavailable_reason = "auth"` 且不自动重试；管理员在后台更新该 Provider 的 `api_key`（或手动清空 `unavailable_reason`）后立即自动恢复 |
| **上游临时故障** | HTTP `5xx` 或网络超时 | **是** → 自动切下一个模型 | **不标记**模型不可用，仅当次请求回退 |
| **客户端请求本身有误** | HTTP `400`（如输入超长、安全策略拦截） | **否** → 直接向客户端返回结构化 `400 invalid_request` | 确定性的请求输入问题不盲目消耗后续候选模型 |

冷却期满后，下一次请求会自动优先探测该模型：一旦返回 `2xx`，服务端立即清空 `unavailable_reason`、`unavailable_until` 与 `last_error`，恢复正常调度。
