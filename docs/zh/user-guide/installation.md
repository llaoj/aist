# 安装、升级、卸载与语言切换

## 支持平台

`aist` 客户端为纯静态零外部依赖二进制（`CGO_ENABLED=0`），预编译支持以下平台：

- **Linux**：`amd64`、`arm64`、`arm`（`GOARM=6`，兼容 ARMv6/v7）、`386`、`riscv64`
- **macOS**：`arm64`（Apple Silicon）、`amd64`（Intel）
- **Windows**：当前推荐在 **WSL2** 中直接安装运行 Linux 版本（原生客户端规划见 [Roadmap #6](https://github.com/llaoj/aist/issues/6)）

## 使用 `run.sh` 安装与自动升级

任何一台 AIST 服务端都自带 `/run.sh` 脚本，并在 `/releases/client/` 下托管同版本客户端安装包：

```bash
curl -fsSL https://<你的服务端地址>/run.sh | sh
```

### 版本对齐策略

客户端与服务端采用**完全统一的版本号**（如 `v0.6.0`）。重复执行 `run.sh` 时按以下规则处理：

| 本机当前状态 | `run.sh` 行为 |
|---|---|
| 尚未安装 | 下载服务端同版本压缩包，校验 `checksums.txt` 中的 SHA-256，解压安装到 `/usr/local/bin`、`~/.local/bin` 或 `~/bin` 并启动 |
| 与服务端版本一致 | 直接启动 `aist` |
| 比服务端版本旧 | 校验通过后静默覆盖升级到服务端版本并启动（若下载失败则回退运行本机现有版本） |
| 比服务端版本新 | 终端交互询问是否降级安装服务端版本（`[y/N]`，默认保留当前版本；非交互环境直接保留） |

启动前 `run.sh` 会自动执行 `aist whoami` 检查登录态：若尚未登录，会自动拉起 `aist login <服务端地址>` 完成首次登录。

## 手动下载压缩包安装

你也可以从 GitHub Releases 或 `https://<服务端地址>/releases/client/<version>/` 手动下载压缩包并校验安装：

```bash
VERSION=v0.6.0
OS=linux      # linux 或 darwin
ARCH=amd64    # amd64, arm64, arm, 386, riscv64

curl -fsSLO "https://aistrun.cc/releases/client/${VERSION}/aist-${VERSION}-${OS}-${ARCH}.tar.gz"
curl -fsSLO "https://aistrun.cc/releases/client/${VERSION}/checksums.txt"
grep "aist-${VERSION}-${OS}-${ARCH}.tar.gz" checksums.txt | shasum -a 256 -c -

tar -xzf "aist-${VERSION}-${OS}-${ARCH}.tar.gz"
install -m 0755 "aist-${VERSION}-${OS}-${ARCH}" /usr/local/bin/aist
```

查看当前客户端版本：

```bash
aist version
```

## 客户端语言偏好（`aist lang`）

终端界面提示、确认菜单、错误说明以及 AI 回复语言完全归属客户端本地维护，存储在 `~/.aist/preferences.json`（权限 `0600`），不在服务端入库。

- **首次自动推断并固化**：当 `~/.aist/preferences.json` 尚不存在时（例如首次执行 `run.sh`），客户端会按 `LC_ALL` → `LC_MESSAGES` → `LANG` 顺序推断默认语言；若终端未设置语言或仅导出了 `UTF-8` 编码占位，在 macOS 上会自动读取系统首选语言（`AppleLanguages` / `AppleLocale`），其余环境默认回退 `en`，推断结果立即写入 `preferences.json`。
- **随时查看与切换**（无需登录，`aist logout` 退出登录也会保留语言设置）：

```bash
aist lang       # 查看当前生效语言（zh 或 en）
aist lang zh    # 切换为简体中文
aist lang en    # 切换为英文
```

切换语言后，不仅新会话的 AI 回复与界面文案立即生效，通过 `aist sessions` 恢复旧会话时，历史工具调用状态也会跟随当前语言重新渲染。

## 卸载客户端

执行服务端提供的卸载脚本，可清理已安装的 `aist` 二进制与本机状态目录 `~/.aist`：

```bash
curl -fsSL https://<你的服务端地址>/uninstall.sh | sh
```

非交互环境下可显式传入 `--yes` 或 `AIST_YES=1`：

```bash
curl -fsSL https://<你的服务端地址>/uninstall.sh | sh -s -- --yes
```

`uninstall.sh` 不会修改你的 Shell 配置文件（如 `~/.zshrc`、`~/.bashrc`），也不会影响任何服务端数据。
