package llm

import "context"

// ChatMessage is one entry in the conversation transcript.
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    *string    `json:"content,omitempty"`      // omitted when an assistant message only has tool calls
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // present on assistant messages
	ToolCallID string     `json:"tool_call_id,omitempty"` // present on tool messages
	Name       string     `json:"name,omitempty"`         // present on tool messages
}

// ToolCall is a tool invocation requested by the model.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // always "function"
	Function FunctionCall `json:"function"`
}

// FunctionCall carries the tool name and its JSON-encoded arguments.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolChoice controls whether the model may invoke tools in a completion.
type ToolChoice string

const (
	ToolChoiceUnknown ToolChoice = ""
	ToolChoiceAuto    ToolChoice = "auto"
	ToolChoiceNone    ToolChoice = "none"
)

// CompletionRequest is a protocol-neutral model call.
type CompletionRequest struct {
	Messages    []ChatMessage
	Tools       []ToolDefinition
	ToolChoice  ToolChoice
	Temperature *float64
	TopP        *float64
}

// Usage is token usage reported by a provider.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// IsZero reports whether no token counts were returned.
func (u Usage) IsZero() bool {
	return u.PromptTokens == 0 && u.CompletionTokens == 0 && u.TotalTokens == 0
}

// CompletionResponse is a protocol-neutral model reply.
type CompletionResponse struct {
	Model     string     // concrete model reported by the provider; may be empty
	Content   string     // text content (may be empty when only tool calls are returned)
	ToolCalls []ToolCall // tool calls requested by the assistant (may be empty)
	Usage     Usage      // provider-reported token usage; zero when missing
}

// Model is the single entry point for an LLM wire protocol.
type Model interface {
	Call(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
}

// StringPtr returns a pointer to the given string.
func StringPtr(s string) *string { return &s }

// DerefPtr dereferences a string pointer, returning an empty string for nil.
func DerefPtr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// Float64Ptr returns a pointer to the given float64.
func Float64Ptr(v float64) *float64 { return &v }
