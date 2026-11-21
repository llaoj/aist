package interactive

import (
	"testing"

	"github.com/llaoj/aist/internal/llm"
)

func TestEnsureToolResultsRepairsHistoricalAndLatestBatches(t *testing.T) {
	input := []llm.ChatMessage{
		{Role: "system", Content: llm.StringPtr("sys")},
		{Role: "tool", ToolCallID: "orphan-leading", Content: llm.StringPtr("orphan")},
		{Role: "user", Content: llm.StringPtr("check proxy ports")},
		{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{
				{ID: "call-1", Type: "function", Function: llm.FunctionCall{Name: "shell_execute", Arguments: "{}"}},
				{ID: "call-2", Type: "function", Function: llm.FunctionCall{Name: "shell_execute", Arguments: "{}"}},
			},
		},
		{Role: "tool", ToolCallID: "call-1", Content: llm.StringPtr("rejected")},
		{Role: "tool", ToolCallID: "call-1", Content: llm.StringPtr("duplicate")},
		{Role: "tool", ToolCallID: "unknown-id", Content: llm.StringPtr("stray")},
		{Role: "user", Content: llm.StringPtr("clash is off, no sudo")},
		{Role: "assistant", Content: llm.StringPtr("Which app is running?")},
		{Role: "tool", ToolCallID: "orphan-middle", Content: llm.StringPtr("orphan")},
		{Role: "user", Content: llm.StringPtr("Now加速")},
	}

	got := EnsureToolResults(input)
	assertValidWireToolPairing(t, got)

	wantRoles := []string{"system", "user", "assistant", "tool", "tool", "user", "assistant", "user"}
	if len(got) != len(wantRoles) {
		t.Fatalf("len = %d, want %d: %+v", len(got), len(wantRoles), got)
	}
	for i, role := range wantRoles {
		if got[i].Role != role {
			t.Fatalf("msg[%d].Role = %q, want %q", i, got[i].Role, role)
		}
	}
	if got[3].ToolCallID != "call-1" || llm.DerefPtr(got[3].Content) != "rejected" {
		t.Fatalf("msg[3] = %+v", got[3])
	}
	if got[4].ToolCallID != "call-2" {
		t.Fatalf("msg[4] = %+v, want placeholder for call-2", got[4])
	}
}

func TestMergeAdjacentUserMessagesDoesNotMutateInput(t *testing.T) {
	first := "summary block"
	second := "first tail question"
	third := "继续"
	input := []llm.ChatMessage{
		{Role: "system", Content: &first},
		{Role: "user", Content: &first},
		{Role: "user", Content: &second},
		{Role: "assistant", Content: llm.StringPtr("ok")},
		{Role: "user", Content: llm.StringPtr("failed turn")},
		{Role: "user", Content: &third},
	}

	got := MergeAdjacentUserMessages(input)
	if first != "summary block" {
		t.Fatalf("input mutated: %q", first)
	}
	if len(got) != 4 {
		t.Fatalf("len = %d, want 4: %+v", len(got), got)
	}
	if want := "summary block\n\nfirst tail question"; llm.DerefPtr(got[1].Content) != want {
		t.Fatalf("got[1] = %q, want %q", llm.DerefPtr(got[1].Content), want)
	}
	if want := "failed turn\n\n继续"; llm.DerefPtr(got[3].Content) != want {
		t.Fatalf("got[3] = %q, want %q", llm.DerefPtr(got[3].Content), want)
	}
}

func assertValidWireToolPairing(t *testing.T, messages []llm.ChatMessage) {
	t.Helper()
	for i := 0; i < len(messages); i++ {
		msg := messages[i]
		if msg.Role == "tool" {
			t.Fatalf("orphan tool message at index %d: %+v", i, msg)
		}
		if msg.Role == "user" && i > 0 && messages[i-1].Role == "user" {
			t.Fatalf("consecutive user messages at %d and %d", i-1, i)
		}
		if msg.Role != "assistant" || len(msg.ToolCalls) == 0 {
			continue
		}
		expected := make(map[string]bool, len(msg.ToolCalls))
		for _, call := range msg.ToolCalls {
			expected[call.ID] = true
		}
		seen := make(map[string]bool, len(msg.ToolCalls))
		j := i + 1
		for k := 0; j < len(messages) && messages[j].Role == "tool"; j, k = j+1, k+1 {
			id := messages[j].ToolCallID
			if !expected[id] {
				t.Fatalf("unexpected tool_call_id %q at %d", id, j)
			}
			if seen[id] {
				t.Fatalf("duplicate tool_call_id %q at %d", id, j)
			}
			if k < len(msg.ToolCalls) && msg.ToolCalls[k].ID != id {
				t.Fatalf("tool result %d has id %q, want %q to match assistant tool_calls order", k, id, msg.ToolCalls[k].ID)
			}
			seen[id] = true
		}
		if len(seen) != len(expected) {
			t.Fatalf("assistant at %d has %d tool_calls, got %d tool results", i, len(expected), len(seen))
		}
		i = j - 1
	}
}
