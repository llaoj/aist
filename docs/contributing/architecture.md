# Internal Design Deep Dive

This guide summarizes the internal architecture across the client and server packages for contributors modifying core behavior.

## 1. Startup & Composition Root (`internal/cmd` → `internal/app`)

1. **CLI Pre-Run (`internal/cmd/root.go`)**:
   - Stateless commands (`version`, `help`, `completion`, `__complete`) skip `~/.aist` initialization completely.
   - All other commands run `state.EnsureRoot("")` (`0700` dirs) and `preferences.Store.LoadOrInit()` (ensuring `~/.aist/preferences.json` exists with inferred `zh` or `en`; skipped only when `aist lang <zh|en>` is explicitly overwriting it).
   - Lightweight commands (`login`, `logout`, `whoami`, `lang`) stop before `app.New` (`whoami` queries weekly usage with a short timeout when logged in and degrades cleanly offline).
2. **Application Wiring (`internal/app/app.go`)**:
   - Ensures `*state.Root`, loads `preferences.Data`, refreshes auth via `account.Startup(ctx)` (`POST /api/collections/users/auth-refresh`), maps `remote.AccountConfig` → `config.Settings`, normalizes/validates, and constructs `i18n`, `llm.ServerModel`, `memory.Manager`, `prompt.Builder`, `commandguard.Guard`, `fileedit.Editor`, `executor.CommandRouter`, and `session.Repository`.

## 2. Prompt Layering (`internal/prompt` & `internal/tools`)

- **Unified English Base Prompt + Dynamic Runtime Language Injection**: A single unified `System` prompt covers both tool-enabled interactive turns and toolless headless turns. All static templates (`System`, `Compaction`, `FollowUpLimit`), tool schemas, and tool result headers (`Status`, `Target`, `Exit Code`, `Detail`) are authored in English for cross-model instruction stability; `prompt.Builder` dynamically injects Memory, command blacklists, and the active reply-language directive (`Chinese (Simplified)` vs `English`) from `preferences.json` at request time without persisting system prompts into session files (`Compaction` stays pure English).
- **Context Placeholders**:
  - `{{MEMORY}}`: replaced by `memory.CatalogProvider.PromptBlock()` (up to `32 KiB` of `Name`, `Type`, `Summary`, `Modified`; full entry text is fetched on demand via `memory_search`).
  - `{{BLACKLIST}}`: replaced by `<command_blacklist>` (up to `4 KiB` when non-empty; zero token overhead when empty).

## 3. Session Model, Token Estimation & Compaction (`internal/session`)

- **Language-Neutral Persistence**: `messages.jsonl` stores `ToolOutcome` (`success` / `failed` / `skipped` / `rejected`) and raw tool call JSON; `tools.DecodeInvocation` + `i18n` render historical tool lines dynamically in the active language during `aist sessions` replay.
- **Protocol Repair**: Before every model call, `ModelMessages()` pairs `assistant.tool_calls` with `role=tool` responses (synthesizing placeholder tool messages if interrupted mid-batch, dropping orphan/duplicate tool messages, and merging adjacent user messages).
- **Heuristic Token Estimator**: Estimates tokens without bundling large tokenizer tables by classifying messages into natural vs structured text (`>= 25%` symbol ratio or tool payloads) with per-character weights (ASCII letters, digits, punctuation, CJK), anchored to the latest upstream `usage.total_tokens` (`TokenAnchor`).
- **Turn-Boundary Compaction (`planner.go`)**: Groups messages by real `user` turns, preserves trailing turns up to `keep_tokens` (always keeping the latest turn), summarizes middle turns into `<compressed_history priority="high">`, and records a `compaction` event in `messages.jsonl`.

## 4. Tool Result Prefix-First Format & Server Audit (`internal/interactive` → `internal/server/audit.go`)

- Client `renderToolResult` places short structured metadata lines (`Status`, `Target`, `Exit Code`, `Error`, `Command` / `Path` / `Sudo`, `Detail`) **before** `[Output]`.
- On `POST /v1/proxy/chat/completions`, `recordToolAudit` inspects only `role=tool` messages after the final `assistant` message, pairs them by `tool_call_id`, truncates UTF-8 prefixes per field (`result` 6000 B, `command`/`old`/`new` 4000 B), and enforces a `14 KB` serialized JSON ceiling (`limitAuditAttrs`) so PocketBase's `16 KB` `MaxDataSize` never drops alphabetically trailing keys like `type`, `tool`, or `userIP`.

## 5. Server Candidate Routing (`internal/server/proxy.go` & `model_availability.go`)

1. Authenticates user via PocketBase `apis.RequireAuth("users")` (`e.Auth`).
2. Records tool audit logs once before candidate retries.
3. Enforces `weekly_token_quota` against UTC Monday `00:00` sum in `daily_model_usage`.
4. Loads enabled `allowed_models` whose parent `upstream_providers` are enabled, sorted by `(priority ASC, provider ASC, model ASC)`, skipping models blocked by `auth` or active `unavailable_until > now`.
5. Clamps `max_tokens` per candidate, forwards upstream, classifies errors (`quota`, `rate_limit`, `auth`, `400`, `5xx`), updates cooldown state or clears expired cooldowns on `2xx`, and atomically increments `daily_model_usage`.
