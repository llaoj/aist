package interactive

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/llaoj/aist/internal/fileedit"
	"github.com/llaoj/aist/internal/llm"
	"github.com/llaoj/aist/internal/memory"
	sessionpkg "github.com/llaoj/aist/internal/session"
	"github.com/llaoj/aist/internal/tools"
)

type scriptedModel struct {
	responses []*llm.CompletionResponse
	index     int
}

func (m *scriptedModel) Call(context.Context, llm.CompletionRequest) (*llm.CompletionResponse, error) {
	return m.next(), nil
}

func (m *scriptedModel) next() *llm.CompletionResponse {
	if m.index >= len(m.responses) {
		return &llm.CompletionResponse{Content: "done"}
	}
	response := m.responses[m.index]
	m.index++
	return response
}

type turnRecorder struct {
	noopRenderer
	turnCompletions    int
	interruptedNotices int
	assistantTexts     []string
	notices            []string
}

func (r *turnRecorder) RenderTurnComplete() { r.turnCompletions++ }

func (r *turnRecorder) RenderInterruptedNotice() { r.interruptedNotices++ }

func (r *turnRecorder) RenderNotice(_ NoticeLevel, message string) {
	r.notices = append(r.notices, message)
}

func (r *turnRecorder) hasNotice(substring string) bool {
	for _, notice := range r.notices {
		if strings.Contains(notice, substring) {
			return true
		}
	}
	return false
}

func (r *turnRecorder) RenderAssistantMessage(content string) {
	r.assistantTexts = append(r.assistantTexts, content)
}

func (r *turnRecorder) lastAssistantText() string {
	if len(r.assistantTexts) == 0 {
		return ""
	}
	return r.assistantTexts[len(r.assistantTexts)-1]
}

type staticPrompts struct{}

func (staticPrompts) System() string           { return "system" }
func (staticPrompts) LanguageReminder() string { return "" }
func (staticPrompts) Compaction() string       { return "summary" }
func (staticPrompts) FollowUpLimit() string {
	return "<system-reminder>MAXIMUM STEPS REACHED</system-reminder>"
}

type fakeMemory struct {
	searchQuery  string
	searchResult []memory.SearchResult
	saveRequest  memory.ChangeRequest
	saveResult   memory.ChangeResult
	saveErr      error
}

func (f *fakeMemory) Search(_ context.Context, query string, _ int) ([]memory.SearchResult, error) {
	f.searchQuery = query
	return f.searchResult, nil
}

func (f *fakeMemory) Change(_ context.Context, req memory.ChangeRequest) (memory.ChangeResult, error) {
	f.saveRequest = req
	return f.saveResult, f.saveErr
}

func newMemorySession(t *testing.T, responses []*llm.CompletionResponse, mem MemoryManager) *Session {
	t.Helper()
	return NewSession(SessionOptions{
		LLM:           &scriptedModel{responses: responses},
		Mode:          ModeInteractive,
		PromptSet:     staticPrompts{},
		MemoryManager: mem,
	})
}

func TestMemorySearchToolFeedsResultBackToModel(t *testing.T) {
	mem := &fakeMemory{searchResult: []memory.SearchResult{{
		Name:      "prod-k8s",
		Type:      "k8s",
		Summary:   "prod Kubernetes cluster",
		Path:      "/m/prod-k8s.md",
		Content:   "- master: k8s-master-01 (10.0.0.1)\n- node-01: 10.0.0.2",
		Truncated: true,
	}}}
	session := newMemorySession(t, []*llm.CompletionResponse{
		{ToolCalls: []llm.ToolCall{{
			ID:   "call-1",
			Type: "function",
			Function: llm.FunctionCall{
				Name:      "memory_search",
				Arguments: `{"query":"10.0.0.1"}`,
			},
		}}},
		{Content: "The master is 10.0.0.1."},
	}, mem)

	if err := session.handleUserTurn("which host is 10.0.0.1?"); err != nil {
		t.Fatalf("handleUserTurn: %v", err)
	}
	if mem.searchQuery != "10.0.0.1" {
		t.Fatalf("query = %q", mem.searchQuery)
	}

	toolResult := findToolResult(t, ToLLMMessages(session.sessionManager.DisplayMessages()), "call-1")
	for _, want := range []string{
		`<memory_entry name="prod-k8s" type="k8s" path="/m/prod-k8s.md">`,
		"summary: prod Kubernetes cluster",
		"- master: k8s-master-01 (10.0.0.1)\n- node-01: 10.0.0.2",
		"[truncated: entry exceeds single-read limit;",
		"</memory_entry>",
	} {
		if !strings.Contains(toolResult, want) {
			t.Fatalf("tool result missing %q:\n%s", want, toolResult)
		}
	}
}

