package interactive

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/llaoj/aist/internal/llm"
	sessionpkg "github.com/llaoj/aist/internal/session"
	"github.com/llaoj/aist/internal/tools"
)

type fakeShellExecutor struct {
	requests []ShellCommandRequest
	result   ShellCommandResult
	err      error
}

func (f *fakeShellExecutor) Execute(_ context.Context, req ShellCommandRequest) (ShellCommandResult, error) {
	f.requests = append(f.requests, req)
	result := f.result
	if result.Target == "" {
		result.Target = req.Target.String()
	}
	if result.Command == "" {
		result.Command = req.Command
	}
	if result.Output == "" {
		result.Output = "output for " + req.Target.String()
	}
	return result, f.err
}

type fakeConfirmer struct {
	requests  []ConfirmationRequest
	decisions []Decision
}

func (f *fakeConfirmer) ConfirmCommand(_ context.Context, req ConfirmationRequest) (Decision, error) {
	f.requests = append(f.requests, req)
	if len(f.decisions) == 0 {
		return DecisionApprove, nil
	}
	decision := f.decisions[0]
	f.decisions = f.decisions[1:]
	return decision, nil
}

func shellCall(id string, step int, target, command, commandType string) llm.ToolCall {
	return llm.ToolCall{
		ID:   id,
		Type: "function",
		Function: llm.FunctionCall{
			Name: tools.ToolShellExecute,
			Arguments: `{"step_index":` + itoa(step) + `,"reason":"check",` +
				`"command":"` + command + `","command_type":"` + commandType + `","target":"` + target + `"}`,
		},
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

func newShellSession(confirmer CommandConfirmer, executor ShellCommandExecutor, calls ...llm.ToolCall) *Session {
	return NewSession(SessionOptions{
		LLM: &scriptedModel{responses: []*llm.CompletionResponse{
			{ToolCalls: calls},
			{Content: "done"},
		}},
		Mode:                 ModeInteractive,
		PromptSet:            staticPrompts{},
		CommandConfirmer:     confirmer,
		ShellCommandExecutor: executor,
	})
}

func lastToolStatus(t *testing.T, s *Session, callID string) sessionpkg.ToolOutcome {
	t.Helper()
	for _, message := range s.sessionManager.DisplayMessages() {
		if message.Role == sessionpkg.RoleTool && message.ToolCallID == callID {
			return message.ToolOutcome
		}
	}
	t.Fatalf("tool status for %s not found", callID)
	return ""
}

func TestShellTargetIsExecutedAndTagged(t *testing.T) {
	confirmer := &fakeConfirmer{}
	executor := &fakeShellExecutor{result: ShellCommandResult{Output: "disk ok"}}
	session := newShellSession(confirmer, executor, shellCall("call-1", 1, "web1", "df -h", "query"))

	if err := session.handleUserTurn("check web1 disk"); err != nil {
		t.Fatalf("handleUserTurn: %v", err)
	}
	if len(executor.requests) != 1 || executor.requests[0].Target.Host() != "web1" {
		t.Fatalf("requests = %+v", executor.requests)
	}
	if len(confirmer.requests) != 1 || len(confirmer.requests[0].Targets) != 1 || confirmer.requests[0].Targets[0] != "web1" {
		t.Fatalf("confirmations = %+v", confirmer.requests)
	}
	if confirmer.requests[0].Invocation != "[web1] $ df -h" {
		t.Fatalf("confirmation display = %q", confirmer.requests[0].Invocation)
	}
	result := findToolResult(t, ToLLMMessages(session.sessionManager.DisplayMessages()), "call-1")
	for _, want := range []string{"Status: success\nTarget: web1\nExit Code: 0\nCommand: df -h\n\n[Output]\ndisk ok"} {
		if !strings.Contains(result, want) {
			t.Fatalf("result = %q, want %q", result, want)
		}
	}
	if status := lastToolStatus(t, session, "call-1"); status != sessionpkg.ToolOutcomeSuccess {
		t.Fatalf("tool status = %q", status)
	}
}

func TestShellInvalidTargetIsRejectedBeforeExecution(t *testing.T) {
	confirmer := &fakeConfirmer{}
	executor := &fakeShellExecutor{}
	session := newShellSession(confirmer, executor, shellCall("call-2", 1, "-oProxyCommand=evil", "df -h", "query"))

	if err := session.handleUserTurn("check disk"); err != nil {
		t.Fatalf("handleUserTurn: %v", err)
	}
	if len(executor.requests) != 0 || len(confirmer.requests) != 0 {
		t.Fatalf("unexpected execution/confirmation: %+v %+v", executor.requests, confirmer.requests)
	}
	result := findToolResult(t, ToLLMMessages(session.sessionManager.DisplayMessages()), "call-2")
	if !strings.Contains(result, "Invalid target") {
		t.Fatalf("result = %q", result)
	}
	if status := lastToolStatus(t, session, "call-2"); status != sessionpkg.ToolOutcomeSkipped {
		t.Fatalf("tool status = %q", status)
	}
}

func TestQueryCommandsRequireConfirmation(t *testing.T) {
	confirmer := &fakeConfirmer{}
	executor := &fakeShellExecutor{}
	session := newShellSession(confirmer, executor,
		shellCall("q1", 1, "web1", "df -h", "query"),
		shellCall("q2", 2, "web2", "free -m", "query"),
		shellCall("m1", 3, "web1", "systemctl restart nginx", "modify"),
	)

	if err := session.handleUserTurn("check both hosts and restart nginx"); err != nil {
		t.Fatalf("handleUserTurn: %v", err)
	}
	if len(executor.requests) != 3 {
		t.Fatalf("executions = %+v", executor.requests)
	}
	if len(confirmer.requests) != 3 {
		t.Fatalf("confirmations = %+v", confirmer.requests)
	}

	want := []struct {
		cmdType    tools.CommandType
		target     string
		invocation string
	}{
		{cmdType: tools.CommandTypeQuery, target: "web1", invocation: "[web1] $ df -h"},
		{cmdType: tools.CommandTypeQuery, target: "web2", invocation: "[web2] $ free -m"},
		{cmdType: tools.CommandTypeModify, target: "web1", invocation: "[web1] $ systemctl restart nginx"},
	}
	for i, item := range want {
		req := confirmer.requests[i]
		if req.Type != item.cmdType || req.UniqueTargets != 1 || len(req.Targets) != 1 || req.Targets[0] != item.target {
			t.Fatalf("confirmation %d = %+v", i, req)
		}
		if req.Invocation != item.invocation {
			t.Fatalf("confirmation %d invocation = %q, want %q", i, req.Invocation, item.invocation)
		}
	}
}

func TestQueryCommandSkipContinuesToNextQuery(t *testing.T) {
	confirmer := &fakeConfirmer{decisions: []Decision{DecisionSkip, DecisionApprove}}
	executor := &fakeShellExecutor{}
	session := newShellSession(confirmer, executor,
		shellCall("q1", 1, "web1", "df -h", "query"),
		shellCall("q2", 2, "web2", "free -m", "query"),
	)

	if err := session.handleUserTurn("check both hosts"); err != nil {
		t.Fatalf("handleUserTurn: %v", err)
	}
	if len(executor.requests) != 1 || executor.requests[0].Target.Host() != "web2" {
		t.Fatalf("executions = %+v", executor.requests)
	}
	if len(confirmer.requests) != 2 {
		t.Fatalf("confirmations = %+v", confirmer.requests)
	}
}

func TestShellNonZeroExitCodeSucceedsAndRecordsExitCode(t *testing.T) {
	confirmer := &fakeConfirmer{}
	renderer := &outputRecordingRenderer{}
	executor := &fakeShellExecutor{result: ShellCommandResult{ExitCode: 1, Output: "---"}}
	session := NewSession(SessionOptions{
		LLM: &scriptedModel{responses: []*llm.CompletionResponse{
			{ToolCalls: []llm.ToolCall{shellCall("call-grep", 1, "web1", "grep missing /etc/hosts", "query")}},
			{Content: "done"},
		}},
		Mode:                 ModeInteractive,
		PromptSet:            staticPrompts{},
		CommandConfirmer:     confirmer,
		ShellCommandExecutor: executor,
		Renderer:             renderer,
	})

	if err := session.handleUserTurn("search hosts"); err != nil {
		t.Fatalf("handleUserTurn: %v", err)
	}
	if status := lastToolStatus(t, session, "call-grep"); status != sessionpkg.ToolOutcomeSuccess {
		t.Fatalf("tool status = %q, want %q", status, sessionpkg.ToolOutcomeSuccess)
	}
	if renderer.finishedExitCode != 1 || renderer.finishedErr != nil {
		t.Fatalf("finished = (%d, %v), want (1, nil)", renderer.finishedExitCode, renderer.finishedErr)
	}
	result := findToolResult(t, ToLLMMessages(session.sessionManager.DisplayMessages()), "call-grep")
	if !strings.Contains(result, "Exit Code: 1") || strings.Contains(result, "Error:") {
		t.Fatalf("tool result = %q", result)
	}
}

func TestShellFailureReportsToolStatus(t *testing.T) {
	confirmer := &fakeConfirmer{}
	executor := &fakeShellExecutor{result: ShellCommandResult{ExitCode: 255}, err: errors.New("ssh target unreachable: web1")}
	session := newShellSession(confirmer, executor, shellCall("call-3", 1, "web1", "false", "query"))

	if err := session.handleUserTurn("run false"); err != nil {
		t.Fatalf("handleUserTurn: %v", err)
	}
	if status := lastToolStatus(t, session, "call-3"); status != sessionpkg.ToolOutcomeFailed {
		t.Fatalf("tool status = %q", status)
	}
	result := findToolResult(t, ToLLMMessages(session.sessionManager.DisplayMessages()), "call-3")
	want := "Status: failed\nTarget: web1\nError: ssh target unreachable: web1\nCommand: false\n\n[Output]\noutput for web1"
	if result != want {
		t.Fatalf("tool result = %q, want %q", result, want)
	}
}

type outputRecordingRenderer struct {
	noopRenderer
	outputs          []string
	finishedExitCode int
	finishedErr      error
}

func (r *outputRecordingRenderer) RenderExecutionOutput(text string) {
	r.outputs = append(r.outputs, text)
}

func (r *outputRecordingRenderer) RenderExecutionFinished(exitCode int, err error) {
	r.finishedExitCode = exitCode
	r.finishedErr = err
}

func TestStreamedPTYOutputIsNotRenderedTwice(t *testing.T) {
	renderer := &outputRecordingRenderer{}
	executor := &fakeShellExecutor{result: ShellCommandResult{
		Output:         "[sudo] password for ops:\nok\n",
		OutputStreamed: true,
	}}
	session := NewSession(SessionOptions{
		LLM: &scriptedModel{responses: []*llm.CompletionResponse{
			{ToolCalls: []llm.ToolCall{shellCall("call-pty", 1, "local", "sudo rm /usr/local/bin/atomcode", "modify")}},
			{Content: "done"},
		}},
		Mode:                 ModeInteractive,
		PromptSet:            staticPrompts{},
		CommandConfirmer:     &fakeConfirmer{},
		ShellCommandExecutor: executor,
		Renderer:             renderer,
	})

	if err := session.handleUserTurn("remove atomcode"); err != nil {
		t.Fatalf("handleUserTurn: %v", err)
	}
	if len(renderer.outputs) != 0 {
		t.Fatalf("streamed output rendered again: %q", renderer.outputs)
	}
	result := findToolResult(t, ToLLMMessages(session.sessionManager.DisplayMessages()), "call-pty")
	if !strings.Contains(result, "sudo rm /usr/local/bin/atomcode") || !strings.Contains(result, "ok") {
		t.Fatalf("tool result = %q", result)
	}
}

func TestFullScreenTUICommandsAreBlockedBeforeConfirmation(t *testing.T) {
	confirmer := &fakeConfirmer{}
	executor := &fakeShellExecutor{}
	session := newShellSession(confirmer, executor,
		shellCall("tui-edit", 1, "web1", "sudo vim /etc/nginx/nginx.conf", "modify"),
		shellCall("tui-pager", 2, "local", "journalctl -u nginx | less", "query"),
	)

	if err := session.handleUserTurn("edit config and inspect logs"); err != nil {
		t.Fatalf("handleUserTurn: %v", err)
	}
	if len(confirmer.requests) != 0 || len(executor.requests) != 0 {
		t.Fatalf("TUI commands must not reach confirmer or executor: %+v %+v", confirmer.requests, executor.requests)
	}

	messages := ToLLMMessages(session.sessionManager.DisplayMessages())
	editResult := findToolResult(t, messages, "tui-edit")
	if !strings.Contains(editResult, "file_edit") || !strings.Contains(editResult, "sudo=true") {
		t.Fatalf("edit result = %q", editResult)
	}
	pagerResult := findToolResult(t, messages, "tui-pager")
	if !strings.Contains(pagerResult, "--no-pager") {
		t.Fatalf("pager result = %q", pagerResult)
	}
	if status := lastToolStatus(t, session, "tui-edit"); status != sessionpkg.ToolOutcomeSkipped {
		t.Fatalf("tui-edit status = %q", status)
	}
	if status := lastToolStatus(t, session, "tui-pager"); status != sessionpkg.ToolOutcomeSkipped {
		t.Fatalf("tui-pager status = %q", status)
	}
}

type strictValidatingModel struct {
	t         *testing.T
	responses []*llm.CompletionResponse
	index     int
}

func (m *strictValidatingModel) Name() string { return "strict" }

func (m *strictValidatingModel) Call(_ context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	assertValidWireToolPairing(m.t, req.Messages)
	if m.index >= len(m.responses) {
		return &llm.CompletionResponse{Content: "ok"}, nil
	}
	resp := m.responses[m.index]
	m.index++
	return resp, nil
}

func TestRejectRemainingPersistsAllBatchResultsAndKeepsSubsequentTurnsValid(t *testing.T) {
	confirmer := &fakeConfirmer{decisions: []Decision{DecisionApprove, DecisionRejectRemaining}}
	executor := &fakeShellExecutor{}
	recorder := &turnRecorder{}
	model := &strictValidatingModel{
		t: t,
		responses: []*llm.CompletionResponse{
			{ToolCalls: []llm.ToolCall{
				shellCall("call-1", 1, "local", "netstat -anv", "query"),
				shellCall("call-2", 2, "local", "sudo lsof -nP", "query"),
				shellCall("call-3", 3, "local", "ps aux", "query"),
			}},
			{Content: "根据已经拿到的监听列表，请确认应用名称。"},
			{Content: "已定位到 Now加速。"},
		},
	}
	session := NewSession(SessionOptions{
		LLM:                  model,
		Mode:                 ModeInteractive,
		PromptSet:            staticPrompts{},
		CommandConfirmer:     confirmer,
		ShellCommandExecutor: executor,
		Renderer:             recorder,
	})

	if err := session.handleUserTurn("排查代理端口"); err != nil {
		t.Fatalf("turn 1: %v", err)
	}
	if !recorder.hasNotice("add more details") {
		t.Fatalf("reject hint not rendered: %v", recorder.notices)
	}
	if recorder.turnCompletions != 0 {
		t.Fatalf("turn completions = %d, want 0", recorder.turnCompletions)
	}
	if status := lastToolStatus(t, session, "call-2"); status != sessionpkg.ToolOutcomeRejected {
		t.Fatalf("call-2 status = %q", status)
	}
	if status := lastToolStatus(t, session, "call-3"); status != sessionpkg.ToolOutcomeRejected {
		t.Fatalf("call-3 status = %q", status)
	}

	if err := session.handleUserTurn("Clash 没开，不能用 sudo"); err != nil {
		t.Fatalf("turn 2: %v", err)
	}
	if err := session.handleUserTurn("Now加速"); err != nil {
		t.Fatalf("turn 3: %v", err)
	}
	if model.index != 3 {
		t.Fatalf("model calls = %d, want 3", model.index)
	}
}

func TestLegacySessionWithMissingToolResultsSelfHealsAcrossMultipleTurns(t *testing.T) {
	ctx := context.Background()
	manager := sessionpkg.NewManager(sessionpkg.ManagerOptions{
		Settings: sessionpkg.Settings{CompactionEnabled: false},
	})
	if _, err := manager.Create(ctx, "system"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, _ = manager.AppendUser(ctx, "initial question")
	_, _ = manager.AppendAssistant(ctx, "", []sessionpkg.ToolCall{
		{ID: "legacy-1", Type: "function", Function: sessionpkg.FunctionCall{Name: tools.ToolShellExecute, Arguments: `{"command":"id","target":"local"}`}},
		{ID: "legacy-2", Type: "function", Function: sessionpkg.FunctionCall{Name: tools.ToolShellExecute, Arguments: `{"command":"whoami","target":"local"}`}},
	}, nil, "p/m")
	_, _ = manager.AppendToolResult(ctx, "legacy-1", "rejected", sessionpkg.ToolOutcomeRejected)

	model := &strictValidatingModel{
		t: t,
		responses: []*llm.CompletionResponse{
			{Content: "first follow-up reply"},
			{Content: "second follow-up reply"},
		},
	}
	session := NewSession(SessionOptions{
		LLM:            model,
		SessionManager: manager,
		Mode:           ModeInteractive,
		PromptSet:      staticPrompts{},
	})

	if err := session.handleUserTurn("first intervention"); err != nil {
		t.Fatalf("first intervention: %v", err)
	}
	if err := session.handleUserTurn("second intervention"); err != nil {
		t.Fatalf("second intervention: %v", err)
	}
}
