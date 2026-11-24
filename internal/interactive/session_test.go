package interactive

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/llaoj/aist/internal/i18n"
	"github.com/llaoj/aist/internal/llm"
	"github.com/llaoj/aist/internal/memory"
	"github.com/llaoj/aist/internal/prompt"
	sessionpkg "github.com/llaoj/aist/internal/session"
	"github.com/llaoj/aist/internal/tools"
)

func TestTruncateContent(t *testing.T) {
	cases := []struct {
		name     string
		content  string
		maxChars int
		check    func(t *testing.T, out string)
	}{
		{
			name:     "short content returned as-is",
			content:  "hello",
			maxChars: 10,
			check: func(t *testing.T, out string) {
				if out != "hello" {
					t.Fatalf("got %q, want unchanged", out)
				}
			},
		},
		{
			name:     "non-positive max returns empty",
			content:  "hello",
			maxChars: 0,
			check: func(t *testing.T, out string) {
				if out != "" {
					t.Fatalf("got %q, want empty", out)
				}
			},
		},
		{
			name:     "tiny max hard cuts within the limit",
			content:  "abcdefghij",
			maxChars: 5,
			check: func(t *testing.T, out string) {
				if out != "abcde" {
					t.Fatalf("got %q, want %q", out, "abcde")
				}
			},
		},
		{
			name:     "long content keeps head, marker and tail",
			content:  strings.Repeat("a", 2000),
			maxChars: 200,
			check: func(t *testing.T, out string) {
				if !strings.Contains(out, "[omitted") {
					t.Fatalf("missing omission marker: %q", out)
				}
			},
		},
		{
			name:     "multibyte chinese is not corrupted",
			content:  strings.Repeat("服务器负载很高", 500),
			maxChars: 200,
			check: func(t *testing.T, out string) {
				if !utf8.ValidString(out) {
					t.Fatalf("truncation produced invalid UTF-8: %q", out)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.check(t, TruncateContent(tc.content, tc.maxChars))
		})
	}
}

type recordingScriptedModel struct {
	responses []*llm.CompletionResponse
	requests  []llm.CompletionRequest
	index     int
}

func (m *recordingScriptedModel) Call(_ context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	m.requests = append(m.requests, req)
	if m.index >= len(m.responses) {
		return &llm.CompletionResponse{Content: "done"}, nil
	}
	resp := m.responses[m.index]
	m.index++
	return resp, nil
}

func TestRunTurnForcesSummaryAtFollowUpLimitAndResumesCleanly(t *testing.T) {
	model := &recordingScriptedModel{
		responses: []*llm.CompletionResponse{
			{ToolCalls: []llm.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: llm.FunctionCall{
					Name:      tools.ToolMemorySearch,
					Arguments: `{"query":"web1"}`,
				},
			}}},
			{ToolCalls: []llm.ToolCall{{
				ID:   "call-2",
				Type: "function",
				Function: llm.FunctionCall{
					Name:      tools.ToolMemorySearch,
					Arguments: `{"query":"web2"}`,
				},
			}}},
			{Content: "Step limit reached after 2 searches; web3 remains."},
			{ToolCalls: []llm.ToolCall{{
				ID:   "call-3",
				Type: "function",
				Function: llm.FunctionCall{
					Name:      tools.ToolMemorySearch,
					Arguments: `{"query":"web3"}`,
				},
			}}},
			{Content: "All nodes verified."},
		},
	}
	recorder := &turnRecorder{}
	mem := &fakeMemory{searchResult: []memory.SearchResult{{
		Name:    "nodes",
		Type:    "infra",
		Path:    "/m/nodes.md",
		Content: "node info",
	}}}
	session := NewSession(SessionOptions{
		LLM:              model,
		Mode:             ModeInteractive,
		PromptSet:        staticPrompts{},
		MaxAutoToolCalls: 2,
		MemoryManager:    mem,
		Renderer:         recorder,
		Tools: []llm.ToolDefinition{{
			Name:        tools.ToolMemorySearch,
			Description: "search memory",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
	})

	if err := session.handleUserTurn("check cluster nodes"); err != nil {
		t.Fatalf("first turn: %v", err)
	}
	if len(model.requests) != 3 {
		t.Fatalf("first turn model calls = %d, want 3", len(model.requests))
	}
	if model.requests[0].ToolChoice != llm.ToolChoiceAuto || model.requests[1].ToolChoice != llm.ToolChoiceAuto {
		t.Fatalf("early tool_choice = (%q, %q), want auto", model.requests[0].ToolChoice, model.requests[1].ToolChoice)
	}

	finalReq := model.requests[2]
	if finalReq.ToolChoice != llm.ToolChoiceNone {
		t.Fatalf("final follow-up tool_choice = %q, want none", finalReq.ToolChoice)
	}
	if len(finalReq.Tools) != 1 {
		t.Fatalf("final follow-up tools = %+v, want tool definitions retained", finalReq.Tools)
	}
	lastMsg := finalReq.Messages[len(finalReq.Messages)-1]
	if lastMsg.Role != "user" || !strings.Contains(llm.DerefPtr(lastMsg.Content), "<system-reminder>") {
		t.Fatalf("final follow-up last message = %+v, want user system-reminder", lastMsg)
	}
	if recorder.lastAssistantText() != "Step limit reached after 2 searches; web3 remains." {
		t.Fatalf("assistant summary = %q", recorder.lastAssistantText())
	}
	if !recorder.hasNotice("Reached the consecutive unattended tool call limit (2 rounds)") {
		t.Fatalf("notices = %+v, want follow-up limit hint", recorder.notices)
	}
	if recorder.turnCompletions != 0 {
		t.Fatalf("turnCompletions = %d, want 0 when paused at limit", recorder.turnCompletions)
	}
	for _, msg := range session.sessionManager.ModelMessages() {
		if strings.Contains(msg.Content, "<system-reminder>") {
			t.Fatalf("temporary reminder leaked into persisted session history: %+v", msg)
		}
	}

	if err := session.handleUserTurn("continue"); err != nil {
		t.Fatalf("second turn: %v", err)
	}
	if len(model.requests) != 5 {
		t.Fatalf("total model calls after resume = %d, want 5", len(model.requests))
	}
	resumeReq := model.requests[3]
	if resumeReq.ToolChoice != llm.ToolChoiceAuto {
		t.Fatalf("resumed turn tool_choice = %q, want auto", resumeReq.ToolChoice)
	}
	for _, msg := range resumeReq.Messages {
		if strings.Contains(llm.DerefPtr(msg.Content), "<system-reminder>") {
			t.Fatalf("temporary reminder leaked into resumed turn request: %+v", msg)
		}
	}
	if recorder.lastAssistantText() != "All nodes verified." {
		t.Fatalf("final assistant text = %q", recorder.lastAssistantText())
	}
	if recorder.turnCompletions != 1 {
		t.Fatalf("turnCompletions = %d, want 1 after normal completion", recorder.turnCompletions)
	}
}

func TestUserConfirmedStepsResetAutoToolCalls(t *testing.T) {
	model := &recordingScriptedModel{
		responses: []*llm.CompletionResponse{
			// 1st auto step (count = 1)
			{ToolCalls: []llm.ToolCall{{
				ID:       "mem-1",
				Type:     "function",
				Function: llm.FunctionCall{Name: tools.ToolMemorySearch, Arguments: `{"query":"nginx"}`},
			}}},
			// User-confirmed shell step resets count -> 0
			{ToolCalls: []llm.ToolCall{shellCall("sh-1", 1, "local", "ss -s", "query")}},
			// Auto TUI guard block (count = 1)
			{ToolCalls: []llm.ToolCall{shellCall("sh-2", 1, "local", "top", "query")}},
			// User-confirmed shell step resets count -> 0
			{ToolCalls: []llm.ToolCall{shellCall("sh-3", 1, "local", "top -b -n 1", "query")}},
			// User-skipped shell step also counts as human interaction -> 0
			{ToolCalls: []llm.ToolCall{shellCall("sh-4", 1, "local", "awk '{print $1}' access.log", "query")}},
			// Another user-confirmed shell step -> 0 (total steps = 6, well above MaxAutoToolCalls = 2)
			{ToolCalls: []llm.ToolCall{shellCall("sh-5", 1, "local", "nginx -t", "query")}},
			{Content: "Attack mitigated cleanly."},
		},
	}
	recorder := &turnRecorder{}
	confirmer := &fakeConfirmer{
		decisions: []Decision{DecisionApprove, DecisionApprove, DecisionSkip, DecisionApprove},
	}
	executor := &fakeShellExecutor{}
	session := NewSession(SessionOptions{
		LLM:                  model,
		Mode:                 ModeInteractive,
		PromptSet:            staticPrompts{},
		MaxAutoToolCalls:     2,
		MemoryManager:        &fakeMemory{},
		CommandConfirmer:     confirmer,
		ShellCommandExecutor: executor,
		Renderer:             recorder,
	})

	if err := session.handleUserTurn("investigate nginx attack"); err != nil {
		t.Fatalf("handleUserTurn: %v", err)
	}
	if len(model.requests) != 7 {
		t.Fatalf("model calls = %d, want 7 without hitting unattended cap", len(model.requests))
	}
	for i, req := range model.requests {
		if req.ToolChoice != llm.ToolChoiceAuto {
			t.Fatalf("req[%d].ToolChoice = %q, want auto", i, req.ToolChoice)
		}
	}
	if len(executor.requests) != 3 {
		t.Fatalf("executed commands = %d, want 3 (approved steps only)", len(executor.requests))
	}
	if recorder.lastAssistantText() != "Attack mitigated cleanly." {
		t.Fatalf("final assistant text = %q", recorder.lastAssistantText())
	}
	if recorder.turnCompletions != 1 {
		t.Fatalf("turnCompletions = %d, want 1", recorder.turnCompletions)
	}
}

func TestRunTurnSkipsStrayToolCallsFromNonCompliantFinalResponse(t *testing.T) {
	model := &recordingScriptedModel{
		responses: []*llm.CompletionResponse{
			{ToolCalls: []llm.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: llm.FunctionCall{
					Name:      tools.ToolMemorySearch,
					Arguments: `{"query":"web1"}`,
				},
			}}},
			{
				Content: "Partial summary",
				ToolCalls: []llm.ToolCall{{
					ID:   "call-stray",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      tools.ToolMemorySearch,
						Arguments: `{"query":"should-not-run"}`,
					},
				}},
			},
		},
	}
	mem := &fakeMemory{}
	recorder := &turnRecorder{}
	session := NewSession(SessionOptions{
		LLM:              model,
		Mode:             ModeInteractive,
		PromptSet:        staticPrompts{},
		MaxAutoToolCalls: 1,
		MemoryManager:    mem,
		Renderer:         recorder,
	})

	if err := session.handleUserTurn("check"); err != nil {
		t.Fatalf("handleUserTurn: %v", err)
	}
	if len(model.requests) != 2 {
		t.Fatalf("model calls = %d, want 2 (must stop at limit)", len(model.requests))
	}
	if mem.searchQuery != "web1" {
		t.Fatalf("last executed searchQuery = %q, want web1 (stray tool call must not execute)", mem.searchQuery)
	}
	if status := lastToolStatus(t, session, "call-stray"); status != sessionpkg.ToolOutcomeSkipped {
		t.Fatalf("stray tool outcome = %q, want skipped", status)
	}
}