func TestMemorySaveToolReportsSuccess(t *testing.T) {
	mem := &fakeMemory{saveResult: memory.ChangeResult{
		Entry:     memory.Entry{Name: "prod-k8s", Summary: "Kubernetes cluster"},
		Operation: memory.ChangeOperationAppend,
		Action:    memory.ChangeActionCreated,
	}}
	recorder := &turnRecorder{}
	model := &scriptedModel{responses: []*llm.CompletionResponse{
		{ToolCalls: []llm.ToolCall{{
			ID:   "call-2",
			Type: "function",
			Function: llm.FunctionCall{
				Name:      "memory_save",
				Arguments: `{"name":"prod-k8s","content":"- master: 10.0.0.1","mode":"append"}`,
			},
		}}},
		{Content: "Saved."},
	}}
	session := NewSession(SessionOptions{
		LLM:           model,
		Mode:          ModeInteractive,
		PromptSet:     staticPrompts{},
		MemoryManager: mem,
		Renderer:      recorder,
	})

	if err := session.handleUserTurn("remember this"); err != nil {
		t.Fatalf("handleUserTurn: %v", err)
	}
	if model.index != 2 {
		t.Fatalf("model calls = %d, want 2 after successful memory save without text", model.index)
	}
	if recorder.turnCompletions != 1 {
		t.Fatalf("turn completions = %d, want 1", recorder.turnCompletions)
	}
	if mem.saveRequest.Name != "prod-k8s" || mem.saveRequest.Operation != memory.ChangeOperationAppend {
		t.Fatalf("save request = %+v", mem.saveRequest)
	}
	if result := findToolResult(t, ToLLMMessages(session.sessionManager.DisplayMessages()), "call-2"); !strings.Contains(result, "created successfully") {
		t.Fatalf("tool result = %q", result)
	}
}

