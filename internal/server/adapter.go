package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type upstreamProtocol string

const (
	upstreamProtocolUnknown upstreamProtocol = ""
	upstreamProtocolOpenAI  upstreamProtocol = "openai"
	upstreamProtocolGemini  upstreamProtocol = "gemini"
)

// upstreamAdapter translates between the proxy's canonical OpenAI Chat
// Completions wire format and a specific upstream provider protocol.
type upstreamAdapter interface {
	BuildRequest(ctx context.Context, baseURL, apiKey string, candidate modelCandidate, openAIBody []byte) (*http.Request, error)
	NormalizeResponse(modelName string, respBody []byte) ([]byte, error)
}

func parseUpstreamProtocol(raw string) upstreamProtocol {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(upstreamProtocolGemini):
		return upstreamProtocolGemini
	case string(upstreamProtocolOpenAI):
		return upstreamProtocolOpenAI
	default:
		return upstreamProtocolUnknown
	}
}

func selectUpstreamAdapter(protocol upstreamProtocol) upstreamAdapter {
	switch protocol {
	case upstreamProtocolGemini:
		return geminiUpstreamAdapter{}
	default:
		return openAIUpstreamAdapter{}
	}
}

// clampMaxTokens applies the model's output limit: a missing or larger
// request uses the limit; a zero limit leaves the request unchanged.
func clampMaxTokens(requested, limit int) int {
	if limit > 0 && (requested <= 0 || requested > limit) {
		return limit
	}
	return requested
}

// --- Canonical OpenAI Chat Completions wire format ---
//
// The proxy speaks this format to clients; adapters that translate to another
// upstream protocol decode requests and encode responses with these types.

type openAIInboundRequest struct {
	Messages    []openAIInboundMessage `json:"messages"`
	Tools       []openAIInboundTool    `json:"tools"`
	ToolChoice  json.RawMessage        `json:"tool_choice"`
	MaxTokens   int                    `json:"max_tokens"`
	Temperature *float64               `json:"temperature"`
	TopP        *float64               `json:"top_p"`
}

type openAIInboundMessage struct {
	Role       string           `json:"role"`
	Content    json.RawMessage  `json:"content"`
	ToolCalls  []openAIToolCall `json:"tool_calls"`
	ToolCallID string           `json:"tool_call_id"`
	Name       string           `json:"name"`
}

// openAIToolCall is used for requests and responses. extra_content carries
// provider extensions such as Google's thought_signature.
type openAIToolCall struct {
	ID           string                     `json:"id"`
	Type         string                     `json:"type"`
	Function     openAIFunctionCall         `json:"function"`
	ExtraContent map[string]json.RawMessage `json:"extra_content,omitempty"`
}

type openAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIInboundTool struct {
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type openAIChatResponse struct {
	Model   string             `json:"model"`
	Choices []openAIChatChoice `json:"choices"`
	Usage   *openAIUsage       `json:"usage,omitempty"`
}

type openAIChatChoice struct {
	Index        int               `json:"index"`
	Message      openAIChatMessage `json:"message"`
	FinishReason string            `json:"finish_reason"`
}

type openAIChatMessage struct {
	Role      string           `json:"role"`
	Content   *string          `json:"content"`
	ToolCalls []openAIToolCall `json:"tool_calls,omitempty"`
}

type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// extractMessageText accepts OpenAI string content or an array of text parts.
func extractMessageText(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	var s string
	if json.Unmarshal(trimmed, &s) == nil {
		return s
	}
	var parts []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(trimmed, &parts) != nil {
		return ""
	}
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(p.Text)
	}
	return b.String()
}
