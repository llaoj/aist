# Local Development, Build & Testing

## Prerequisites

- **Go**: version specified in `go.mod` (`1.27+`)
- **Git** & POSIX shell (`bash` / `sh`)
- **Node.js 20+**: only required when previewing or building the VitePress documentation site under `docs/`

## Repository Structure

```text
cmd/
  aist/                 # CLI client entrypoint
  server/               # Server entrypoint (PocketBase Go framework)
internal/
  app/                  # Composition root: settings snapshot, runtime wiring, session factory
  auth/                 # Client auth manager & sliding token refresh
  buildinfo/            # Version, commit, and build date metadata
  cmd/                  # Cobra subcommands: login, logout, whoami, lang, sessions, version
  commandguard/         # Shell AST blacklist matcher & full-screen TUI blocker
  config/               # Server-delivered account Settings model, normalization & validation
  executor/             # Local & SSH command runners, PTY synchronization, output buffer
  fileedit/             # Structured file editing (unique match, SHA-256 CAS, local backups)
  i18n/                 # English & Simplified Chinese UI dictionaries
  interactive/          # User-facing turn loop, confirmations, history replay
  llm/                  # OpenAI-compatible client & logical ServerModel ("auto")
  memory/               # Local environment Memory domain, Markdown codec, catalog & search
  preferences/          # Client-local ~/.aist/preferences.json store & locale inference
  prompt/               # System prompts, Memory catalog & blacklist injection
  remote/               # HTTP client for PocketBase auth & /v1/proxy
  scriptexec/           # Scripted helper execution for file_edit
  server/               # Server API, register, proxy, priority router, cooldown, usage, audit, adminui
  session/              # Durable session domain, JSONL repository, token estimator, compaction planner
  state/                # ~/.aist path layout, 0700/0600 permissions & versioned backups
  storage/              # Atomic WriteFile / ReplaceFile primitives
  target/               # SSH / local Target validation
  termio/               # Terminal raw/PTY helpers
  token/                # ~/.aist/token.json store
  tools/                # Tool JSON schemas, argument parsers & language-neutral ToolDisplay
  ui/                   # Terminal prompts, Markdown rendering & spinners
  util/                 # Shared helpers (FirstNonEmpty, ExpandHome, TruncatePrefix, etc.)
migrations/             # PocketBase collection & settings migrations
frontend/               # Embedded static site (index.html, register.html, run.sh, uninstall.sh)
deploy/server/          # Dockerfile, compose.yaml, docker-entrypoint.sh, fly.toml
scripts/                # Release build, E2E test suites & naming-check.sh
docs/                   # Bilingual VitePress documentation site
```

## Building & Running Locally

```bash
make build          # Build all client (7 targets) + server (linux/amd64, arm64) release bundles into dist/
make build-client   # Build client release bundles only
make build-server   # Build server release bundles only
make dev            # Run server locally in dev mode (:8090, loads .env.local if present)
```

Override version metadata when testing release scripts locally:

```bash
VERSION=v0.6.0 COMMIT=$(git rev-parse --short HEAD) BUILD_DATE=$(date -u +%F) make build-client
```

## Verification & Test Suites

Before submitting any pull request, all four checks must pass:

```bash
make format         # gofmt -w -s .
go vet ./...
make naming-check   # ./scripts/naming-check.sh (see Naming Conventions)
go test ./...       # or: make test
```

End-to-end integration suites:

```bash
make test-e2e       # Runs file_edit E2E + multi-target/SSH/confirmation E2E
make test-e2e-all   # Runs all Go unit tests + every automated E2E suite
```

*(Interactive real-TTY `sudo` / hardware MFA / live `Ctrl+C` verification remains a manual check on a terminal.)*

## Working on Documentation (`docs/`)

```bash
npm --prefix docs ci
npm --prefix docs run dev     # Local preview with hot reload
npm --prefix docs run build   # Production build + dead-link verification
```

## Contributor License Agreement (CLA)

All code and documentation contributions are covered by the project's [Contributor License Agreement](https://github.com/llaoj/aist/blob/main/CLA.md). On your first pull request, our automated workflow will guide you to sign by posting a one-line comment.