func TestFileEditThenShellThenMemorySaveReachesFinalConclusion(t *testing.T) {
	editor := &fakeFileEditor{change: &fileedit.Change{
		Path:   "/tmp/antigravity-proxy",
		Diff:   "--- /tmp/antigravity-proxy\n+++ /tmp/antigravity-proxy\n",
		Reason: "write proxy launcher",
	}}
	mem := &fakeMemory{saveResult: memory.ChangeResult{
		Entry:     memory.Entry{Name: "mac-antigravity", Summary: "proxy config"},
		Operation: memory.ChangeOperationAppend,
		Action:    memory.ChangeActionCreated,
	}}
	recorder := &turnRecorder{}
	model := &scriptedModel{responses: []*llm.CompletionResponse{
		{ToolCalls: []llm.ToolCall{{
			ID:   "edit-1",
			Type: "function",
			Function: llm.FunctionCall{
				Name:      "file_edit",
				Arguments: `{"path":"/tmp/antigravity-proxy","old_content":"","new_content":"#!/bin/sh\n","reason":"write proxy launcher"}`,
			},
		}}},
		{ToolCalls: []llm.ToolCall{shellCall("shell-1", 1, "local", "chmod 755 /tmp/antigravity-proxy", "modify")}},
		{ToolCalls: []llm.ToolCall{{
			ID:   "save-1",
			Type: "function",
			Function: llm.FunctionCall{
				Name:      "memory_save",
				Arguments: `{"name":"mac-antigravity","content":"- proxy launcher: ~/.local/bin/antigravity-proxy","mode":"append"}`,
			},
		}}},
		{Content: "代理脚本已配置并验证通过。"},
	}}
	session := NewSession(SessionOptions{
		LLM:                  model,
		Mode:                 ModeInteractive,
		PromptSet:            staticPrompts{},
		FileEditor:           editor,
		FileEditConfirmer:    &fakeFilePrompter{confirm: true},
		MemoryManager:        mem,
		CommandConfirmer:     &fakeConfirmer{},
		ShellCommandExecutor: &fakeShellExecutor{},
		Renderer:             recorder,
	})

	if err := session.handleUserTurn("configure proxy"); err != nil {
		t.Fatalf("handleUserTurn: %v", err)
	}
	if model.index != 4 {
		t.Fatalf("model calls = %d, want 4 including final conclusion", model.index)
	}
	if recorder.turnCompletions != 1 {
		t.Fatalf("turn completions = %d, want 1", recorder.turnCompletions)
	}
	if got := recorder.lastAssistantText(); got != "代理脚本已配置并验证通过。" {
		t.Fatalf("final assistant text = %q", got)
	}
}

func TestSideEffectToolWithIntermediateTextContinuesUntilNoToolCalls(t *testing.T) {
	editor := &fakeFileEditor{change: &fileedit.Change{
		Path:   "/tmp/settings.json",
		Diff:   "--- /tmp/settings.json\n+++ /tmp/settings.json\n",
		Reason: "update proxy setting",
	}}
	mem := &fakeMemory{saveResult: memory.ChangeResult{
		Entry:     memory.Entry{Name: "mac-antigravity"},
		Operation: memory.ChangeOperationAppend,
		Action:    memory.ChangeActionCreated,
	}}
	recorder := &turnRecorder{}
	model := &scriptedModel{responses: []*llm.CompletionResponse{
		{
			Content: "先写入配置文件，随后验证并记录。",
			ToolCalls: []llm.ToolCall{{
				ID:   "edit-with-text",
				Type: "function",
				Function: llm.FunctionCall{
					Name:      "file_edit",
					Arguments: `{"path":"/tmp/settings.json","old_content":"{}","new_content":"{\"proxy\":true}","reason":"update proxy setting"}`,
				},
			}},
		},
		{ToolCalls: []llm.ToolCall{{
			ID:   "save-2",
			Type: "function",
			Function: llm.FunctionCall{
				Name:      "memory_save",
				Arguments: `{"name":"mac-antigravity","content":"- settings updated","mode":"append"}`,
			},
		}}},
		{Content: "已完成配置写入并记录到 Memory。"},
	}}
	session := NewSession(SessionOptions{
		LLM:               model,
		Mode:              ModeInteractive,
		PromptSet:         staticPrompts{},
		FileEditor:        editor,
		FileEditConfirmer: &fakeFilePrompter{confirm: true},
		MemoryManager:     mem,
		Renderer:          recorder,
	})

	if err := session.handleUserTurn("update settings"); err != nil {
		t.Fatalf("handleUserTurn: %v", err)
	}
	if model.index != 3 {
		t.Fatalf("model calls = %d, want 3", model.index)
	}
	if recorder.turnCompletions != 1 {
		t.Fatalf("turn completions = %d, want 1", recorder.turnCompletions)
	}
	if got := recorder.lastAssistantText(); got != "已完成配置写入并记录到 Memory。" {
		t.Fatalf("final assistant text = %q", got)
	}
}