func TestHandleUserTurnPreservesFullAssistantContentAndTruncatesUserInput(t *testing.T) {
	longUser := strings.Repeat("u", MaxTurnChars+1500)
	longAssistant := strings.Repeat("a", MaxTurnChars+2500)

	model := &recordingScriptedModel{
		responses: []*llm.CompletionResponse{
			{Content: longAssistant},
		},
	}
	recorder := &turnRecorder{}
	session := NewSession(SessionOptions{
		LLM:      model,
		Mode:     ModeInteractive,
		Renderer: recorder,
	})

	if err := session.handleUserTurn(longUser); err != nil {
		t.Fatalf("handleUserTurn: %v", err)
	}

	messages := session.sessionManager.DisplayMessages()
	if len(messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(messages))
	}
	if !strings.Contains(messages[0].Content, "[omitted") {
		t.Fatalf("expected oversized user message to be truncated, got len=%d", len(messages[0].Content))
	}
	if messages[1].Content != longAssistant {
		t.Fatalf("assistant message len = %d, want full len %d", len(messages[1].Content), len(longAssistant))
	}
}

type interruptInput struct{}

func (interruptInput) ReadInput() (string, error) { return "", ErrInterrupted }

func TestInteractivePipedSessionRunsWithToolsAndDynamicSystemPrompt(t *testing.T) {
	model := &recordingScriptedModel{
		responses: []*llm.CompletionResponse{
			{ToolCalls: []llm.ToolCall{{
				ID:   "call-mem",
				Type: "function",
				Function: llm.FunctionCall{
					Name:      tools.ToolMemorySearch,
					Arguments: `{"query":"nginx"}`,
				},
			}}},
			{Content: "Checked piped nginx error and memory.", Model: "p/m"},
			{Content: "Follow-up in English.", Model: "p/m"},
		},
	}
	manager := sessionpkg.NewManager(sessionpkg.ManagerOptions{
		Settings: sessionpkg.Settings{CompactionEnabled: false},
	})
	if _, err := manager.Create(context.Background()); err != nil {
		t.Fatalf("Create: %v", err)
	}

	zhPrompts := prompt.NewBuilder(prompt.BuilderOptions{Language: "zh"})
	toolDefs := []llm.ToolDefinition{{
		Name:        tools.ToolMemorySearch,
		Description: "search memory",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}}

	liveRecorder := &historyRecorder{}
	sess := NewSession(SessionOptions{
		LLM:            model,
		SessionManager: manager,
		Translator:     i18n.New("zh"),
		Mode:           ModeInteractive,
		PromptSet:      zhPrompts,
		MemoryManager:  &fakeMemory{},
		PipeReader:     strings.NewReader("upstream timed out while reading response header"),
		InputReader:    interruptInput{},
		Renderer:       liveRecorder,
		Tools:          toolDefs,
	})
	if err := sess.Run("分析 nginx 502 超时"); err != nil {
		t.Fatalf("Run interactive pipe: %v", err)
	}

	if got := manager.Current().Title; got != "分析nginx502超时" {
		t.Fatalf("title = %q, want 分析nginx502超时 (must not include <piped_input>)", got)
	}
	if len(liveRecorder.users) != 1 || liveRecorder.users[0] != "分析 nginx 502 超时" {
		t.Fatalf("rendered user messages = %+v, want question only without raw <piped_input>", liveRecorder.users)
	}
	if !strings.Contains(strings.Join(liveRecorder.notices, "\n"), "已载入管道输入（1 行）") {
		t.Fatalf("notices = %+v, want pipe loaded line count", liveRecorder.notices)
	}
	for _, m := range manager.Current().Messages {
		if m.Role == sessionpkg.RoleSystem {
			t.Fatalf("system prompt must not be persisted in session messages: %+v", m)
		}
		if strings.Contains(m.Content, "Please respond in") || strings.Contains(m.Content, "<system-reminder>") {
			t.Fatalf("language instruction leaked into persisted message: %+v", m)
		}
	}
	if len(model.requests[0].Tools) != 1 {
		t.Fatalf("interactive pipe must include tools, got %+v", model.requests[0].Tools)
	}
	if !strings.Contains(llm.DerefPtr(model.requests[0].Messages[0].Content), "Please respond in Chinese") {
		t.Fatalf("expected dynamic Chinese prompt on first turn")
	}
	firstUserWire := llm.DerefPtr(model.requests[0].Messages[1].Content)
	if !strings.Contains(firstUserWire, "</piped_input>\n\n<system-reminder>Respond in Chinese (Simplified).") {
		t.Fatalf("expected Chinese tail reminder after </piped_input>, got:\n%s", firstUserWire)
	}

	enPrompts := prompt.NewBuilder(prompt.BuilderOptions{Language: "en"})
	resumed := NewSession(SessionOptions{
		LLM:            model,
		SessionManager: manager,
		Translator:     i18n.New("en"),
		Mode:           ModeInteractive,
		PromptSet:      enPrompts,
		InputReader:    interruptInput{},
		Renderer:       &turnRecorder{},
		Tools:          toolDefs,
	})
	if err := resumed.handleUserTurn("continue"); err != nil {
		t.Fatalf("resumed handleUserTurn: %v", err)
	}
	lastReq := model.requests[len(model.requests)-1]
	sysContent := llm.DerefPtr(lastReq.Messages[0].Content)
	if !strings.Contains(sysContent, "Please respond in English") || strings.Contains(sysContent, "Please respond in Chinese") {
		t.Fatalf("expected dynamic English prompt after language switch, got:\n%s", sysContent)
	}
	priorUserInLastReq := llm.DerefPtr(lastReq.Messages[1].Content)
	if strings.Contains(priorUserInLastReq, "<system-reminder>") {
		t.Fatalf("prior turn user message must not retain transient reminder:\n%s", priorUserInLastReq)
	}
	latestUserInLastReq := llm.DerefPtr(lastReq.Messages[len(lastReq.Messages)-1].Content)
	if !strings.HasSuffix(latestUserInLastReq, "<system-reminder>Respond in English.</system-reminder>") {
		t.Fatalf("latest user message missing English tail reminder:\n%s", latestUserInLastReq)
	}
}

