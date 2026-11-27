//go:build gemini_live

// Live tests against the real Google Gemini API.
//
//	set -a; . ./.env.local; set +a
//	go test -tags gemini_live -run TestGeminiLive -v ./internal/server/
//
// GEMINI_API_KEY is required; GEMINI_MODEL defaults to gemini-3.5-flash-lite.
package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/llaoj/aist/internal/tools"
)

const geminiLiveBaseURL = "https://generativelanguage.googleapis.com/v1beta"

func geminiLiveCandidate(t *testing.T) (modelCandidate, string) {
	t.Helper()
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("GEMINI_API_KEY not set")
	}
	model := os.Getenv("GEMINI_MODEL")
	if model == "" {
		model = "gemini-3.5-flash-lite"
	}
	return modelCandidate{Protocol: upstreamProtocolGemini, Model: model, MaxOutputTokens: 4096}, apiKey
}

// geminiLiveCall sends one OpenAI-format request through the adapter to the
// real Gemini API and returns the normalized OpenAI response.
func geminiLiveCall(t *testing.T, candidate modelCandidate, apiKey string, body map[string]any) openAIChatResponse {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	adapter := geminiUpstreamAdapter{}
	req, err := adapter.BuildRequest(ctx, geminiLiveBaseURL, apiKey, candidate, raw)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("gemini status %d: %s", resp.StatusCode, summarizeUpstreamError(resp.StatusCode, respBody))
	}

	normalized, err := adapter.NormalizeResponse(candidate.Model, respBody)
	if err != nil {
		t.Fatalf("NormalizeResponse: %v", err)
	}
	var out openAIChatResponse
	if err := json.Unmarshal(normalized, &out); err != nil {
		t.Fatalf("unmarshal normalized: %v", err)
	}
	if len(out.Choices) != 1 {
		t.Fatalf("choices = %d", len(out.Choices))
	}
	return out
}

func geminiLiveTools() []map[string]any {
	def := tools.ShellExecuteDefinition()
	return []map[string]any{{
		"type": "function",
		"function": map[string]any{
			"name":        def.Name,
			"description": def.Description,
			"parameters":  json.RawMessage(def.InputSchema),
		},
	}}
}

func toolResult(id, content string) map[string]any {
	return map[string]any{"role": "tool", "tool_call_id": id, "content": content}
}

// Plain text turn with a system prompt.
func TestGeminiLiveText(t *testing.T) {
	candidate, apiKey := geminiLiveCandidate(t)
	out := geminiLiveCall(t, candidate, apiKey, map[string]any{
		"messages": []map[string]any{
			{"role": "system", "content": "Answer with a single word."},
			{"role": "user", "content": "Reply with the word OK."},
		},
	})
	msg := out.Choices[0].Message
	if msg.Content == nil || *msg.Content == "" {
		t.Fatalf("empty content, finish_reason=%s", out.Choices[0].FinishReason)
	}
	if out.Usage == nil || out.Usage.TotalTokens == 0 {
		t.Fatalf("usage missing: %+v", out.Usage)
	}
	t.Logf("model=%s content=%q finish=%s usage=%+v", out.Model, *msg.Content, out.Choices[0].FinishReason, *out.Usage)
}

// Real aist tool schema, Gemini-generated tool call, then replay with the
// returned thought signature and the tool result.
func TestGeminiLiveToolCallRoundTrip(t *testing.T) {
	candidate, apiKey := geminiLiveCandidate(t)
	messages := []map[string]any{
		{"role": "system", "content": "You are an ops assistant. Use shell_execute to run commands."},
		{"role": "user", "content": "Check the system uptime on the local machine."},
	}
	first := geminiLiveCall(t, candidate, apiKey, map[string]any{
		"messages":    messages,
		"tools":       geminiLiveTools(),
		"tool_choice": "required",
	})
	calls := first.Choices[0].Message.ToolCalls
	if len(calls) == 0 {
		t.Fatalf("expected tool calls, finish_reason=%s", first.Choices[0].FinishReason)
	}
	if first.Choices[0].FinishReason != "tool_calls" {
		t.Fatalf("finish_reason = %s", first.Choices[0].FinishReason)
	}
	t.Logf("tool calls=%d first=%s(%s) signed=%v", len(calls), calls[0].Function.Name, calls[0].Function.Arguments,
		extractGoogleThoughtSignature(calls[0].ExtraContent) != "")

	messages = append(messages, map[string]any{"role": "assistant", "content": nil, "tool_calls": calls})
	for _, call := range calls {
		messages = append(messages, toolResult(call.ID, "up 3 days, 4:12, load average: 0.10, 0.08, 0.05"))
	}
	second := geminiLiveCall(t, candidate, apiKey, map[string]any{
		"messages":    messages,
		"tools":       geminiLiveTools(),
		"tool_choice": "auto",
	})
	t.Logf("second turn: finish=%s content=%v tool_calls=%d", second.Choices[0].FinishReason,
		derefString(second.Choices[0].Message.Content), len(second.Choices[0].Message.ToolCalls))
}

// History produced by another model: parallel unsigned tool calls, tool
// results without "name", then the user's "继续" right after the results.
// This is the case that failed through Google's /v1beta/openai layer.
func TestGeminiLiveCrossModelHistoryWithInterruptedContinuation(t *testing.T) {
	candidate, apiKey := geminiLiveCandidate(t)
	out := geminiLiveCall(t, candidate, apiKey, map[string]any{
		"messages": []map[string]any{
			{"role": "system", "content": "You are an ops assistant."},
			{"role": "user", "content": "看看 ceph 状态"},
			{"role": "assistant", "content": nil, "tool_calls": []map[string]any{
				{"id": "call_a1", "type": "function", "function": map[string]any{"name": "shell_execute", "arguments": `{"step_index":1,"reason":"status","command":"ceph -s","command_type":"query"}`}},
				{"id": "call_a2", "type": "function", "function": map[string]any{"name": "shell_execute", "arguments": `{"step_index":2,"reason":"disk","command":"ceph df","command_type":"query"}`}},
			}},
			toolResult("call_a1", "health: HEALTH_WARN 1 osds down"),
			toolResult("call_a2", "RAW USED 62%"),
			{"role": "user", "content": "继续"},
		},
		"tools":       geminiLiveTools(),
		"tool_choice": "auto",
	})
	t.Logf("finish=%s content=%v tool_calls=%d", out.Choices[0].FinishReason,
		derefString(out.Choices[0].Message.Content), len(out.Choices[0].Message.ToolCalls))
}

// tool_choice "none" with tools still declared (aist's final follow-up step).
func TestGeminiLiveToolChoiceNone(t *testing.T) {
	candidate, apiKey := geminiLiveCandidate(t)
	out := geminiLiveCall(t, candidate, apiKey, map[string]any{
		"messages": []map[string]any{
			{"role": "user", "content": "Say hello in one short sentence."},
		},
		"tools":       geminiLiveTools(),
		"tool_choice": "none",
	})
	if len(out.Choices[0].Message.ToolCalls) != 0 {
		t.Fatalf("tool_choice none returned tool calls")
	}
	t.Logf("content=%v", derefString(out.Choices[0].Message.Content))
}

func derefString(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
