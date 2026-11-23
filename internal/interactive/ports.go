package interactive

import (
	"context"
	"errors"
	"time"

	"github.com/llaoj/aist/internal/fileedit"
	"github.com/llaoj/aist/internal/llm"
	"github.com/llaoj/aist/internal/memory"
	"github.com/llaoj/aist/internal/target"
	"github.com/llaoj/aist/internal/tools"
)

// LLM is the conversation dependency. The concrete implementation is usually
// llm.ServerModel, but tests can provide a scripted model.
type LLM interface {
	Call(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error)
}

// InputReader reads one user message from the terminal. The implementation owns
// the prompt label and input widget.
type InputReader interface {
	ReadInput() (string, error)
}

// PromptSet supplies the runtime system, language, compaction and follow-up prompts.
type PromptSet interface {
	System() string
	LanguageReminder() string
	Compaction() string
	FollowUpLimit() string
}

// ShellCommandRequest is one approved command execution.
type ShellCommandRequest struct {
	Target  target.Target
	Command string
}

// ShellCommandResult is the outcome of one command execution.
type ShellCommandResult struct {
	Target         string
	Command        string
	Output         string
	OutputStreamed bool
	ExitCode       int
	Duration       time.Duration
}

// ShellCommandExecutor runs an approved shell command and returns combined output.
type ShellCommandExecutor interface {
	Execute(ctx context.Context, req ShellCommandRequest) (ShellCommandResult, error)
}

// Decision is the user's answer to a confirmation question.
type Decision int

const (
	DecisionUnknown Decision = iota
	DecisionApprove
	DecisionSkip
	DecisionRejectRemaining
)

// CommandConfirmer asks the user to approve a command. Implementations own the
// prompt wording; the session only receives the decision.
type CommandConfirmer interface {
	ConfirmCommand(ctx context.Context, cmdType tools.CommandType) (Decision, error)
}

// MemoryManager performs Memory search and save operations for tool calls.
type MemoryManager interface {
	Search(ctx context.Context, query string, limit int) ([]memory.SearchResult, error)
	Change(ctx context.Context, req memory.ChangeRequest) (memory.ChangeResult, error)
}

// FileEditor prepares and applies user-confirmed file changes.
type FileEditor interface {
	Prepare(ctx context.Context, req fileedit.EditRequest) (*fileedit.Change, error)
	Apply(ctx context.Context, change *fileedit.Change) error
}

// FileEditConfirmer asks the user before writing a prepared file change.
type FileEditConfirmer interface {
	ConfirmFileEdit(ctx context.Context, change *fileedit.Change) (Decision, error)
}

// NoticeLevel classifies short status messages.
type NoticeLevel int

const (
	NoticeLevelInfo NoticeLevel = iota
	NoticeLevelWarning
	NoticeLevelError
	NoticeLevelSuccess
)

// Renderer owns all terminal output produced by a session.
type Renderer interface {
	RenderWelcome(version string)
	RenderUserMessage(text string)
	RenderAssistantMessage(content string)
	RenderShellCommand(stepIndex string, cmd tools.ShellCommand)
	RenderFileChange(change *fileedit.Change)
	RenderExecutionOutput(text string)
	RenderExecutionOutputHeader(target string)
	RenderExecutionFinished(exitCode int, err error)
	RenderNotice(level NoticeLevel, message string)
	RenderInterruptedNotice()
	RenderBlacklistNotice(pattern string)
	RenderTurnComplete()
	RenderHistoryHeader(text string)
	RenderToolSummary(text string)
	Sync()
}

// ErrInterrupted means the user cancelled an input or confirmation prompt.
var ErrInterrupted = errors.New("interrupted by user")

// ErrPipeQuestionRequired means piped input was invoked without a controlling
// terminal and without an explicit question argument.
var ErrPipeQuestionRequired = errors.New("question is required in non-interactive pipe mode")
