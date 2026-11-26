package server

import (
	"encoding/json"
	"testing"
)

func TestRewriteForwardBodyInjectsGeminiThoughtSignatures(t *testing.T) {
	reqBody := []byte(`{
		"model": "auto",
		"messages": [
			{"role": "user", "content": "check ceph"},
			{
				"role": "assistant",
				"content": null,
				"tool_calls": [
					{
						"id": "call_1",
						"type": "function",
						"function": {"name": "shell_execute", "arguments": "{\"command\":\"ceph -s\"}"}
					},
					{
						"id": "call_2",
						"type": "function",
						"function": {"name": "shell_execute", "arguments": "{\"command\":\"hostname\"}"},
						"extra_content": {
							"google": {"thought_signature": "real-sig-abc", "other": 1},
							"custom": true
						}
					}
				]
			},
			{"role": "tool", "tool_call_id": "call_1", "content": "ok"}
		]
	}`)

	rewritten, err := rewriteForwardBody(reqBody, "https://generativelanguage.googleapis.com/v1beta/openai", "gemini-3.8-flash", 8192, nil)
	if err != nil {
		t.Fatalf("rewriteForwardBody: %v", err)
	}

	var payload struct {
		Model     string `json:"model"`
		MaxTokens int    `json:"max_tokens"`
		Messages  []struct {
			Role      string `json:"role"`
			ToolCalls []struct {
				ID           string         `json:"id"`
				ExtraContent map[string]any `json:"extra_content"`
			} `json:"tool_calls"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(rewritten, &payload); err != nil {
		t.Fatalf("unmarshal rewritten: %v", err)
	}
	if payload.Model != "gemini-3.8-flash" || payload.MaxTokens != 8192 {
		t.Fatalf("model=%q max_tokens=%d", payload.Model, payload.MaxTokens)
	}
	calls := payload.Messages[1].ToolCalls
	if len(calls) != 2 {
		t.Fatalf("tool_calls len = %d, want 2", len(calls))
	}

	google0, _ := calls[0].ExtraContent["google"].(map[string]any)
	if got := google0["thought_signature"]; got != geminiSkipThoughtSignature {
		t.Fatalf("call_1 thought_signature = %v, want %q", got, geminiSkipThoughtSignature)
	}

	google1, _ := calls[1].ExtraContent["google"].(map[string]any)
	if got := google1["thought_signature"]; got != "real-sig-abc" {
		t.Fatalf("call_2 thought_signature = %v, want real-sig-abc", got)
	}
	if got := google1["other"]; got != float64(1) {
		t.Fatalf("call_2 google.other = %v, want 1", got)
	}
	if got := calls[1].ExtraContent["custom"]; got != true {
		t.Fatalf("call_2 extra_content.custom = %v, want true", got)
	}
}

func TestRewriteForwardBodySkipsNonGeminiUpstreams(t *testing.T) {
	reqBody := []byte(`{
		"model": "auto",
		"messages": [
			{
				"role": "assistant",
				"content": null,
				"tool_calls": [
					{
						"id": "call_1",
						"type": "function",
						"function": {"name": "shell_execute", "arguments": "{\"command\":\"uptime\"}"}
					}
				]
			}
		]
	}`)

	rewritten, err := rewriteForwardBody(reqBody, "https://api.deepseek.com/v1", "deepseek-chat", 4096, nil)
	if err != nil {
		t.Fatalf("rewriteForwardBody: %v", err)
	}

	var payload struct {
		Messages []struct {
			ToolCalls []map[string]any `json:"tool_calls"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(rewritten, &payload); err != nil {
		t.Fatalf("unmarshal rewritten: %v", err)
	}
	if _, exists := payload.Messages[0].ToolCalls[0]["extra_content"]; exists {
		t.Fatalf("non-gemini upstream should not receive extra_content: %s", string(rewritten))
	}
}

func TestIsGoogleGeminiUpstream(t *testing.T) {
	tests := []struct {
		baseURL string
		model   string
		want    bool
	}{
		{"https://generativelanguage.googleapis.com/v1beta/openai", "gemini-3.8-flash", true},
		{"https://us-central1-aiplatform.googleapis.com/v1beta1/projects/p/endpoints/openapi", "custom-model", true},
		{"https://proxy.example.com/v1", "gemini-3.5-flash-lite", true},
		{"https://api.deepseek.com/v1", "deepseek-chat", false},
		{"https://open.bigmodel.cn/api/paas/v4", "glm-5.3-flash", false},
		{"https://api-inference.modelscope.cn/v1", "deepseek-ai/DeepSeek-V3.2", false},
	}
	for _, tt := range tests {
		if got := isGoogleGeminiUpstream(tt.baseURL, tt.model); got != tt.want {
			t.Fatalf("isGoogleGeminiUpstream(%q, %q) = %v, want %v", tt.baseURL, tt.model, got, tt.want)
		}
	}
}

func TestRewriteForwardBodyMergesExtraBodyAndProtectsCoreFields(t *testing.T) {
	reqBody := []byte(`{
		"model": "auto",
		"messages": [{"role": "user", "content": "check disk"}],
		"max_tokens": 16384
	}`)
	extraBody, err := decodeModelExtraBody(`{
		"thinking": {"type": "disabled"},
		"enable_thinking": false,
		"model": "hijacked-model",
		"messages": [{"role": "user", "content": "hijacked"}]
	}`)
	if err != nil {
		t.Fatalf("decodeModelExtraBody: %v", err)
	}

	rewritten, err := rewriteForwardBody(reqBody, "https://api.deepseek.com/v1", "deepseek-flash", 8192, extraBody)
	if err != nil {
		t.Fatalf("rewriteForwardBody: %v", err)
	}

	var payload struct {
		Model          string `json:"model"`
		MaxTokens      int    `json:"max_tokens"`
		EnableThinking *bool  `json:"enable_thinking"`
		Thinking       struct {
			Type string `json:"type"`
		} `json:"thinking"`
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(rewritten, &payload); err != nil {
		t.Fatalf("unmarshal rewritten: %v", err)
	}
	if payload.Model != "deepseek-flash" {
		t.Fatalf("model = %q, want deepseek-flash", payload.Model)
	}
	if payload.MaxTokens != 8192 {
		t.Fatalf("max_tokens = %d, want 8192", payload.MaxTokens)
	}
	if payload.Thinking.Type != "disabled" {
		t.Fatalf("thinking.type = %q, want disabled", payload.Thinking.Type)
	}
	if payload.EnableThinking == nil || *payload.EnableThinking {
		t.Fatalf("enable_thinking = %v, want false", payload.EnableThinking)
	}
	if len(payload.Messages) != 1 || payload.Messages[0].Content != "check disk" {
		t.Fatalf("messages were overwritten: %+v", payload.Messages)
	}
}

func TestDecodeModelExtraBody(t *testing.T) {
	for _, empty := range []any{nil, "", "  ", "null", "{}"} {
		got, err := decodeModelExtraBody(empty)
		if err != nil || got != nil {
			t.Fatalf("decodeModelExtraBody(%v) = (%v, %v), want (nil, nil)", empty, got, err)
		}
	}
	for _, invalid := range []any{`["not-an-object"]`, `"string"`, `{bad json`} {
		if _, err := decodeModelExtraBody(invalid); err == nil {
			t.Fatalf("decodeModelExtraBody(%v) succeeded, want error", invalid)
		}
	}
}
