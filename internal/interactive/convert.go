package interactive

import (
	"fmt"

	"github.com/llaoj/aist/internal/llm"
	"github.com/llaoj/aist/internal/session"
)

// ToLLMMessages converts durable messages to the wire protocol shape.
func ToLLMMessages(messages []session.Message) []llm.ChatMessage {
	out := make([]llm.ChatMessage, 0, len(messages))
	for _, message := range messages {
		wire := llm.ChatMessage{
			Role:       string(message.Role),
			ToolCallID: message.ToolCallID,
			Name:       message.Name,
		}
		if message.Content != "" || message.Role != session.RoleAssistant || len(message.ToolCalls) == 0 {
			wire.Content = llm.StringPtr(message.Content)
		}
		if len(message.ToolCalls) > 0 {
			wire.ToolCalls = make([]llm.ToolCall, 0, len(message.ToolCalls))
			for _, call := range message.ToolCalls {
				wire.ToolCalls = append(wire.ToolCalls, llm.ToolCall{
					ID:   call.ID,
					Type: call.Type,
					Function: llm.FunctionCall{
						Name:      call.Function.Name,
						Arguments: call.Function.Arguments,
					},
				})
			}
		}
		out = append(out, wire)
	}
	return out
}

// ToSessionToolCalls converts wire tool calls to the durable domain shape.
func ToSessionToolCalls(calls []llm.ToolCall) []session.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]session.ToolCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, session.ToolCall{
			ID:   call.ID,
			Type: call.Type,
			Function: session.FunctionCall{
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			},
		})
	}
	return out
}

// ToSessionUsage converts provider usage to the durable session shape.
func ToSessionUsage(usage llm.Usage) session.Usage {
	return session.Usage{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
	}
}

func usageFromLLM(usage llm.Usage) *session.Usage {
	if usage.IsZero() {
		return nil
	}
	converted := ToSessionUsage(usage)
	return &converted
}

// EnsureToolResults repairs tool-call / tool-result pairing across the entire
// message slice: orphan or duplicate tool messages are dropped, and missing
// tool results receive placeholders so OpenAI-compatible upstreams never reject
// historical turns.
func EnsureToolResults(messages []llm.ChatMessage) []llm.ChatMessage {
	if len(messages) == 0 {
		return messages
	}

	out := make([]llm.ChatMessage, 0, len(messages))
	for idx := 0; idx < len(messages); idx++ {
		msg := messages[idx]
		if msg.Role == "tool" {
			continue
		}
		out = append(out, msg)
		if msg.Role != "assistant" || len(msg.ToolCalls) == 0 {
			continue
		}

		expected := make(map[string]bool, len(msg.ToolCalls))
		for _, call := range msg.ToolCalls {
			if call.ID != "" {
				expected[call.ID] = true
			}
		}

		seen := make(map[string]bool, len(msg.ToolCalls))
		nextIdx := idx + 1
		for ; nextIdx < len(messages) && messages[nextIdx].Role == "tool"; nextIdx++ {
			id := messages[nextIdx].ToolCallID
			if id == "" || !expected[id] || seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, messages[nextIdx])
		}

		for _, call := range msg.ToolCalls {
			if call.ID == "" || seen[call.ID] {
				continue
			}
			out = append(out, llm.ChatMessage{
				Role:       "tool",
				Content:    llm.StringPtr(fmt.Sprintf("Tool call '%s' was not executed. No result available.", call.ID)),
				ToolCallID: call.ID,
			})
		}
		idx = nextIdx - 1
	}
	return out
}

// MergeAdjacentUserMessages combines consecutive user messages so providers
// with strict user/assistant alternation do not reject requests after
// compaction, cancelled turns, or retries.
func MergeAdjacentUserMessages(messages []llm.ChatMessage) []llm.ChatMessage {
	if len(messages) < 2 {
		return messages
	}

	out := make([]llm.ChatMessage, 0, len(messages))
	for _, msg := range messages {
		if msg.Role == "user" && len(out) > 0 && out[len(out)-1].Role == "user" {
			prev := llm.DerefPtr(out[len(out)-1].Content)
			curr := llm.DerefPtr(msg.Content)
			merged := curr
			if prev != "" && curr != "" {
				merged = prev + "\n\n" + curr
			} else if prev != "" {
				merged = prev
			}
			out[len(out)-1].Content = llm.StringPtr(merged)
			continue
		}
		out = append(out, msg)
	}
	return out
}