type scriptedInput struct {
	inputs []string
	index  int
}

func (s *scriptedInput) ReadInput() (string, error) {
	if s.index >= len(s.inputs) {
		return "", ErrInterrupted
	}
	out := s.inputs[s.index]
	s.index++
	return out, nil
}

func TestInteractivePipeWithoutInitialQuestionWaitsForPromptInput(t *testing.T) {
	model := &recordingScriptedModel{
		responses: []*llm.CompletionResponse{
			{Content: "Added 8080 proxy block.", Model: "p/m"},
			{Content: "Updated proxy_read_timeout.", Model: "p/m"},
		},
	}

	emptyManager := sessionpkg.NewManager(sessionpkg.ManagerOptions{
		Settings: sessionpkg.Settings{CompactionEnabled: false},
	})
	_, _ = emptyManager.Create(context.Background())
	abortedRecorder := &turnRecorder{}
	aborted := NewSession(SessionOptions{
		LLM:            model,
		SessionManager: emptyManager,
		Translator:     i18n.New("zh"),
		Mode:           ModeInteractive,
		PromptSet:      staticPrompts{},
		PipeReader:     strings.NewReader("server {\n  listen 80;\n}\n"),
		InputReader:    interruptInput{},
		Renderer:       abortedRecorder,
	})
	if err := aborted.Run(""); err != nil {
		t.Fatalf("aborted Run: %v", err)
	}
	if len(model.requests) != 0 {
		t.Fatalf("expected 0 model calls when user exits before entering question, got %d", len(model.requests))
	}
	if len(emptyManager.Current().Messages) != 0 {
		t.Fatalf("expected 0 persisted messages on aborted pipe session, got %+v", emptyManager.Current().Messages)
	}
	if !abortedRecorder.hasNotice("已载入管道输入（3 行），请输入你的问题。") {
		t.Fatalf("notices = %+v, want pipe loaded notice", abortedRecorder.notices)
	}

	activeManager := sessionpkg.NewManager(sessionpkg.ManagerOptions{
		Settings: sessionpkg.Settings{CompactionEnabled: false},
	})
	_, _ = activeManager.Create(context.Background())
	active := NewSession(SessionOptions{
		LLM:            model,
		SessionManager: activeManager,
		Translator:     i18n.New("zh"),
		Mode:           ModeInteractive,
		PromptSet:      staticPrompts{},
		PipeReader:     strings.NewReader("server {\n  listen 80;\n}\n"),
		InputReader:    &scriptedInput{inputs: []string{"帮我加个 8080 反向代理", "再把超时改成 60s"}},
		Renderer:       &turnRecorder{},
	})
	if err := active.Run(""); err != nil {
		t.Fatalf("active Run: %v", err)
	}
	if len(model.requests) != 2 {
		t.Fatalf("model calls = %d, want 2", len(model.requests))
	}
	if got := activeManager.Current().Title; got != "帮我加个8080反向代理" {
		t.Fatalf("title = %q, want 帮我加个8080反向代理", got)
	}
	firstUser := llm.DerefPtr(model.requests[0].Messages[1].Content)
	if !strings.HasPrefix(firstUser, "帮我加个 8080 反向代理\n\n<piped_input>\nserver {") {
		t.Fatalf("first turn user message = %q", firstUser)
	}
	secondUser := llm.DerefPtr(model.requests[1].Messages[len(model.requests[1].Messages)-1].Content)
	if strings.Contains(secondUser, "<piped_input>") {
		t.Fatalf("second turn must not re-append piped input: %q", secondUser)
	}
}

