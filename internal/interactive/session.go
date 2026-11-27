package interactive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/llaoj/aist/internal/commandguard"
	"github.com/llaoj/aist/internal/i18n"
	"github.com/llaoj/aist/internal/llm"
	"github.com/llaoj/aist/internal/session"
	"github.com/llaoj/aist/internal/tools"
)

// Mode represents the mode of the session.
type Mode string

const (
	ModeUnknown     Mode = ""
	ModeInteractive Mode = "interactive"
	ModePipe        Mode = "pipe"
)

// DefaultMaxAutoToolCalls caps consecutive tool-call rounds that run without
// user confirmation within one turn. Any user confirmation resets the counter.
const DefaultMaxAutoToolCalls = 15

// SessionOptions configures a Session.
type SessionOptions struct {
	LLM                  LLM
	Mode                 Mode
	PromptSet            PromptSet
	Version              string
	MaxAutoToolCalls     int
	CommandGuard         *commandguard.Guard
	MemoryManager        MemoryManager
	FileEditor           FileEditor
	FileEditConfirmer    FileEditConfirmer
	InputReader          InputReader
	Renderer             Renderer
	CommandConfirmer     CommandConfirmer
	ShellCommandExecutor ShellCommandExecutor
	PipeReader           io.Reader
	Tools                []llm.ToolDefinition
	// SessionManager is the durable session orchestrator. When nil, a private
	// in-memory manager is created so tests and library callers still work
	// without the application container.
	SessionManager *session.Manager
	// Translator localizes tool invocations, statuses and history markers.
	Translator tools.Translator
}

// Session orchestrates a conversation: it builds requests, records messages
// through the durable session manager, dispatches tool calls and delegates all
// presentation and execution to ports.
type Session struct {
	llm                  LLM
	sessionManager       *session.Manager
	translator           tools.Translator
	mode                 Mode
	promptSet            PromptSet
	version              string
	maxAutoToolCalls     int
	autoToolCalls        int
	commandGuard         *commandguard.Guard
	memoryManager        MemoryManager
	fileEditor           FileEditor
	fileEditConfirmer    FileEditConfirmer
	inputReader          InputReader
	renderer             Renderer
	commandConfirmer     CommandConfirmer
	shellCommandExecutor ShellCommandExecutor
	pipeReader           io.Reader
	pendingPipeInput     string
	tools                []llm.ToolDefinition
}

// NewSession creates a new session for the given mode.
func NewSession(opts SessionOptions) *Session {
	maxAutoToolCalls := opts.MaxAutoToolCalls
	if maxAutoToolCalls <= 0 {
		maxAutoToolCalls = DefaultMaxAutoToolCalls
	}
	s := &Session{
		llm:                  opts.LLM,
		sessionManager:       opts.SessionManager,
		translator:           opts.Translator,
		mode:                 opts.Mode,
		promptSet:            opts.PromptSet,
		version:              opts.Version,
		maxAutoToolCalls:     maxAutoToolCalls,
		commandGuard:         opts.CommandGuard,
		memoryManager:        opts.MemoryManager,
		fileEditor:           opts.FileEditor,
		fileEditConfirmer:    opts.FileEditConfirmer,
		inputReader:          opts.InputReader,
		renderer:             opts.Renderer,
		commandConfirmer:     opts.CommandConfirmer,
		shellCommandExecutor: opts.ShellCommandExecutor,
		pipeReader:           opts.PipeReader,
		tools:                opts.Tools,
	}
	if s.renderer == nil {
		s.renderer = noopRenderer{}
	}
	if s.translator == nil {
		s.translator = i18n.New("en")
	}
	if s.commandGuard == nil {
		s.commandGuard = commandguard.New(nil)
	}
	if s.sessionManager == nil {
		manager := session.NewManager(session.ManagerOptions{
			Settings: session.Settings{
				CompactionEnabled: false,
			},
		})
		_, _ = manager.Create(context.Background())
		s.sessionManager = manager
	}
	return s
}

