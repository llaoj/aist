package llm

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// ErrAuthentication marks authentication failures that should not be retried.
var ErrAuthentication = errors.New("authentication failed")

const (
	ErrorCodeQuotaExceeded       = "quota_exceeded"
	ErrorCodeNoAvailableModel    = "no_available_model"
	ErrorCodeUpstreamUnavailable = "upstream_unavailable"
	ErrorCodeInvalidRequest      = "invalid_request"
)

// StatusError is an unexpected HTTP status from the server proxy.
type StatusError struct {
	StatusCode int
	ErrorCode  string
	Message    string
	RetryAt    time.Time
}

// Error implements error.
func (e *StatusError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("request failed, status: %d", e.StatusCode)
}

var _ Model = (*OpenAICompatibleModel)(nil)

// OpenAICompatibleModel speaks the OpenAI Chat Completions protocol.
type OpenAICompatibleModel struct {
	name              string
	baseURL           string
	apiKey            string
	accessTokenSource AccessTokenSource
	modelName         string
	headers           map[string]string
	maxTokens         int
	httpClient        *http.Client
}

type chatCompletionResponse struct {
	Model   string   `json:"model"`
	Choices []choice `json:"choices"`
	Usage   *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type choice struct {
	Message ChatMessage `json:"message"`
}

// NewOpenAICompatibleModel creates an OpenAI-compatible model from a spec.
func NewOpenAICompatibleModel(spec ProviderSpec) (*OpenAICompatibleModel, error) {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
		},
		TLSHandshakeTimeout: 10 * time.Second,
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
		DisableCompression:  false,
	}
	if spec.HeaderTimeout > 0 {
		transport.ResponseHeaderTimeout = spec.HeaderTimeout
	}

	client := &http.Client{Transport: transport}
	if spec.Timeout > 0 {
		client.Timeout = spec.Timeout
	}

	return &OpenAICompatibleModel{
		name:              spec.ID,
		baseURL:           spec.BaseURL,
		apiKey:            spec.APIKey,
		accessTokenSource: spec.AccessTokenSource,
		modelName:         spec.ModelName,
		headers:           spec.Headers,
		maxTokens:         spec.MaxTokens,
		httpClient:        client,
	}, nil
}

// Name returns the provider/model identifier.
func (o *OpenAICompatibleModel) Name() string {
	return o.name
}

// Call sends one Chat Completions request and normalizes the response.
func (o *OpenAICompatibleModel) Call(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	body, err := o.buildRequestBody(req)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}

	httpResp, err := o.doRequest(ctx, body)
	if err != nil {
		return nil, err
	}
	if httpResp.StatusCode == http.StatusUnauthorized && o.accessTokenSource != nil {
		_ = httpResp.Body.Close()
		if _, refreshErr := o.accessTokenSource.RefreshAccessToken(ctx); refreshErr != nil {
			return nil, fmt.Errorf("%w: %v", ErrAuthentication, refreshErr)
		}
		httpResp, err = o.doRequest(ctx, body)
		if err != nil {
			return nil, err
		}
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("%w: invalid access token", ErrAuthentication)
	}
	if httpResp.StatusCode > 299 {
		return nil, parseStatusError(httpResp.StatusCode, httpResp.Body)
	}

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var payload chatCompletionResponse
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if payload.Error != nil {
		return nil, fmt.Errorf("API error from %s: %s", o.name, payload.Error.Message)
	}
	if len(payload.Choices) == 0 {
		return nil, fmt.Errorf("no response from %s", o.name)
	}

	msg := payload.Choices[0].Message
	response := &CompletionResponse{
		Model:     payload.Model,
		Content:   DerefPtr(msg.Content),
		ToolCalls: msg.ToolCalls,
	}
	if payload.Usage != nil {
		response.Usage = Usage{
			PromptTokens:     payload.Usage.PromptTokens,
			CompletionTokens: payload.Usage.CompletionTokens,
			TotalTokens:      payload.Usage.TotalTokens,
		}
		if response.Usage.TotalTokens == 0 {
			response.Usage.TotalTokens = response.Usage.PromptTokens + response.Usage.CompletionTokens
		}
	}
	return response, nil
}

func (o *OpenAICompatibleModel) doRequest(ctx context.Context, body []byte) (*http.Response, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	if err := o.applyHeaders(ctx, httpReq); err != nil {
		return nil, err
	}
	resp, err := o.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%s API call failed: %w", o.name, err)
	}
	return resp, nil
}

func (o *OpenAICompatibleModel) applyHeaders(ctx context.Context, req *http.Request) error {
	req.Header.Set("Content-Type", "application/json")
	token := o.apiKey
	if o.accessTokenSource != nil {
		value, err := o.accessTokenSource.AccessToken(ctx)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrAuthentication, err)
		}
		token = value
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for key, value := range o.headers {
		req.Header.Set(key, value)
	}
	return nil
}

func (o *OpenAICompatibleModel) buildRequestBody(req CompletionRequest) ([]byte, error) {
	body := map[string]any{
		"model":    o.modelName,
		"messages": req.Messages,
	}

	if tools := wireTools(req.Tools); len(tools) > 0 {
		body["tools"] = tools
		choice := req.ToolChoice
		if choice == ToolChoiceUnknown {
			choice = ToolChoiceAuto
		}
		body["tool_choice"] = string(choice)
	}
	if o.maxTokens > 0 {
		body["max_tokens"] = o.maxTokens
	}
	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		body["top_p"] = *req.TopP
	}

	return json.Marshal(body)
}

func wireTools(defs []ToolDefinition) []map[string]any {
	tools := make([]map[string]any, 0, len(defs))
	for _, tool := range defs {
		tools = append(tools, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        tool.Name,
				"description": tool.Description,
				"parameters":  tool.InputSchema,
			},
		})
	}
	return tools
}

func parseStatusError(statusCode int, body io.Reader) *StatusError {
	statusErr := &StatusError{
		StatusCode: statusCode,
	}
	if body == nil {
		return statusErr
	}
	raw, err := io.ReadAll(io.LimitReader(body, 64<<10))
	if err != nil || len(raw) == 0 {
		return statusErr
	}
	var payload struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			RetryAt string `json:"retry_at"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return statusErr
	}
	statusErr.ErrorCode = payload.Error.Code
	statusErr.Message = payload.Error.Message
	if payload.Error.RetryAt != "" {
		if parsed, err := time.Parse(time.RFC3339, payload.Error.RetryAt); err == nil {
			statusErr.RetryAt = parsed
		}
	}
	return statusErr
}
