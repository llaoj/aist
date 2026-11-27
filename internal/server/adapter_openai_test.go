package server

import (
	"context"
	"encoding/json"
	"io"
	"testing"
)

func TestOpenAIAdapterBuildRequestAndExtraBody(t *testing.T) {
	extraBody, err := decodeModelExtraBody(`{
		"thinking": {"type": "disabled"},
		"enable_thinking": false,
		"model": "hijacked-model",
		"messages": [{"role": "user", "content": "hijacked"}]
	}`)
	if err != nil {
		t.Fatalf("decodeModelExtraBody: %v", err)
	}

	reqBody := []byte(`{
		"model": "auto",
		"messages": [{"role": "user", "content": "check disk"}],
		"max_tokens": 16384
	}`)
	candidate := modelCandidate{
		Protocol:        upstreamProtocolOpenAI,
		Model:           "deepseek-flash",
		MaxOutputTokens: 8192,
		ExtraBody:       extraBody,
	}

	adapter := selectUpstreamAdapter(candidate.Protocol)
	req, err := adapter.BuildRequest(context.Background(), "https://api.deepseek.com/v1/", "sk-test", candidate, reqBody)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	if req.URL.String() != "https://api.deepseek.com/v1/chat/completions" {
		t.Fatalf("URL = %q", req.URL.String())
	}
	if got := req.Header.Get("Authorization"); got != "Bearer sk-test" {
		t.Fatalf("Authorization = %q", got)
	}

	sentBytes, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
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
	if err := json.Unmarshal(sentBytes, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.Model != "deepseek-flash" || payload.MaxTokens != 8192 {
		t.Fatalf("model=%q max_tokens=%d", payload.Model, payload.MaxTokens)
	}
	if payload.Thinking.Type != "disabled" || payload.EnableThinking == nil || *payload.EnableThinking {
		t.Fatalf("extra_body not merged: %s", string(sentBytes))
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