// SetPipeReader attaches buffered piped stdin so an interactive or headless
// session can ingest piped input before reading terminal prompts.
func (s *Session) SetPipeReader(reader io.Reader) {
	s.pipeReader = reader
}

// Run starts the interactive session, processing optional piped input or an
// initial question before entering the interactive input loop.
func (s *Session) Run(initialQuestion string) error {
	s.renderWelcome()
	if s.pipeReader != nil {
		rawPipe, err := s.drainPipeReader()
		if err != nil {
			return err
		}
		trimmedQuestion := strings.TrimSpace(initialQuestion)
		lineCount := countPipeLines(rawPipe)
		if trimmedQuestion == "" {
			s.pendingPipeInput = rawPipe
			s.renderer.RenderNotice(NoticeLevelInfo, s.translator.T("pipe.input_loaded_prompt", lineCount))
			return s.runLoop()
		}

		ctx := context.Background()
		if err := s.sessionManager.SetDerivedTitle(ctx, trimmedQuestion); err != nil {
			return err
		}
		userMessage := buildPipedUserMessage(trimmedQuestion, rawPipe)
		s.renderer.RenderNotice(NoticeLevelInfo, s.translator.T("pipe.input_loaded", lineCount))
		s.renderer.RenderUserMessage(trimmedQuestion)
		if _, err := s.sessionManager.AppendUser(ctx, userMessage); err != nil {
			return err
		}
		if err := s.runTurn(ctx); err != nil {
			if errors.Is(err, ErrInterrupted) {
				return nil
			}
			return err
		}
		return s.runLoop()
	}

	if initialQuestion != "" {
		s.renderer.RenderUserMessage(initialQuestion)
		if err := s.handleUserTurn(initialQuestion); err != nil {
			if errors.Is(err, ErrInterrupted) {
				return nil
			}
			return err
		}
	}

	return s.runLoop()
}

// Resume starts an existing session, replays its local display history,
// then enters the interactive loop. It never calls the model for old messages.
func (s *Session) Resume() error {
	s.renderWelcome()
	s.ReplayHistory()
	return s.runLoop()
}

func (s *Session) renderWelcome() {
	s.renderer.RenderWelcome(s.version)
}

// RunPipeAnalysis runs a single headless read-only turn when stdout is
// redirected or no controlling terminal (/dev/tty) is available.
func (s *Session) RunPipeAnalysis(initialQuestion string) error {
	trimmedQuestion := strings.TrimSpace(initialQuestion)
	if trimmedQuestion == "" {
		return ErrPipeQuestionRequired
	}
	rawPipe, err := s.drainPipeReader()
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := s.sessionManager.SetDerivedTitle(ctx, trimmedQuestion); err != nil {
		return err
	}
	userMessage := buildPipedUserMessage(trimmedQuestion, rawPipe)
	if _, err := s.sessionManager.AppendUser(ctx, userMessage); err != nil {
		return err
	}
	if err := s.sessionManager.CompactIfNeeded(ctx); err != nil {
		return err
	}

	messages := s.buildRequestMessages(false)
	resp, err := s.llm.Call(ctx, llm.CompletionRequest{Messages: messages})
	if err != nil {
		return err
	}

	usage := usageFromLLM(resp.Usage)
	if _, err := s.sessionManager.AppendAssistant(ctx, resp.Content, nil, usage, resp.Model); err != nil {
		return err
	}
	if err := s.sessionManager.RecordResponseUsage(ctx, usage); err != nil {
		return err
	}
	_ = s.sessionManager.EnsureTitle(ctx)
	s.renderer.RenderAssistantMessage(resp.Content)
	s.renderer.Sync()
	return nil
}

func (s *Session) drainPipeReader() (string, error) {
	if s.pipeReader == nil {
		return "", errors.New("pipe reader is not configured")
	}
	pipeData, err := io.ReadAll(io.LimitReader(s.pipeReader, MaxPipeDataBytes))
	if err != nil {
		return "", err
	}
	s.pipeReader = nil
	return string(pipeData), nil
}

