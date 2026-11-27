package server

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestGeminiModelIDRules(t *testing.T) {
	idCases := []struct {
		model string
		omit  bool
	}{
		{"gemini-1.5-flash", true},
		{"gemini-2.5-flash", true},
		{"gemini-live-2.5-flash", true},
		{"gemini-3.5-flash-lite", false},
		{"gemini-3.8-flash", false},
		{"gemma-3-27b-it", false},
	}
	for _, tc := range idCases {
		if got := omitsGeminiCallIDs(tc.model); got != tc.omit {
			t.Fatalf("omitsGeminiCallIDs(%q) = %v, want %v", tc.model, got, tc.omit)
		}
	}
}

func TestGeminiAdapterBuildRequestHandlesParallelToolsAndInterruptedContinuation(t *testing.T) {
	extraBody, err := decodeModelExtraBody(`{"reasoning_effort": "low"}`)
	if err != nil {
		t.Fatalf("decodeModelExtraBody: %v", err)
	}

	// Reproduces the exact multi-turn history:
	// 1. system prompt
	// 2. user question
	// 3. parallel unsigned tool_calls (from Zhipu/DeepSeek) + two tool results WITHOUT "name"
	// 4. immediate user "继续" without an assistant reply in between
	// 5. parallel tool_calls where call_3 already has a real signature
	openAIBody := []byte(`{
		"model": "auto",
		"messages": [
			{"role": "system", "content": "You are an ops expert."},
			{"role": "user", "content": "check ceph"},
			{
				"role": "assistant",
				"content": null,
				"tool_calls": [
					{"id": "call_1", "type": "function", "function": {"name": "shell_execute", "arguments": "{\"command\":\"ceph -s\"}"}},
					{"id": "call_2", "type": "function", "function": {"name": "shell_execute", "arguments": "{\"command\":\"df -h\"}"}}
				]
			},
			{"role": "tool", "tool_call_id": "call_1", "content": "HEALTH_OK"},
			{"role": "tool", "tool_call_id": "call_2", "content": "50%"},
			{"role": "user", "content": "继续"},
			{
				"role": "assistant",
				"content": "Checking config.",
				"tool_calls": [
					{
						"id": "call_3",
						"type": "function",
						"function": {"name": "shell_execute", "arguments": "{\"command\":\"ls /etc/ceph\"}"},
						"extra_content": {"google": {"thought_signature": "real-sig-xyz"}}
					},
					{
						"id": "call_4",
						"type": "function",
						"function": {"name": "shell_execute", "arguments": "{\"command\":\"date\"}"}}
				]
			},
			{"role": "tool", "tool_call_id": "call_3", "content": "ceph.conf"},
			{"role": "tool", "tool_call_id": "call_4", "content": "Fri"}
		],
		"tools": [
			{
				"type": "function",
				"function": {
					"name": "shell_execute",
					"description": "Run shell command",
					"parameters": {"type": "object", "properties": {"command": {"type": "string"}}}
				}
			}
		],
		"tool_choice": "auto",
		"max_tokens": 16000,
		"temperature": 0
	}`)

	candidate := modelCandidate{
		Protocol:        upstreamProtocolGemini,
		Model:           "gemini-3.5-flash-lite",
		MaxOutputTokens: 8192,
		ExtraBody:       extraBody,
	}

	adapter := selectUpstreamAdapter(candidate.Protocol)
	req, err := adapter.BuildRequest(
		context.Background(),
		"https://generativelanguage.googleapis.com/v1beta/",
		"goog-secret-key",
		candidate,
		openAIBody,
	)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	if wantURL := "https://generativelanguage.googleapis.com/v1beta/models/gemini-3.5-flash-lite:generateContent"; req.URL.String() != wantURL {
		t.Fatalf("URL = %q, want %q", req.URL.String(), wantURL)
	}
	if got := req.Header.Get("x-goog-api-key"); got != "goog-secret-key" {
		t.Fatalf("x-goog-api-key = %q", got)
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Fatalf("unexpected Authorization header %q", got)
	}

	sentBytes, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	var parsed geminiRequest
	if err := json.Unmarshal(sentBytes, &parsed); err != nil {
		t.Fatalf("unmarshal geminiRequest: %v\n%s", err, string(sentBytes))
	}
	if parsed.SystemInstruction == nil || len(parsed.SystemInstruction.Parts) != 1 || parsed.SystemInstruction.Parts[0].Text != "You are an ops expert." {
		t.Fatalf("systemInstruction = %+v", parsed.SystemInstruction)
	}

	// Contents: user, model(calls), user(functionResponses), user("继续"), model(text+calls), user(functionResponses).
	// "继续" must start its own user turn instead of being mixed into the functionResponse turn,
	// and no fabricated model reply is inserted.
	wantRoles := []string{"user", "model", "user", "user", "model", "user"}
	if len(parsed.Contents) != len(wantRoles) {
		t.Fatalf("contents len = %d, want %d\n%s", len(parsed.Contents), len(wantRoles), string(sentBytes))
	}
	for i, wantRole := range wantRoles {
		if parsed.Contents[i].Role != wantRole {
			t.Fatalf("contents[%d].Role = %q, want %q", i, parsed.Contents[i].Role, wantRole)
		}
	}

	// Turn 1 (index 1): unsigned parallel calls both receive skip_thought_signature_validator and preserve IDs on Gemini 3.
	turn1 := parsed.Contents[1].Parts
	if len(turn1) != 2 || turn1[0].ThoughtSignature != geminiSkipThoughtSignature || turn1[1].ThoughtSignature != geminiSkipThoughtSignature {
		t.Fatalf("unsigned parallel calls = %+v", turn1)
	}
	if turn1[0].FunctionCall.ID != "call_1" || turn1[1].FunctionCall.ID != "call_2" {
		t.Fatalf("gemini 3 call IDs = %q, %q", turn1[0].FunctionCall.ID, turn1[1].FunctionCall.ID)
	}

	// Turn 2 (index 2): parallel tool responses merged into ONE user turn with auto-resolved function name and IDs.
	turn2 := parsed.Contents[2].Parts
	if len(turn2) != 2 || turn2[0].FunctionResponse == nil || turn2[1].FunctionResponse == nil {
		t.Fatalf("merged functionResponse parts = %+v", turn2)
	}
	if turn2[0].FunctionResponse.Name != "shell_execute" || turn2[1].FunctionResponse.Name != "shell_execute" {
		t.Fatalf("resolved names = %q, %q", turn2[0].FunctionResponse.Name, turn2[1].FunctionResponse.Name)
	}
	if turn2[0].FunctionResponse.ID != "call_1" || turn2[1].FunctionResponse.ID != "call_2" {
		t.Fatalf("functionResponse IDs = %q, %q", turn2[0].FunctionResponse.ID, turn2[1].FunctionResponse.ID)
	}
	if got := turn2[0].FunctionResponse.Response["content"]; got != "HEALTH_OK" {
		t.Fatalf("functionResponse content = %v", got)
	}

	// Turn 3 (index 3): "继续" is a pure text user turn.
	if len(parsed.Contents[3].Parts) != 1 || parsed.Contents[3].Parts[0].Text != "继续" {
		t.Fatalf("continued user turn = %+v", parsed.Contents[3])
	}

	// Turn 4 (index 4): signed first call keeps real-sig-xyz, sibling call_4 stays unsigned.
	turn4 := parsed.Contents[4].Parts
	if len(turn4) != 3 { // text + 2 functionCalls
		t.Fatalf("turn4 parts len = %d, want 3", len(turn4))
	}
	if turn4[1].ThoughtSignature != "real-sig-xyz" || turn4[2].ThoughtSignature != "" {
		t.Fatalf("signed parallel calls = (%q, %q), want (real-sig-xyz, empty)", turn4[1].ThoughtSignature, turn4[2].ThoughtSignature)
	}

	// GenerationConfig & ToolConfig checks.
	if parsed.GenerationConfig == nil || parsed.GenerationConfig.MaxOutputTokens != 8192 {
		t.Fatalf("generationConfig = %+v", parsed.GenerationConfig)
	}
	if !strings.Contains(string(parsed.GenerationConfig.ThinkingConfig), `"thinkingLevel":"low"`) {
		t.Fatalf("thinkingConfig = %s", string(parsed.GenerationConfig.ThinkingConfig))
	}
	if parsed.ToolConfig == nil || parsed.ToolConfig.FunctionCallingConfig.Mode != "AUTO" {
		t.Fatalf("toolConfig = %+v", parsed.ToolConfig)
	}
}

