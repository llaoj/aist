# 登录认证与邀请码注册

## 使用邀请码自助注册

当管理员发放邀请码后，用户可在浏览器访问服务端的注册页创建账号：

```text
https://<服务端地址>/register
https://<服务端地址>/register?code=<邀请码>
```

1. 填写邀请码、邮箱与密码（8–71 个字符）提交注册；
2. 新账号会自动继承该邀请码关联的可用模型白名单（`allowed_models`）、每周 Token 限额（`weekly_token_quota`）以及扩展策略（`blacklist`、`session`）；
3. 若服务端开启了邮件服务（SMTP），系统会自动发送激活验证邮件，点击邮件中的验证链接即可激活（若未收到可在注册结果页点击重新发送）；
4. 若忘记密码，可随时访问 `https://<服务端地址>/register#reset` 发送密码重置邮件。

## 命令行登录（`aist login`）

```bash
aist login                              # 默认使用上次记忆的服务端或 https://aistrun.cc
aist login https://aist.example.com     # 指定自建服务端地址
aist login -e alice@example.com 127.0.0.1:8090
```

### 服务端地址优先级

1. `aist login [server]` 位置参数
2. `AIST_SERVER` 环境变量
3. `~/.aist/token.json` 中已记忆的 `server_url`
4. 默认地址 `https://aistrun.cc`

地址自动补全规则：
- 未写协议头时默认补 `https://`（如 `aist.example.com` → `https://aist.example.com`）；
- 本机回环地址（`localhost`、`127.*`、`[::1]`）默认补 `http://`（如 `127.0.0.1:8090` → `http://127.0.0.1:8090`）。

> [!TIP]
> 本机尚未登录时，`aist login` 会在输入邮箱前先把目标 `server_url` 记入 `token.json`。即使你在 `run.sh` 首次引导登录时按 `Ctrl+C` 取消，之后直接敲 `aist login` 也会自动连向同一台服务端。

### 自适应登录方式

客户端会先查询服务端的 `GET /api/collections/users/auth-methods`，自动匹配后台 `users` 集合开启的认证模式：

| 服务端开启的认证组合 | 客户端交互流程 |
|---|---|
| **邮箱 + 密码** | 终端隐藏输入密码直接登录 |
| **仅邮箱验证码（OTP）** | 自动触发验证码邮件，在终端输入一次性验证码登录 |
| **密码 + 邮件二次验证（MFA）** | 先校验密码，再发送并校验一次性邮件验证码 |

密码全程在终端隐藏读取，**刻意不提供 `--password` 参数或密码环境变量**，避免密码残留在 Shell 历史记录或进程列表中。

## 查看状态与退出登录

```bash
aist whoami   # 纯本地读取当前登录邮箱与服务端地址（不发起联网请求）
aist logout   # 删除本地 ~/.aist/token.json（保留语言偏好 preferences.json、历史会话与环境记忆）
```

### Token 自动续期与配置同步

- 登录凭证保存在 `~/.aist/token.json`（权限 `0600`），默认有效期 7 天（滑动续期）。
- 每次启动 `aist` 会话时，客户端都会调用一次 `POST /api/collections/users/auth-refresh` 刷新 Token，并在内存中拉取最新的账号配置快照（模型上下文限制、压缩策略、命令黑名单），服务端配置不落盘。
- 长时间排障过程中，若距离 Token 过期不足 60 秒会自动提前刷新；遇到代理返回 `401` 时也会自动尝试刷新并重发一次。
