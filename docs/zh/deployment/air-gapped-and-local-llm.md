# 纯内网离线部署与本地模型接入（vLLM / Ollama）

AIST 从架构设计之初就完整支持无外网（Air-Gapped）企业内网环境，数据、客户端安装包与模型流量全程不出内网。

## 为什么开箱支持完全断网运行

1. **镜像内置全平台客户端**：`ghcr.io/llaoj/aist-server` 镜像（总大小约 42 MB）已在 `/usr/share/aist/client` 内置全部 7 个 Linux / macOS 平台的客户端压缩包与 `checksums.txt`。
2. **闭环安装脚本**：内网机器执行 `curl -fsSL https://aist.corp.internal/run.sh | sh` 时，脚本直接从本服务端 `/releases/client/` 下载并校验二进制，**零外网请求，不依赖 GitHub 或公共 CDN**。
3. **零外部静态依赖**：落地页 `/` 与注册页 `/register` 的所有 HTML、CSS、SVG 与 JS 均已静态编译进 `server` 二进制。

## 第 1 步：离线导出并导入服务端镜像

在有外网的机器拉取并导出镜像：

```bash
docker pull --platform linux/amd64 ghcr.io/llaoj/aist-server:0.6.0
docker save ghcr.io/llaoj/aist-server:0.6.0 | gzip > aist-server-0.6.0-amd64.tar.gz
```

拷贝到内网服务器后加载并启动：

```bash
docker load -i aist-server-0.6.0-amd64.tar.gz
```

按 [Docker Compose 生产部署](./docker-compose.md) 启动容器，并在后台 **Settings → Application → Application URL** 填入内网访问地址（如 `https://aist.corp.internal`；若使用企业自签证书，请确保客户端机器已信任内网根证书）。

## 第 2 步：接入内网 vLLM 或 Ollama

任何兼容 OpenAI `/chat/completions` 接口且**支持 Tool Calling（函数调用）**的模型服务均可直接接入。

> [!IMPORTANT]
> 排障闭环高度依赖稳定的工具参数生成与读写命令判别能力，建议选用 32B 及以上规模的指令模型（如 Qwen2.5/Qwen3-32B+、DeepSeek-V3/R1 系列等），不建议使用 9B 以下小模型。

### 接入 vLLM

启动 vLLM 时请开启工具调用解析参数（以 Qwen 为例）：

```bash
vllm serve Qwen/Qwen2.5-32B-Instruct \
  --enable-auto-tool-choice \
  --tool-call-parser hermes \
  --max-model-len 65536 \
  --port 8000
```

在 AIST 管理后台（`/_/`）配置：
- **`upstream_providers`**：
  - `name`：`intranet-vllm`
  - `base_url`：`http://192.0.2.50:8000/v1`
  - `api_key`：未设鉴权可留空（留空时服务端转发不携带 `Authorization` 头），若有 `--api-key` 则填入
  - `enabled`：启用
- **`upstream_models`**：
  - `provider`：`intranet-vllm`
  - `name`：`Qwen/Qwen2.5-32B-Instruct`
  - `display_name`：`Qwen 2.5 32B（内网）`
  - `priority`：`10`
  - `context_window`：`65536`
  - `max_output_tokens`：`8192`
  - `enabled`：启用

### 接入 Ollama

将 Ollama 监听对 AIST 服务端开放后，在后台配置：
- **`upstream_providers`**：
  - `name`：`local-ollama`
  - `base_url`：`http://192.0.2.51:11434/v1`
  - `api_key`：留空即可
  - `enabled`：启用
- **`upstream_models`**：
  - `name`：如 `qwen2.5:32b`
  - `context_window`：与 Ollama Modelfile 中配置的 `num_ctx` 保持一致

## 第 3 步：内网跳板机与运维终端一键安装

在内网任意 Linux 或 macOS 机器上执行：

```bash
curl -fsSL https://aist.corp.internal/run.sh | sh
```

即可完成离线安装、校验与登录。