func TestFailedCommandThenSideEffectFollowsUpForSummary(t *testing.T) {
	mem := &fakeMemory{saveResult: memory.ChangeResult{
		Entry:     memory.Entry{Name: "ecs-b1home"},
		Operation: memory.ChangeOperationAppend,
		Action:    memory.ChangeActionCreated,
	}}
	recorder := &turnRecorder{}
	model := &scriptedModel{responses: []*llm.CompletionResponse{
		{ToolCalls: []llm.ToolCall{shellCall("shell-1", 43, "web1", "x", "query")}},
		{ToolCalls: []llm.ToolCall{{
			ID:   "save-after-fail",
			Type: "function",
			Function: llm.FunctionCall{
				Name:      "memory_save",
				Arguments: `{"name":"ecs-b1home","content":"- docker stack","mode":"append"}`,
			},
		}}},
		{Content: "入侵并未成功，两个攻击 IP 均已被封禁。"},
	}}
	session := NewSession(SessionOptions{
		LLM:                  model,
		Mode:                 ModeInteractive,
		PromptSet:            staticPrompts{},
		MemoryManager:        mem,
		CommandConfirmer:     &fakeConfirmer{},
		ShellCommandExecutor: &fakeShellExecutor{err: errors.New("ssh target unreachable: web1")},
		Renderer:             recorder,
	})

	if err := session.handleUserTurn("confirm no intrusion"); err != nil {
		t.Fatalf("handleUserTurn: %v", err)
	}
	if model.index != 3 {
		t.Fatalf("model calls = %d, want 3 including the summary follow-up", model.index)
	}
	if got := recorder.lastAssistantText(); !strings.Contains(got, "入侵并未成功") {
		t.Fatalf("final assistant text = %q", got)
	}
}

func TestMemorySaveToolReportsFailure(t *testing.T) {
	mem := &fakeMemory{saveErr: memory.ErrSensitiveContent}
	session := newMemorySession(t, []*llm.CompletionResponse{
		{ToolCalls: []llm.ToolCall{{
			ID:   "call-3",
			Type: "function",
			Function: llm.FunctionCall{
				Name:      "memory_save",
				Arguments: `{"name":"prod","content":"- new","mode":"replace"}`,
			},
		}}},
		{Content: "Not changed."},
	}, mem)

	if err := session.handleUserTurn("replace memory"); err != nil {
		t.Fatalf("handleUserTurn: %v", err)
	}
	if model := session.llm.(*scriptedModel); model.index != 2 {
		t.Fatalf("model calls = %d, want 2 after failed memory save", model.index)
	}
	if status := lastToolStatus(t, session, "call-3"); status != sessionpkg.ToolOutcomeFailed {
		t.Fatalf("status = %q, want %q", status, sessionpkg.ToolOutcomeFailed)
	}
	if result := findToolResult(t, ToLLMMessages(session.sessionManager.DisplayMessages()), "call-3"); !strings.Contains(result, "No changes were written") {
		t.Fatalf("tool result = %q", result)
	}
}

type orderedMemory struct {
	events *[]string
}

func (m *orderedMemory) Search(_ context.Context, query string, _ int) ([]memory.SearchResult, error) {
	*m.events = append(*m.events, "memory_search:"+query)
	return nil, nil
}

func (m *orderedMemory) Change(_ context.Context, req memory.ChangeRequest) (memory.ChangeResult, error) {
	*m.events = append(*m.events, "memory_save:"+req.Name)
	return memory.ChangeResult{
		Entry:     memory.Entry{Name: req.Name},
		Operation: memory.ChangeOperationAppend,
		Action:    memory.ChangeActionCreated,
	}, nil
}

type orderedConfirmer struct {
	events   *[]string
	decision Decision
}

func (c *orderedConfirmer) ConfirmCommand(_ context.Context, cmdType tools.CommandType) (Decision, error) {
	*c.events = append(*c.events, "confirm_shell:"+cmdType.String())
	if c.decision != DecisionUnknown {
		return c.decision, nil
	}
	return DecisionApprove, nil
}

type orderedFilePrompter struct {
	events *[]string
}

