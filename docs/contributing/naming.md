# Naming Conventions

This document is the single source of truth for domain vocabulary and identifier naming across Go code, tests, CLI commands, documentation, and commit messages.

AIST does **not** keep legacy naming aliases for internal backward compatibility — when a name changes, update all callers, tests, and docs directly, and verify with `make naming-check`.

## 1. Domain Vocabulary

| Concept | Canonical Name | Notes | Avoid |
|---|---|---|---|
| Persisted session | `session.Session` | Session data structure | `Conversation` as a type name |
| Session orchestrator | `session.Manager` | Append, compaction, title, pruning | `session.Service` |
| Interactive loop | `interactive.Session` | User-facing LLM/tool loop | `Agent`, `Runner` |
| Single user turn | `Turn` | One real `user` message + following `assistant`/`tool` messages | `Round`, `Question` |
| Persisted message | `Message` | Session message | `Entry` for messages |
| Tool invocation | `ToolCall` | Model-requested tool call | `Command` for generic tools |
| Tool result | `ToolResult` | Result written back to session | `Output` for generic tool results |
| Tool outcome | `ToolOutcome` | `success` / `failed` / `skipped` / `rejected` | `ResultStatus`, `ToolStatus` |
| Follow-up call | `FollowUp` | Model call triggered by tool results | `Analysis` |
| Shell command | `ShellCommand` | Parsed/pending shell command | `Invocation` as command struct |
| Query command | `CommandTypeQuery` | Read-only command | — |
| Modify command | `CommandTypeModify` | State-changing command | — |
| Execution host | `Target` | Local or SSH host | — |
| Memory entry | `memory.Entry` | Environment knowledge item | `memory.Memory` |
| Memory mutation | `Change` | Unified `append` / `replace` | `Save` for domain model |
| Account settings | `config.Settings` | Runtime snapshot delivered by server | `config.Config`, `remote.ServerConfig` |
| Memory catalog provider | `memory.CatalogProvider` | Builds `<memory_catalog>` block | `memory.Provider` |
| Prompt set | `prompt.Set` | System / follow-up / compaction prompts | `prompt.Provider` |
| Memory snapshot store | `memory.SnapshotStore` | In-memory loaded snapshot | `memory.Store` |
| Logical model client | `llm.ServerModel` | Sole client model entry (`model="auto"`) | `llm.Manager`, `CloudModel` |
| Terminal renderer | `Renderer` | Terminal rendering port | `Display` as a port/verb |

Terminology boundary: `user` refers strictly to login identity (`users`, `user_id`, `email`); `account` refers to the configuration + credentials aggregate (`AccountProvider`, `remote.AccountConfig`, `config.Settings`).

## 2. Go Naming Rules

- **General**: `PascalCase` for exported, `camelCase` for unexported; uppercase initialisms (`ID`, `URL`, `API`, `LLM`, `SSH`, `JSON`, `YAML`, `CLI`, `HTTP`); no underscores in Go identifiers (except test names); never stutter package names (`state.Root`, `config.Settings`, `memory.Entry`, `preferences.Data`).
- **Constructors**: `NewX` (`NewManager`, `NewStore`, `NewEditor`); bare `New` is allowed only when a package has a single primary type.
- **Conversions**: `toX` / `ToX` into a target domain type, `fromX` out of it.
- **Output Verbs**: `Render` (pure string formatting), `Print` (write to terminal), `Prompt` (interactive input), `Write` / `Save` (persist to file), `Append` / `Record` (append log/event), `Apply` (apply mutation). Never use `Display` as a verb.
- **Booleans**: Positive, specific names (`needsFollowUp`, `skipConfirm`, `IsSupportedLanguage`); avoid bare `handled`, `asked`, `stop`, `state`, `ok` across function boundaries.
- **Errors**: Sentinel errors `ErrX`, error types `XError`, wrap with `fmt.Errorf("...: %w", err)`.
- **Enums**: `<Type><Value>` (`CommandTypeUnknown`, `CommandTypeQuery`, `CommandTypeModify`); zero value must be safe/explicit (`Unknown`).
- **Query Methods**: Do **not** use `Get` prefixes (`LookupProvider`, `EffectiveContextWindow`, `EffectiveMaxOutputTokens`).
- **Struct / Interface Suffixes**: `Repository`, `Snapshot`, `Manager`, `<Type>Options` (`app.Options` is the sole bare `Options` where multiple types exist).
- **Package Boundaries**: Domain packages (`internal/config`, `internal/session`, `internal/memory`, `internal/target`) must not depend on UI, CLI, environment variables, or global state; adapters (`internal/preferences`, `internal/token`, `internal/session/file`) depend inward on domain packages.
- **Files & Packages**: Lowercase single-word package names (`fileedit`, `commandguard`, `preferences`); snake_case multi-word filenames (`account_config.go`); kebab-case shell scripts under `scripts/`.

## 3. Package-Specific Conventions

- `internal/config`: `Settings` holds only server-delivered `limits`, `session`, and `blacklist` (client UI/reply language lives in `internal/preferences`).
- `internal/llm`: `ServerModel` is the sole client LLM entry; `CompletionRequest` / `CompletionResponse`.
- `internal/interactive`: `Session`, `Turn`, `FollowUp`, `DecisionApprove` / `DecisionSkip` / `DecisionRejectRemaining`.

## 4. Prohibited Legacy Identifiers (`make naming-check`)

`./scripts/naming-check.sh` rejects all deprecated identifiers below:

```text
analyzeCommandOutput
needsAnalysis
AnalysisComplete
ContinueAnalysis
processQuestion
RunRestored
RunWithPipe
QueryCommand
ModifyCommand
ResultSuccess
ResultFailed
ResultSkipped
ResultRejected
ToolStatusSuccess
ToolStatusFailed
ToolStatusSkipped
ToolStatusRejected
SaveMode
SaveRequest
SaveResult
SaveAppend
SaveReplace
SaveDelete
Summarizer
SummaryRequest
SummaryResult
session.Service
memory.Service
memory.Provider
config.Provider
prompt.Provider
llm.Manager
CompactBlankLines
DisplayWelcome
DisplayShellCommand
EndRegion
clientConfig
clientLimits
buildClientConfig
remote.ServerConfig
CloudModel
CloudModelOptions
NewCloudModel
cmd/cloud
internal/cloud
deploy/cloud
scripts/build-cloud.sh
release-cloud
aist-cloud
AIST_CLOUD_*
```

## 5. Review Checklist

- Does every identifier directly describe behavior and match §1 vocabulary?
- Are enum zero values safe and boolean names positive/specific?
- Do output verbs follow `Render` / `Print` / `Prompt` / `Save`?
- Did `gofmt -l .`, `go vet ./...`, `make naming-check`, and `go test ./...` all pass?