const emptyPipedInputPlaceholder = "(empty input)"

func countPipeLines(rawPipe string) int {
	trimmed := strings.TrimRight(rawPipe, "\r\n")
	if trimmed == "" || trimmed == emptyPipedInputPlaceholder {
		return 0
	}
	return strings.Count(trimmed, "\n") + 1
}

func buildPipedUserMessage(question, rawPipe string) string {
	pipeBody := TruncateContent(strings.TrimRight(rawPipe, "\r\n"), MaxTurnChars*MaxPipeInputMessages)
	if strings.TrimSpace(pipeBody) == "" {
		pipeBody = emptyPipedInputPlaceholder
	}
	return fmt.Sprintf("%s\n\n<piped_input>\n%s\n</piped_input>", TruncateContent(question, MaxTurnChars), pipeBody)
}

func splitPipedUserMessage(content string) (question, pipeBody string, hasPipe bool) {
	q, rest, found := strings.Cut(content, "\n\n<piped_input>\n")
	if !found || !strings.HasSuffix(rest, "\n</piped_input>") {
		return content, "", false
	}
	return q, strings.TrimSuffix(rest, "\n</piped_input>"), true
}

func (s *Session) runLoop() error {
	for {
		userInput, err := s.readUserInput()
		if err != nil {
			if errors.Is(err, ErrInterrupted) {
				return nil
			}
			s.renderer.RenderNotice(NoticeLevelError, RenderErrorMessage(s.translator, err))
			continue
		}

		if err := s.handleUserTurn(userInput); err != nil {
			if errors.Is(err, ErrInterrupted) {
				return nil
			}
			s.renderer.RenderNotice(NoticeLevelError, RenderErrorMessage(s.translator, err))
		}
	}
}

// handleUserTurn handles one user question and any tool loop it triggers.
func (s *Session) handleUserTurn(userInput string) error {
	ctx := context.Background()
	if s.pendingPipeInput != "" {
		rawPipe := s.pendingPipeInput
		s.pendingPipeInput = ""
		if err := s.sessionManager.SetDerivedTitle(ctx, userInput); err != nil {
			return err
		}
		if _, err := s.sessionManager.AppendUser(ctx, buildPipedUserMessage(userInput, rawPipe)); err != nil {
			return err
		}
		return s.runTurn(ctx)
	}
	if _, err := s.sessionManager.AppendUser(ctx, TruncateContent(userInput, MaxTurnChars)); err != nil {
		return err
	}
	return s.runTurn(ctx)
}

// runTurn drives the model/tool loop until the model stops requesting tools or
// reaches the consecutive auto tool-call limit.
func (s *Session) runTurn(ctx context.Context) error {
	toolsRan := false
	s.autoToolCalls = 0

	for {
		atAutoLimit := s.autoToolCalls >= s.maxAutoToolCalls
		resp, err := s.runModelStep(ctx, atAutoLimit)
		if err != nil || resp == nil {
			return err
		}
		if atAutoLimit {
			if len(resp.ToolCalls) > 0 {
				s.appendRemainingToolResults(ctx, resp.ToolCalls, followUpLimitToolResult, tools.ToolOutcomeSkipped)
			}
			s.renderer.RenderNotice(NoticeLevelWarning, s.translator.T("interactive.followup_limit_hint", s.maxAutoToolCalls))
			return nil
		}
		if len(resp.ToolCalls) == 0 {
			if toolsRan {
				s.renderer.RenderTurnComplete()
			}
			return nil
		}

		toolsRan = true
		s.autoToolCalls++
		rejectRemaining, err := s.runToolCalls(ctx, resp.ToolCalls)
		if err != nil {
			return err
		}
		if rejectRemaining {
			s.renderer.RenderNotice(NoticeLevelInfo, s.translator.T("interactive.reject_remaining_hint"))
			return nil
		}
	}
}

