# HTTP API Contract & Error Codes

## Custom Server Endpoints (`/v1/*` & Static)

| Method | Path | Auth | Success | Error Statuses |
|---|---|---|---|---|
| `GET` | `/v1/healthz` | None | `200 {"status":"ok","version":"..."}` | — |
| `POST` | `/v1/register` | None (`application/json`, rate-limited) | `201 {"email":"...","verified":false,"verification_sent":bool}` | `400`, `415`, `429`, `500` |
| `GET` | `/v1/account/usage` | `Bearer <auth_token>` | `200 {"week_start":"...","reset_at":"...","requests":int,"prompt_tokens":int,"completion_tokens":int,"used_tokens":int,"weekly_token_quota":int,"remaining_tokens"?:int}` | `401`, `500` |
| `POST` | `/v1/proxy/chat/completions` | `Bearer <auth_token>` | Upstream `2xx` OpenAI completion JSON | `400`, `401`, `403`, `429`, `502`, `503` |
| `GET` | `/`, `/register` | None | Embedded landing & registration HTML (`__AIST_SERVER_URL__` injected from `Settings.Meta.AppURL`) | — |
| `GET` | `/run.sh`, `/uninstall.sh` | None | Bootstrap & uninstall shell scripts | — |
| `GET` | `/releases/client/*` | None | Version-matched client archives & `checksums.txt` | `404` |

## PocketBase User Auth Endpoints

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `GET` | `/api/collections/users/auth-methods` | None | Discover enabled password / OTP / MFA login modes |
| `POST` | `/api/collections/users/auth-with-password` | None | Password sign-in (returns `authResponse`, or `401 {"mfaId":"..."}` when MFA is required) |
| `POST` | `/api/collections/users/request-otp` | None | Send email OTP (`200 {"otpId":"..."}`) |
| `POST` | `/api/collections/users/auth-with-otp` | None | Exchange `otpId` + code (and optional `mfaId`) for `authResponse` |
| `POST` | `/api/collections/users/auth-refresh` | `Bearer <auth_token>` | Sliding token renewal + fresh `meta.config` |
| `POST` | `/api/collections/users/request-verification` | None | Resend email verification link (`204`) |
| `POST` | `/api/collections/users/request-password-reset` | None | Send password reset email (`204`) |

### `authResponse` Payload & `meta.config` Projection

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

## Structured Proxy Error Envelope

Non-`2xx` responses from `POST /v1/proxy/chat/completions` use a uniform JSON envelope so the client can surface localized, actionable guidance without blind retries:

```json
{
  "error": {
    "code": "quota_exceeded",
    "message": "weekly token quota exceeded",
    "retry_at": "2026-11-23T00:00:00Z"
  }
}
```

| HTTP Status | `error.code` | Retryable by Client? | Meaning |
|---|---|---|---|
| `400` | `invalid_request` | No | Malformed payload, context window exceeded, or upstream content/safety rejection |
| `401` | *(PocketBase auth)* | Refresh token once | Invalid, expired, or revoked user JWT |
| `403` | `no_available_model` | No | User has no enabled `allowed_models` (or all assigned models are blocked by `auth` error) |
| `429` | `quota_exceeded` | No | User's `weekly_token_quota` reached; `retry_at` indicates next Monday `00:00 UTC` |
| `503` | `no_available_model` | No | All assigned models are currently cooling down (`quota` or `rate_limit`); `retry_at` gives earliest recovery time |
| `502` | `upstream_unavailable` | Transient | All available candidates failed with transient `5xx` or network errors |

## `POST /v1/register` Error Codes

```json
{
  "code": "invite_code_exhausted",
  "message": "This invite code has reached its maximum uses."
}
```

Possible `code` values: `invalid_content_type` (`415`), `invalid_json`, `invite_code_required`, `invalid_invite_code`, `invite_code_expired`, `invite_code_exhausted`, `email_already_registered`, `validation_failed`, `registration_failed`.