func (p *orderedFilePrompter) ConfirmFileEdit(_ context.Context, change *fileedit.Change) (Decision, error) {
	*p.events = append(*p.events, "confirm_file:"+change.Path)
	return DecisionApprove, nil
}

func TestToolCallsExecuteInModelOrder(t *testing.T) {
	var events []string
	editor := &fakeFileEditor{change: &fileedit.Change{Path: "/tmp/app.conf", Diff: "diff"}}
	model := &strictValidatingModel{
		t: t,
		responses: []*llm.CompletionResponse{
			{ToolCalls: []llm.ToolCall{
				shellCall("shell-first", 1, "local", "df -h", "query"),
				{
					ID:   "search-mid",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "memory_search",
						Arguments: `{"query":"prod"}`,
					},
				},
				{
					ID:   "edit-mid",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "file_edit",
						Arguments: `{"path":"/tmp/app.conf","old_content":"a","new_content":"b","reason":"update"}`,
					},
				},
				{
					ID:   "save-last",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "memory_save",
						Arguments: `{"name":"prod-hosts","content":"- web1: 10.0.0.1","mode":"append"}`,
					},
				},
			}},
			{Content: "done"},
		},
	}
	session := NewSession(SessionOptions{
		LLM:                  model,
		Mode:                 ModeInteractive,
		PromptSet:            staticPrompts{},
		MemoryManager:        &orderedMemory{events: &events},
		CommandConfirmer:     &orderedConfirmer{events: &events},
		ShellCommandExecutor: &fakeShellExecutor{},
		FileEditor:           editor,
		FileEditConfirmer:    &orderedFilePrompter{events: &events},
	})

	if err := session.handleUserTurn("run all"); err != nil {
		t.Fatalf("handleUserTurn: %v", err)
	}

	wantOrder := []string{
		"confirm_shell:query",
		"memory_search:prod",
		"confirm_file:/tmp/app.conf",
		"memory_save:prod-hosts",
	}
	if len(events) != len(wantOrder) {
		t.Fatalf("events = %v, want %v", events, wantOrder)
	}
	for i, want := range wantOrder {
		if events[i] != want {
			t.Fatalf("events[%d] = %q, want %q (all=%v)", i, events[i], want, events)
		}
	}
}

