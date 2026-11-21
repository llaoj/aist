#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

DEMO_BIN="/tmp/aist-demo-bin"
DEMO_SRV_DIR="/tmp/aist-demo-server"
DEMO_APP_DIR="/tmp/aist-demo-app"
DEMO_HOME_EN="/tmp/aist-demo-home-en"
DEMO_HOME_ZH="/tmp/aist-demo-home-zh"
SERVER_URL="http://127.0.0.1:18090"
DEMO_ADMIN_PASS="${DEMO_ADMIN_PASS:-demo-$(openssl rand -hex 8)}"
DEMO_USER_PASS="${DEMO_USER_PASS:-demo-$(openssl rand -hex 8)}"
MODE="${1:-record}"
LOGIN_INFO_FILE="${2:-}"

cleanup() {
  if [[ -n "${UPSTREAM_PID:-}" ]]; then kill "$UPSTREAM_PID" 2>/dev/null || true; fi
  if [[ -n "${SERVER_PID:-}" ]]; then kill "$SERVER_PID" 2>/dev/null || true; fi
  rm -rf "$DEMO_SRV_DIR" "$DEMO_APP_DIR" "$DEMO_HOME_EN" "$DEMO_HOME_ZH" "$DEMO_BIN"
}
trap cleanup EXIT

rm -rf "$DEMO_BIN" "$DEMO_SRV_DIR" "$DEMO_APP_DIR" "$DEMO_HOME_EN" "$DEMO_HOME_ZH"
mkdir -p "$DEMO_BIN" "$DEMO_SRV_DIR" "$DEMO_APP_DIR" "$DEMO_HOME_EN/.aist" "$DEMO_HOME_ZH/.aist"

echo "Building demo aist & server binaries..."
go build -ldflags "-X main.BuildVersion=v0.6.0 -s -w" -o "$DEMO_BIN/aist" ./cmd/aist
go build -ldflags "-X main.BuildVersion=v0.6.0 -s -w" -o "$DEMO_BIN/server" ./cmd/server

node "$ROOT_DIR/scripts/demo-upstream.mjs" >/dev/null 2>&1 &
UPSTREAM_PID=$!

(
  cd "$DEMO_SRV_DIR"
  "$DEMO_BIN/server" superuser upsert admin@example.com "$DEMO_ADMIN_PASS" >/dev/null 2>&1
  export AIST_SERVER_ADDR="127.0.0.1:18090"
  exec "$DEMO_BIN/server" serve >/dev/null 2>&1
) &
SERVER_PID=$!

for _ in $(seq 1 40); do
  if curl -fsS "$SERVER_URL/v1/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 0.25
done

echo "Seeding demo server state..."
SERVER_URL="$SERVER_URL" \
DEMO_HOME_EN="$DEMO_HOME_EN" \
DEMO_HOME_ZH="$DEMO_HOME_ZH" \
DEMO_ADMIN_PASS="$DEMO_ADMIN_PASS" \
DEMO_USER_PASS="$DEMO_USER_PASS" \
node --input-type=module - <<'EOF'
import fs from 'node:fs';

const base = process.env.SERVER_URL;
async function api(path, method = 'GET', body = undefined, token = '') {
  const headers = { 'Content-Type': 'application/json' };
  if (token) headers.Authorization = token;
  const res = await fetch(`${base}${path}`, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined
  });
  if (!res.ok) {
    throw new Error(`${method} ${path} -> ${res.status} ${await res.text()}`);
  }
  return res.status === 204 ? null : res.json();
}

const adminAuth = await api('/api/collections/_superusers/auth-with-password', 'POST', {
  identity: 'admin@example.com',
  password: process.env.DEMO_ADMIN_PASS
});
const adminToken = adminAuth.token;

const pVllm = await api('/api/collections/upstream_providers/records', 'POST', {
  name: 'intranet-vllm',
  base_url: 'http://192.0.2.50:8000/v1',
  api_key: '',
  enabled: true
}, adminToken);

const pSf = await api('/api/collections/upstream_providers/records', 'POST', {
  name: 'siliconflow',
  base_url: 'https://api.siliconflow.cn/v1',
  api_key: 'configured-in-vault',
  enabled: true
}, adminToken);

const pDs = await api('/api/collections/upstream_providers/records', 'POST', {
  name: 'deepseek',
  base_url: 'http://127.0.0.1:18081/v1',
  api_key: 'configured-in-vault',
  enabled: true
}, adminToken);

const m1 = await api('/api/collections/upstream_models/records', 'POST', {
  provider: pVllm.id,
  name: 'Qwen/Qwen2.5-32B-Instruct',
  display_name: 'Qwen 2.5 32B (Daily Free)',
  priority: 10,
  context_window: 65536,
  max_output_tokens: 8192,
  enabled: true,
  unavailable_reason: 'quota',
  unavailable_until: '2099-01-01 00:00:00.000Z',
  last_error: '429 insufficient_quota: daily free quota exhausted'
}, adminToken);

const m2 = await api('/api/collections/upstream_models/records', 'POST', {
  provider: pSf.id,
  name: 'deepseek-ai/DeepSeek-V3',
  display_name: 'DeepSeek V3 (Trial Credits)',
  priority: 20,
  context_window: 128000,
  max_output_tokens: 8192,
  enabled: true,
  unavailable_reason: 'rate_limit',
  unavailable_until: '2099-01-01 00:05:00.000Z',
  last_error: '429 Too Many Requests (Retry-After: 30s)'
}, adminToken);

const m3 = await api('/api/collections/upstream_models/records', 'POST', {
  provider: pDs.id,
  name: 'deepseek-chat',
  display_name: 'DeepSeek Chat (Primary Fallback)',
  priority: 100,
  context_window: 128000,
  max_output_tokens: 8192,
  enabled: true
}, adminToken);

