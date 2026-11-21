# Air-Gapped Deployment & Local LLMs (vLLM / Ollama)

AIST is designed from the ground up to operate inside completely isolated enterprise intranets where neither the server nor client hosts have outbound internet access.

## Why AIST Works Offline Out of the Box

1. **Bundled Client Distribution**: `ghcr.io/llaoj/aist-server` embeds all 7 Linux/macOS client `.tar.gz` archives and `checksums.txt` inside `/usr/share/aist/client` (~28 MB compressed, ~42 MB total image).
2. **Self-Contained Bootstrap Script**: When an internal server runs `curl -fsSL https://aist.corp.internal/run.sh | sh`, `run.sh` downloads `aist` directly from `https://aist.corp.internal/releases/client/` — **zero calls to GitHub, CDNjs, or external registries**.
3. **Strict Self-Hosted Web Assets**: All HTML, CSS, SVG logos, and JS in `/` and `/register` are embedded in the `server` Go binary.

## Step 1: Import the Server Image into Your Air-Gapped Network

On an internet-connected workstation:

```bash
docker pull --platform linux/amd64 ghcr.io/llaoj/aist-server:0.6.0
docker save ghcr.io/llaoj/aist-server:0.6.0 | gzip > aist-server-0.6.0-amd64.tar.gz
```

Transfer `aist-server-0.6.0-amd64.tar.gz` into your internal network and load it:

```bash
docker load -i aist-server-0.6.0-amd64.tar.gz
```

Start with Docker Compose (using internal TLS certificates on your reverse proxy, or trusted internal CA installed on client nodes) and set **Settings → Application → Application URL** to your internal address (e.g. `https://aist.corp.internal`).

## Step 2: Connect vLLM or Ollama as an Upstream Provider

Any OpenAI-compatible `/chat/completions` endpoint that supports **function/tool calling** (`tools` array) works with AIST.

> [!IMPORTANT]
> Choose models with reliable tool-calling capabilities (for example Qwen2.5/Qwen3 32B+, DeepSeek-V3/R1-Distill 32B+, or Llama 3.3 70B). Small models below ~14B often hallucinate shell JSON arguments or misclassify `query` vs `modify` commands.

### vLLM Configuration

Start vLLM with tool-call parsing enabled (example with Qwen):

```bash
vllm serve Qwen/Qwen2.5-32B-Instruct \
  --enable-auto-tool-choice \
  --tool-call-parser hermes \
  --max-model-len 65536 \
  --port 8000
```

In AIST Admin (`/_/`):
- **`upstream_providers`**:
  - `name`: `intranet-vllm`
  - `base_url`: `http://192.0.2.50:8000/v1`
  - `api_key`: leave empty if unauthenticated, or enter your vLLM `--api-key`
  - `enabled`: `true`
- **`upstream_models`**:
  - `provider`: `intranet-vllm`
  - `name`: `Qwen/Qwen2.5-32B-Instruct`
  - `display_name`: `Qwen 2.5 32B (Intranet)`
  - `priority`: `10`
  - `context_window`: `65536`
  - `max_output_tokens`: `8192`
  - `enabled`: `true`

### Ollama Configuration

Expose Ollama to your AIST server container and create:
- **`upstream_providers`**:
  - `name`: `local-ollama`
  - `base_url`: `http://192.0.2.51:11434/v1`
  - `api_key`: *(leave empty — AIST omits the `Authorization` header when `api_key` is empty)*
  - `enabled`: `true`
- **`upstream_models`**:
  - `name`: e.g. `qwen2.5:32b`
  - `context_window`: match `num_ctx` configured on your Ollama Modelfile

## Step 3: Install Clients Across Internal Jump Hosts

On any Linux or macOS machine inside the intranet:

```bash
curl -fsSL https://aist.corp.internal/run.sh | sh
```

Data never leaves your private network.
