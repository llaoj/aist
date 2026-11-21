import http from 'node:http';

const upstreamPort = Number(process.env.DEMO_UPSTREAM_PORT || 18081);
const backendPort = Number(process.env.DEMO_BACKEND_PORT || 18080);

const backendServer = http.createServer((req, res) => {
  res.writeHead(200, { 'Content-Type': 'application/json' });
  res.end(JSON.stringify({ service: 'orders-api', status: 'healthy', port: backendPort }) + '\n');
});

const llmServer = http.createServer((req, res) => {
  if (req.method !== 'POST' || !req.url?.endsWith('/chat/completions')) {
    res.writeHead(404);
    res.end('not found');
    return;
  }

  let raw = '';
  req.on('data', chunk => { raw += chunk; });
  req.on('end', () => {
    const payload = JSON.parse(raw || '{}');
    const messages = Array.isArray(payload.messages) ? payload.messages : [];
    const sys = messages.find(m => m.role === 'system')?.content || '';
    const isZh = sys.includes('Chinese (Simplified)');
    const toolCount = messages.filter(m => m.role === 'tool').length;

    let message;
    if (toolCount === 0) {
      message = {
        role: 'assistant',
        content: '',
        tool_calls: [
          {
            id: 'call_demo_inspect_1',
            type: 'function',
            function: {
              name: 'shell_execute',
              arguments: JSON.stringify({
                step_index: 1,
                command_type: 'query',
                target: 'local',
                reason: isZh
                  ? '检查 Nginx 上游配置并探测本机 18080 后端健康状态'
                  : 'Inspect nginx upstream config and probe local backend health on port 18080',
                command: 'cat /tmp/aist-demo-app/nginx-upstream.conf && curl -sS http://127.0.0.1:18080/healthz'
              })
            }
          }
        ]
      };
    } else if (toolCount === 1) {
      message = {
        role: 'assistant',
        content: '',
        tool_calls: [
          {
            id: 'call_demo_edit_2',
            type: 'function',
            function: {
              name: 'file_edit',
              arguments: JSON.stringify({
                path: '/tmp/aist-demo-app/nginx-upstream.conf',
                target: 'local',
                sudo: false,
                reason: isZh
                  ? '将上游端口从错误的 18089 修正为正常运行的 18080'
                  : 'Update upstream port from 18089 to healthy backend port 18080',
                old_content: '    server 127.0.0.1:18089 max_fails=3 fail_timeout=10s;',
                new_content: '    server 127.0.0.1:18080 max_fails=3 fail_timeout=10s;'
              })
            }
          }
        ]
      };
    } else {
      message = {
        role: 'assistant',
        content: isZh
          ? '已定位并修复 `/tmp/aist-demo-app/nginx-upstream.conf` 的 502 上游端口配置错误：\n\n- **根因**：上游指向了未监听的 `127.0.0.1:18089`，实际 `orders-api` 在 `127.0.0.1:18080` 健康运行。\n- **变更**：已将 `server` 端口更新为 `18080`（原文件已自动备份至 `~/.aist/fileedit/backups/`）。'
          : 'Resolved the 502 upstream port mismatch in `/tmp/aist-demo-app/nginx-upstream.conf`:\n\n- **Root cause**: Upstream pointed to `127.0.0.1:18089`, while `orders-api` is healthy on `127.0.0.1:18080`.\n- **Fix applied**: Updated `server` to `127.0.0.1:18080` (original backed up in `~/.aist/fileedit/backups/`).'
      };
    }

    setTimeout(() => {
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({
        id: `chatcmpl-demo-${toolCount + 1}`,
        object: 'chat.completion',
        created: Math.floor(Date.now() / 1000),
        model: 'deepseek-chat',
        choices: [{ index: 0, message, finish_reason: message.tool_calls ? 'tool_calls' : 'stop' }],
        usage: {
          prompt_tokens: 1380 + toolCount * 240,
          completion_tokens: 92,
          total_tokens: 1472 + toolCount * 240
        }
      }));
    }, 450);
  });
});

backendServer.listen(backendPort, '127.0.0.1');
llmServer.listen(upstreamPort, '127.0.0.1', () => {
  console.log(`demo upstream listening on ${upstreamPort}, backend on ${backendPort}`);
});