const defaultSettings = {
  session: { max_sessions: 10, compaction: { keep_tokens: 20000, trigger_ratio: 0.85 } },
  blacklist: ['rm -rf /', 'mkfs*', 'dd if=* of=/dev/*', 'shutdown*', 'reboot*']
};

await api('/api/collections/invite_codes/records', 'POST', {
  code: 'SRE-ONCALL-2026',
  enabled: true,
  max_uses: 50,
  used_count: 12,
  allowed_models: [m1.id, m2.id, m3.id],
  weekly_token_quota: 500000,
  settings: defaultSettings,
  note: 'Platform SRE Team'
}, adminToken);

await api('/api/collections/invite_codes/records', 'POST', {
  code: 'COMMUNITY-TRIAL',
  enabled: true,
  max_uses: 200,
  used_count: 87,
  allowed_models: [m1.id, m2.id, m3.id],
  weekly_token_quota: 100000,
  settings: defaultSettings,
  note: 'Public Early Access'
}, adminToken);

await api('/api/collections/users/records', 'POST', {
  email: 'alice@example.com',
  emailVisibility: true,
  verified: true,
  password: process.env.DEMO_USER_PASS,
  passwordConfirm: process.env.DEMO_USER_PASS,
  name: 'Alice (SRE)',
  allowed_models: [m1.id, m2.id, m3.id],
  weekly_token_quota: 500000,
  settings: defaultSettings
}, adminToken);

const userAuth = await api('/api/collections/users/auth-with-password', 'POST', {
  identity: 'alice@example.com',
  password: process.env.DEMO_USER_PASS
});

const creds = JSON.stringify({
  server_url: base,
  token: userAuth.token,
  email: 'alice@example.com'
}, null, 2) + '\n';

fs.writeFileSync(`${process.env.DEMO_HOME_EN}/.aist/token.json`, creds, { mode: 0o600 });
fs.writeFileSync(`${process.env.DEMO_HOME_EN}/.aist/preferences.json`, '{\n  "language": "en"\n}\n', { mode: 0o600 });

fs.writeFileSync(`${process.env.DEMO_HOME_ZH}/.aist/token.json`, creds, { mode: 0o600 });
fs.writeFileSync(`${process.env.DEMO_HOME_ZH}/.aist/preferences.json`, '{\n  "language": "zh"\n}\n', { mode: 0o600 });

// Seed realistic tool audit logs & token usage via real /v1/proxy/chat/completions
await api('/v1/proxy/chat/completions', 'POST', {
  model: 'auto',
  messages: [
    { role: 'user', content: 'orders-api returns 502, check /etc/nginx/conf.d/orders.conf and fix it' },
    {
      role: 'assistant',
      content: '',
      tool_calls: [
        {
          id: 'call_audit_1',
          type: 'function',
          function: {
            name: 'shell_execute',
            arguments: JSON.stringify({
              step_index: 1,
              command_type: 'query',
              target: 'prod-web1',
              reason: 'Inspect upstream config and verify healthy backend port on prod-web1',
              command: 'cat /etc/nginx/conf.d/orders.conf && curl -sS http://127.0.0.1:8080/healthz'
            })
          }
        },
        {
          id: 'call_audit_2',
          type: 'function',
          function: {
            name: 'file_edit',
            arguments: JSON.stringify({
              path: '/etc/nginx/conf.d/orders.conf',
              target: 'prod-web1',
              sudo: true,
              reason: 'Update upstream port from 8089 to healthy backend port 8080',
              old_content: '    server 127.0.0.1:8089 max_fails=3 fail_timeout=10s;',
              new_content: '    server 127.0.0.1:8080 max_fails=3 fail_timeout=10s;'
            })
          }
        }
      ]
    },
    {
      role: 'tool',
      tool_call_id: 'call_audit_1',
      content: 'Status: success\nTarget: prod-web1\nExit Code: 0\nCommand: cat /etc/nginx/conf.d/orders.conf && curl -sS http://127.0.0.1:8080/healthz\n\n[Output]\nupstream orders_backend {\n    server 127.0.0.1:8089 max_fails=3 fail_timeout=10s;\n}\n{"service":"orders-api","status":"healthy","port":8080}'
    },
    {
      role: 'tool',
      tool_call_id: 'call_audit_2',
      content: 'Status: success\nTarget: prod-web1\nPath: /etc/nginx/conf.d/orders.conf\nSudo: true\nDetail: File updated successfully after user confirmation.'
    }
  ]
}, `Bearer ${userAuth.token}`);
EOF

if [[ "$MODE" == "--admin-preview" ]]; then
  if [[ -n "$LOGIN_INFO_FILE" ]]; then
    mkdir -p "$(dirname "$LOGIN_INFO_FILE")"
    printf 'email: admin@example.com\npassword: %s\n' "$DEMO_ADMIN_PASS" > "$LOGIN_INFO_FILE"
    chmod 600 "$LOGIN_INFO_FILE"
  fi
  echo "Demo server ready at $SERVER_URL/_/"
  wait "$SERVER_PID"
  exit 0
fi

reset_nginx_conf() {
  cat > "$DEMO_APP_DIR/nginx-upstream.conf" <<'CONF'
upstream orders_backend {
    server 127.0.0.1:18089 max_fails=3 fail_timeout=10s;
    keepalive 32;
}
CONF
}

echo "Recording English demo GIF (docs/public/demo.gif)..."
reset_nginx_conf
vhs "$ROOT_DIR/scripts/demo-en.tape"

echo "Recording Chinese demo GIF (docs/public/demo-zh.gif)..."
reset_nginx_conf
vhs "$ROOT_DIR/scripts/demo-zh.tape"

echo "Demo recordings complete!"