func TestRejectRemainingSkipsSubsequentConfirmedToolsButRunsMemoryTools(t *testing.T) {
	var events []string
	editor := &fakeFileEditor{change: &fileedit.Change{Path: "/tmp/app.conf", Diff: "diff"}}
	recorder := &turnRecorder{}
	model := &strictValidatingModel{
		t: t,
		responses: []*llm.CompletionResponse{
			{ToolCalls: []llm.ToolCall{
				shellCall("shell-1", 1, "local", "sudo rm -rf /tmp/x", "modify"),
				{
					ID:   "search-after-reject",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "memory_search",
						Arguments: `{"query":"prod"}`,
					},
				},
				{
					ID:   "edit-after-reject",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "file_edit",
						Arguments: `{"path":"/tmp/app.conf","old_content":"a","new_content":"b","reason":"update"}`,
					},
				},
				{
					ID:   "save-after-reject",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "memory_save",
						Arguments: `{"name":"prod","content":"- host: web1","mode":"append"}`,
					},
				},
				shellCall("shell-2", 2, "local", "ls /tmp", "query"),
			}},
			{Content: "已收到补充说明。"},
		},
	}
	session := NewSession(SessionOptions{
		LLM:                  model,
		Mode:                 ModeInteractive,
		PromptSet:            staticPrompts{},
		MemoryManager:        &orderedMemory{events: &events},
		CommandConfirmer:     &orderedConfirmer{events: &events, decision: DecisionRejectRemaining},
		ShellCommandExecutor: &fakeShellExecutor{},
		FileEditor:           editor,
		FileEditConfirmer:    &orderedFilePrompter{events: &events},
		Renderer:             recorder,
	})

	if err := session.handleUserTurn("清理并记住主机"); err != nil {
		t.Fatalf("turn 1: %v", err)
	}
	wantOrder := []string{
		"confirm_shell:modify",
		"memory_search:prod",
		"memory_save:prod",
	}
	if len(events) != len(wantOrder) {
		t.Fatalf("events = %v, want %v", events, wantOrder)
	}
	for i, want := range wantOrder {
		if events[i] != want {
			t.Fatalf("events[%d] = %q, want %q (all=%v)", i, events[i], want, events)
		}
	}
	if editor.applied {
		t.Fatal("file_edit after reject remaining must not be applied")
	}
	if !recorder.hasNotice("add more details") || recorder.turnCompletions != 0 {
		t.Fatalf("notices=%v turnCompletions=%d", recorder.notices, recorder.turnCompletions)
	}
	if status := lastToolStatus(t, session, "shell-1"); status != sessionpkg.ToolOutcomeRejected {
		t.Fatalf("shell-1 status = %q, want %q", status, sessionpkg.ToolOutcomeRejected)
	}
	if status := lastToolStatus(t, session, "search-after-reject"); status != sessionpkg.ToolOutcomeSuccess {
		t.Fatalf("memory_search status = %q, want %q", status, sessionpkg.ToolOutcomeSuccess)
	}
	if status := lastToolStatus(t, session, "edit-after-reject"); status != sessionpkg.ToolOutcomeRejected {
		t.Fatalf("file_edit status = %q, want %q", status, sessionpkg.ToolOutcomeRejected)
	}
	if status := lastToolStatus(t, session, "save-after-reject"); status != sessionpkg.ToolOutcomeSuccess {
		t.Fatalf("memory_save status = %q, want %q", status, sessionpkg.ToolOutcomeSuccess)
	}
	if status := lastToolStatus(t, session, "shell-2"); status != sessionpkg.ToolOutcomeRejected {
		t.Fatalf("shell-2 status = %q, want %q", status, sessionpkg.ToolOutcomeRejected)
	}

	if err := session.handleUserTurn("不要删 /tmp/x"); err != nil {
		t.Fatalf("turn 2: %v", err)
	}
	if model.index != 2 {
		t.Fatalf("model calls = %d, want 2", model.index)
	}
}

func TestMixedMemorySaveAndShellKeepsSingleFollowUp(t *testing.T) {
	mem := &fakeMemory{saveResult: memory.ChangeResult{
		Entry:     memory.Entry{Name: "hosts"},
		Operation: memory.ChangeOperationAppend,
	}}
	model := &scriptedModel{responses: []*llm.CompletionResponse{
		{ToolCalls: []llm.ToolCall{
			{
				ID:   "mem-mixed",
				Type: "function",
				Function: llm.FunctionCall{
					Name:      "memory_save",
					Arguments: `{"name":"hosts","content":"- ecs-b1home","mode":"append"}`,
				},
			},
			shellCall("shell-mixed", 1, "local", "docker ps", "query"),
		}},
		{Content: "follow-up done"},
	}}
	session := NewSession(SessionOptions{
		LLM:                  model,
		Mode:                 ModeInteractive,
		PromptSet:            staticPrompts{},
		MemoryManager:        mem,
		CommandConfirmer:     &fakeConfirmer{},
		ShellCommandExecutor: &fakeShellExecutor{},
	})

	if err := session.handleUserTurn("check and remember"); err != nil {
		t.Fatalf("handleUserTurn: %v", err)
	}
	if model.index != 2 {
		t.Fatalf("model calls = %d, want 2 for mixed memory/shell tool calls", model.index)
	}
}

func findToolResult(t *testing.T, messages []llm.ChatMessage, callID string) string {
	t.Helper()
	for _, message := range messages {
		if message.Role == "tool" && message.ToolCallID == callID {
			return llm.DerefPtr(message.Content)
		}
	}
	t.Fatalf("tool result for %s not found: %+v", callID, messages)
	return ""
}
