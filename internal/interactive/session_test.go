package interactive

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/llaoj/aist/internal/llm"
	"github.com/llaoj/aist/internal/memory"
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
		LLM:           model,
		Mode:          ModeInteractive,
		PromptSet:     staticPrompts{},
		MaxFollowUps:  2,
		MemoryManager: mem,
		Renderer:      recorder,
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
	if !recorder.hasNotice("Reached the per-turn tool call limit (2 rounds)") {
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
		LLM:           model,
		Mode:          ModeInteractive,
		PromptSet:     staticPrompts{},
		MaxFollowUps:  1,
		MemoryManager: mem,
		Renderer:      recorder,
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
