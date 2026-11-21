package llm

import (
	"context"
	"encoding/json"
	"time"
)

// ToolDefinition is a protocol-neutral tool definition.
type ToolDefinition struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

// AccessTokenSource supplies a dynamic access token for proxy authentication.
type AccessTokenSource interface {
	AccessToken(ctx context.Context) (string, error)
	RefreshAccessToken(ctx context.Context) (string, error)
}

// ProviderSpec is everything an adapter needs to construct a model.
type ProviderSpec struct {
	ID                string
	BaseURL           string
	APIKey            string
	AccessTokenSource AccessTokenSource
	ModelName         string
	Headers           map[string]string
	Timeout           time.Duration
	HeaderTimeout     time.Duration
	ChunkTimeout      time.Duration
	MaxTokens         int
}