func (s *Session) runModelStep(ctx context.Context, isFinalFollowUp bool) (*llm.CompletionResponse, error) {
	if err := s.sessionManager.CompactIfNeeded(ctx); err != nil {
		return nil, s.handleModelCallError(err)
	}

	toolChoice := llm.ToolChoiceAuto
	if isFinalFollowUp {
		toolChoice = llm.ToolChoiceNone
	}

	messages := s.buildRequestMessages(isFinalFollowUp)
	resp, err := s.llm.Call(ctx, llm.CompletionRequest{
		Messages:    messages,
		Tools:       s.tools,
		ToolChoice:  toolChoice,
		Temperature: llm.Float64Ptr(0.0),
		TopP:        llm.Float64Ptr(0.2),
	})
	if err != nil {
		return nil, s.handleModelCallError(err)
	}

	usage := usageFromLLM(resp.Usage)
	if _, err := s.sessionManager.AppendAssistant(ctx, resp.Content, ToSessionToolCalls(resp.ToolCalls), usage, resp.Model); err != nil {
		return nil, err
	}
	if err := s.sessionManager.RecordResponseUsage(ctx, usage); err != nil {
		return nil, err
	}
	_ = s.sessionManager.EnsureTitle(ctx)
	s.renderer.RenderAssistantMessage(resp.Content)
	return resp, nil
}

func (s *Session) handleModelCallError(err error) error {
	if errors.Is(err, ErrInterrupted) {
		return ErrInterrupted
	}
	if errors.Is(err, context.Canceled) {
		s.renderer.RenderInterruptedNotice()
		return nil
	}
	return err
}

func (s *Session) buildRequestMessages(isFinalFollowUp bool) []llm.ChatMessage {
	raw := EnsureToolResults(ToLLMMessages(s.sessionManager.ModelMessages()))
	messages := make([]llm.ChatMessage, 0, len(raw)+2)
	if s.promptSet != nil {
		if sys := strings.TrimSpace(s.promptSet.System()); sys != "" {
			messages = append(messages, llm.ChatMessage{
				Role:    string(session.RoleSystem),
				Content: llm.StringPtr(sys),
			})
		}
	}
	messages = append(messages, raw...)
	messages = MergeAdjacentUserMessages(messages)
	if s.promptSet != nil {
		if langReminder := strings.TrimSpace(s.promptSet.LanguageReminder()); langReminder != "" {
			appendReminderToLastUserMessage(messages, langReminder)
		}
		if isFinalFollowUp {
			if reminder := strings.TrimSpace(s.promptSet.FollowUpLimit()); reminder != "" {
				messages = append(messages, llm.ChatMessage{
					Role:    "user",
					Content: llm.StringPtr(reminder),
				})
				messages = MergeAdjacentUserMessages(messages)
			}
		}
	}
	return messages
}

func appendReminderToLastUserMessage(messages []llm.ChatMessage, reminder string) {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != string(session.RoleUser) {
			continue
		}
		curr := llm.DerefPtr(messages[i].Content)
		if curr == "" {
			messages[i].Content = llm.StringPtr(reminder)
		} else {
			messages[i].Content = llm.StringPtr(curr + "\n\n" + reminder)
		}
		return
	}
}

func (s *Session) appendToolResult(ctx context.Context, callID, content string, outcome tools.ToolOutcome) {
	if s.sessionManager == nil {
		return
	}
	_, _ = s.sessionManager.AppendToolResult(ctx, callID, content, outcome)
}

func (s *Session) readUserInput() (string, error) {
	if s.inputReader == nil {
		return "", errors.New("input reader is not configured")
	}
	for {
		input, err := s.inputReader.ReadInput()
		if err != nil {
			return "", err
		}
		if input = strings.TrimSpace(input); input != "" {
			return input, nil
		}
	}
}