func TestGeminiAdapterNormalizeResponseAndErrorFinishReason(t *testing.T) {
	adapter := geminiUpstreamAdapter{}

	geminiResp := []byte(`{
		"modelVersion": "gemini-3.5-flash-lite",
		"candidates": [
			{
				"content": {
					"role": "model",
					"parts": [
						{"text": "hidden chain of thought", "thought": true},
						{"text": "Let me check."},
						{
							"functionCall": {"id": "dup_id", "name": "shell_execute", "args": {"command": "uptime"}},
							"thoughtSignature": "sig-token-1"
						},
						{
							"functionCall": {"id": "dup_id", "name": "shell_execute", "args": {"command": "date"}}
						}
					]
				},
				"finishReason": "STOP"
			}
		],
		"usageMetadata": {
			"promptTokenCount": 100,
			"toolUsePromptTokenCount": 20,
			"candidatesTokenCount": 30,
			"thoughtsTokenCount": 50,
			"totalTokenCount": 200
		}
	}`)

	normalized, err := adapter.NormalizeResponse("gemini-3.5-flash-lite", geminiResp)
	if err != nil {
		t.Fatalf("NormalizeResponse: %v", err)
	}

	var out struct {
		Model   string `json:"model"`
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Role      string  `json:"role"`
				Content   *string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
					ExtraContent map[string]map[string]string `json:"extra_content"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(normalized, &out); err != nil {
		t.Fatalf("unmarshal normalized: %v", err)
	}
	msg := out.Choices[0].Message
	if msg.Content == nil || *msg.Content != "Let me check." {
		t.Fatalf("content = %v, want 'Let me check.' (thought filtered)", msg.Content)
	}
	if len(msg.ToolCalls) != 2 {
		t.Fatalf("tool_calls len = %d, want 2", len(msg.ToolCalls))
	}
	if msg.ToolCalls[0].ID != "dup_id" || msg.ToolCalls[1].ID == "dup_id" || msg.ToolCalls[1].ID == "" {
		t.Fatalf("duplicate ID was not deduplicated: %q vs %q", msg.ToolCalls[0].ID, msg.ToolCalls[1].ID)
	}
	if got := msg.ToolCalls[0].ExtraContent["google"]["thought_signature"]; got != "sig-token-1" {
		t.Fatalf("thought_signature = %q, want sig-token-1", got)
	}
	if out.Usage.PromptTokens != 120 || out.Usage.CompletionTokens != 80 || out.Usage.TotalTokens != 200 {
		t.Fatalf("usage = %+v, want 120/80/200", out.Usage)
	}

	// Recoverable/malformed finishReason must return an error so proxy fails over.
	malformed := []byte(`{"candidates":[{"finishReason":"MALFORMED_FUNCTION_CALL"}]}`)
	if _, err := adapter.NormalizeResponse("gemini-3.5-flash-lite", malformed); err == nil {
		t.Fatal("expected error on MALFORMED_FUNCTION_CALL")
	}

	// Blocked prompt without candidates is reported as a content-filter stop (OpenCode behavior).
	// An error object in a 2xx body is an upstream failure.
	errBody := []byte(`{"error":{"code":400,"message":"bad thing","status":"INVALID_ARGUMENT"}}`)
	if _, err := adapter.NormalizeResponse("gemini-3.5-flash-lite", errBody); err == nil || !strings.Contains(err.Error(), "bad thing") {
		t.Fatalf("expected error from error body, got %v", err)
	}
	if _, err := adapter.NormalizeResponse("gemini-3.5-flash-lite", []byte(`{}`)); err == nil {
		t.Fatal("expected error when there are no candidates and no block reason")
	}

	finishCases := []struct {
		body string
		want string
	}{
		{`{"candidates":[{"content":{"parts":[{"text":"cut"}]},"finishReason":"MAX_TOKENS"}]}`, "length"},
		{`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"shell_execute","args":{}}}]},"finishReason":"MAX_TOKENS"}]}`, "length"},
		{`{"candidates":[{"finishReason":"SAFETY"}]}`, "content_filter"},
		{`{"candidates":[{"finishReason":"MODEL_ARMOR"}]}`, "content_filter"},
		{`{"promptFeedback":{"blockReason":"SAFETY"}}`, "content_filter"},
		{`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"shell_execute"}}]},"finishReason":"STOP"}]}`, "tool_calls"},
		{`{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`, "stop"},
	}
	for _, tc := range finishCases {
		normalized, err := adapter.NormalizeResponse("gemini-3.5-flash-lite", []byte(tc.body))
		if err != nil {
			t.Fatalf("NormalizeResponse(%s): %v", tc.body, err)
		}
		var resp struct {
			Choices []struct {
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(normalized, &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.Choices[0].FinishReason != tc.want {
			t.Fatalf("finish_reason for %s = %q, want %q", tc.body, resp.Choices[0].FinishReason, tc.want)
		}
	}
}

func TestGeminiAdapterThoughtSignatureMatchesOpenCodeRule(t *testing.T) {
	// Signed call appears after an unsigned one: OpenCode gives the leading
	// unsigned call the skip sentinel, keeps the real signature, and leaves
	// later unsigned siblings empty.
	msg := openAIInboundMessage{
		Role: "assistant",
		ToolCalls: []openAIToolCall{
			{ID: "a"},
			{ID: "b", ExtraContent: map[string]json.RawMessage{"google": json.RawMessage(`{"thought_signature":"real"}`)}},
			{ID: "c"},
		},
	}
	for i := range msg.ToolCalls {
		msg.ToolCalls[i].Function.Name = "shell_execute"
	}
	parts := lowerAssistantParts(msg, false)
	got := []string{parts[0].ThoughtSignature, parts[1].ThoughtSignature, parts[2].ThoughtSignature}
	want := []string{geminiSkipThoughtSignature, "real", ""}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("signatures = %q, want %q", got, want)
		}
	}
}

func TestGeminiAdapterLegacyModelOmitsIDsAndThinkingOptions(t *testing.T) {
	openAIBody := []byte(`{
		"messages": [
			{"role": "user", "content": "hi"},
			{"role": "assistant", "content": null, "tool_calls": [
				{"id": "call_1", "type": "function", "function": {"name": "shell_execute", "arguments": ""}}
			]},
			{"role": "tool", "tool_call_id": "call_1", "name": "shell_execute", "content": "done"},
			{"role": "system", "content": "late update"}
		],
		"tools": [{"type": "function", "function": {"name": "shell_execute", "description": "run"}}],
		"tool_choice": {"type": "function", "function": {"name": "shell_execute"}}
	}`)

	invalidEffort, err := decodeModelExtraBody(`{"reasoning_effort": "none"}`)
	if err != nil {
		t.Fatalf("decodeModelExtraBody: %v", err)
	}
	candidate := modelCandidate{Model: "gemini-2.5-flash", ExtraBody: invalidEffort}

	req, err := geminiUpstreamAdapter{}.BuildRequest(context.Background(), "https://generativelanguage.googleapis.com/v1beta", "", candidate, openAIBody)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	if want := "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent"; req.URL.String() != want {
		t.Fatalf("URL = %q, want %q", req.URL.String(), want)
	}
	if req.Header.Get("x-goog-api-key") != "" {
		t.Fatal("empty api key must not set header")
	}

	sentBytes, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if strings.Contains(string(sentBytes), `"id"`) {
		t.Fatalf("gemini 2.x request must omit call IDs: %s", string(sentBytes))
	}
	if strings.Contains(string(sentBytes), "thinkingLevel") {
		t.Fatalf("invalid reasoning_effort must be ignored: %s", string(sentBytes))
	}

	var parsed geminiRequest
	if err := json.Unmarshal(sentBytes, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// user, model(call), user(functionResponse), user(system-update as its own turn)
	if len(parsed.Contents) != 4 || parsed.Contents[3].Role != "user" ||
		parsed.Contents[3].Parts[0].Text != "<system-update>\nlate update\n</system-update>" {
		t.Fatalf("contents = %s", string(sentBytes))
	}
	if parsed.SystemInstruction != nil {
		t.Fatalf("late system message must not become systemInstruction")
	}
	call := parsed.Contents[1].Parts[0].FunctionCall
	if call == nil || call.Name != "shell_execute" {
		t.Fatalf("functionCall = %+v", call)
	}
	if args, ok := call.Args.(map[string]any); !ok || len(args) != 0 {
		t.Fatalf("empty arguments must become {} args, got %#v", call.Args)
	}
	if len(parsed.Tools) != 1 || string(parsed.Tools[0].FunctionDeclarations[0].ParametersJSONSchema) != `{"type":"object","properties":{}}` {
		t.Fatalf("tools = %+v", parsed.Tools)
	}
	cfg := parsed.ToolConfig
	if cfg == nil || cfg.FunctionCallingConfig.Mode != "ANY" || len(cfg.FunctionCallingConfig.AllowedFunctionNames) != 1 {
		t.Fatalf("toolConfig = %+v", cfg)
	}

	thinkingBody, err := decodeModelExtraBody(`{"thinkingConfig": {"thinkingBudget": 0}}`)
	if err != nil {
		t.Fatalf("decodeModelExtraBody: %v", err)
	}
	candidate.ExtraBody = thinkingBody
	req2, err := geminiUpstreamAdapter{}.BuildRequest(context.Background(), "https://generativelanguage.googleapis.com/v1beta", "k", candidate,
		[]byte(`{"messages":[{"role":"user","content":"hi"}],"tool_choice":"none"}`))
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	sent2, _ := io.ReadAll(req2.Body)
	if !strings.Contains(string(sent2), `"thinkingConfig":{"thinkingBudget":0}`) {
		t.Fatalf("thinkingConfig passthrough missing: %s", string(sent2))
	}
	if strings.Contains(string(sent2), "toolConfig") {
		t.Fatalf("toolConfig must be omitted when no tools are declared: %s", string(sent2))
	}
}
