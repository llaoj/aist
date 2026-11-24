# Providers, Priority Routing & Automatic Cooldown

Clients always send `"model": "auto"` to `POST /v1/proxy/chat/completions`. All provider selection, priority ordering, token limit clamping, error classification, cooldown backoff, and automatic recovery happen centrally on the server.

![Model priority routing and cooldown status in Admin UI](/admin-models.png)

## Data Collections

### `upstream_providers`

| Field | Type | Description |
|---|---|---|
| `name` | text (unique) | Provider identifier (e.g. `deepseek`, `siliconflow`, `vllm`) |
| `base_url` | text | OpenAI-compatible root URL (without `/chat/completions`, e.g. `https://api.deepseek.com/v1`) |
| `api_key` | text | Upstream API key; always masked as `••••••••` in API responses and optionally encrypted at rest with `--encryptionEnv` |
| `enabled` | bool | Disabling a provider immediately skips all models belonging to it |

### `upstream_models`

| Field | Type | Description |
|---|---|---|
| `provider` | relation → `upstream_providers` | Parent provider (cascade delete) |
| `name` | text(128) | Upstream model ID sent in JSON body (`unique(provider, name)`) |
| `display_name` | text(128) | Friendly label shown in admin UI relations |
| `priority` | int | **Ascending priority**: smaller values are tried first (`10` before `20` before `100`; ties break alphabetically by `provider`, then `name`) |
| `context_window` | int | Model context limit; user's effective `limits.context_window` is the minimum positive value across enabled candidates (default `128000`) |
| `max_output_tokens` | int | Outgoing `max_tokens` is clamped per candidate to this ceiling (default `8192`) |
| `enabled` | bool | Master switch for this model |
| `unavailable_until` | date (UTC) | Cooldown expiration timestamp; empty when healthy or permanently blocked by `auth` |
| `unavailable_reason` | select | `quota` / `rate_limit` / `auth` / `transient` |
| `last_error` | text(1024) | Sanitized upstream error summary from the latest failure |

## Multi-Tier Cost & Reliability Strategy

Use `priority` to layer free daily quotas, promotional credits, and paid fallback models seamlessly:

| `priority` | Tier Example | Behavior |
|---|---|---|
| `10` | Internal vLLM or daily free-quota provider | Tried first on every request; automatically cools down when daily quota runs out and resumes automatically after cooldown |
| `20` | Secondary / trial credit provider | Absorbs overflow when tier `10` is cooling down |
| `100` | Paid primary provider (e.g. DeepSeek API) | Reliable final fallback so users never see downtime |

## Upstream Error Classification & Cooldown Rules

When a candidate returns an error during `POST /v1/proxy/chat/completions`, the server classifies both the HTTP status code and response JSON (`error.code` / `error.type` / message):

| Category | Upstream Signal | Fallback? | Cooldown / Recovery Policy |
|---|---|---|---|
| **Quota / Balance Exhausted** (`quota`) | HTTP `402`, or `429` with `insufficient_quota` / `insufficient_balance` / balance markers | **Yes** → tries next candidate | Marks `unavailable_reason = "quota"`, cools down **6 hours** on first hit and **24 hours** if a probe after expiry fails again |
| **Rate Limited** (`rate_limit`) | HTTP `429` (non-quota) | **Yes** → tries next candidate | Marks `unavailable_reason = "rate_limit"`, honors `Retry-After` header (default **30s**, escalating to **2m** on consecutive failure, capped at 24h) |
| **Invalid Key / Unauthorized** (`auth`) | HTTP `401` or `403` | **Yes** → tries next candidate | Marks `unavailable_reason = "auth"` with empty `unavailable_until` (no automatic retry). Updating the provider's `api_key` or clearing `unavailable_reason` in `/_/` immediately restores it |
| **Transient Upstream Fault / Timeout** (`transient`) | HTTP `5xx` or per-candidate timeout (default `30s`; total proxy budget `85s`) | **Yes** → tries next candidate | Marks `unavailable_reason = "transient"`, cools down **1 minute** on first hit and **3 minutes** on consecutive failure |
| **Invalid Client Request** | HTTP `400` (context length exceeded, safety filter, bad schema) | **No** → returns structured `400 invalid_request` immediately | Does not waste fallback calls on deterministic client input errors |

When a cooled-down model's `unavailable_until` passes, the next eligible request automatically probes it first: a `2xx` response clears `unavailable_reason`, `unavailable_until`, and `last_error` automatically.
