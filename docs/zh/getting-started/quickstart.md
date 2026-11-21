# 快速体验（官方服务）

无需部署任何基础设施，两分钟内即可在本地终端或测试服务器上体验 `https://aistrun.cc` 官方体验服务。

## 1. 使用邀请码注册账号

1. 在浏览器打开 `https://aistrun.cc/register`（若持有专属链接 `https://aistrun.cc/register?code=<邀请码>`，邀请码会自动填入）。
2. 输入邀请码、邮箱与登录密码（8–71 个字符）完成创建。
3. 若提示已发送验证邮件，请先点击邮件中的激活链接完成邮箱验证。

## 2. 一行命令安装并登录

在 Linux 或 macOS 终端执行：

```bash
curl -fsSL https://aistrun.cc/run.sh | sh
```

如果机器上只有 `wget`：

```bash
wget -qO- https://aistrun.cc/run.sh | sh
```

`run.sh` 会自动完成以下步骤：

1. 识别当前操作系统与架构，从服务端下载同版本客户端压缩包并严格校验 `checksums.txt` 的 SHA-256 摘要；
2. 将 `aist` 安装到可写的 `PATH` 目录（依次尝试 `/usr/local/bin`、`~/.local/bin`、`~/bin`）；
3. 首次运行时根据系统语言环境自动推断一次默认语言（`zh` 或 `en`）并写入 `~/.aist/preferences.json`；
4. 自动拉起 `aist login https://aistrun.cc` 引导你输入邮箱与密码完成登录。

## 3. 开始第一次排障

进入多轮交互模式：

```bash
aist
```

或者直接在命令行发起单次排查：

```bash
aist "检查当前磁盘空间，找出 /var/log 下占用最大的目录"
```

也可以将日志或命令输出通过管道交给 AIST 做只读分析：

```bash
dmesg -T | tail -n 100 | aist "有没有 OOM 或硬件异常？"
```

## 4. 随时切换中英文语言

客户端界面提示与 AI 回复语言保存在本地 `~/.aist/preferences.json`，免登录即可随时查看或切换：

```bash
aist lang       # 查看当前语言（zh 或 en）
aist lang zh    # 切换为简体中文
aist lang en    # 切换为英文
```

## 常用快捷键

| 所在位置 | 按键 | 效果 |
|---|---|---|
| 模型思考或等待响应时 | `Esc` | 仅取消当前请求并回到输入框，已保存的上下文不会丢失 |
| 输入框或确认菜单 | `Ctrl+C` | 安全退出当前会话（随时可通过 `aist sessions` 恢复） |
| 命令 / 文件修改确认菜单 | `↑` / `↓` + `Enter` | 选择「执行」、「跳过此命令」或「拒绝全部，由我补充说明」 |