func TestHeadlessPipeRequiresQuestionAndRunsWithoutTools(t *testing.T) {
	model := &recordingScriptedModel{
		responses: []*llm.CompletionResponse{
			{Content: "Found OOM killer event.", Model: "p/m"},
		},
	}
	manager := sessionpkg.NewManager(sessionpkg.ManagerOptions{
		Settings: sessionpkg.Settings{CompactionEnabled: false},
	})
	_, _ = manager.Create(context.Background())

	pipeRecorder := &turnRecorder{}
	pipeSession := NewSession(SessionOptions{
		LLM:            model,
		SessionManager: manager,
		Translator:     i18n.New("zh"),
		Mode:           ModePipe,
		PromptSet:      staticPrompts{},
		PipeReader:     strings.NewReader("kernel: Out of memory: Killed process 1234"),
		Renderer:       pipeRecorder,
		Tools: []llm.ToolDefinition{{
			Name:        tools.ToolShellExecute,
			Description: "shell",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
	})
	if err := pipeSession.RunPipeAnalysis(""); !errors.Is(err, ErrPipeQuestionRequired) {
		t.Fatalf("RunPipeAnalysis empty err = %v, want ErrPipeQuestionRequired", err)
	}
	if len(model.requests) != 0 {
		t.Fatalf("expected 0 model calls when question is missing, got %d", len(model.requests))
	}

	if err := pipeSession.RunPipeAnalysis("总结 OOM 原因"); err != nil {
		t.Fatalf("RunPipeAnalysis with question: %v", err)
	}
	if got := manager.Current().Title; got != "总结OOM原因" {
		t.Fatalf("title = %q, want 总结OOM原因", got)
	}
	if len(model.requests[0].Tools) != 0 {
		t.Fatalf("headless pipe request must omit tools, got %+v", model.requests[0].Tools)
	}
	if gotSys := llm.DerefPtr(model.requests[0].Messages[0].Content); gotSys != "system" {
		t.Fatalf("headless pipe system prompt = %q, want unified 'system'", gotSys)
	}
}
