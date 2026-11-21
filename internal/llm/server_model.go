package llm

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// Hooks receives notifications around model calls.
type Hooks interface {
	BeforeCall(model string, cancel context.CancelFunc)
	AfterCall(model string, err error)
}

const (
	defaultServerMaxAttempts = 3
	defaultServerBackoff     = 500 * time.Millisecond
	maxServerBackoff         = 5 * time.Second
)

var _ Model = (*ServerModel)(nil)

// ServerModel is the single client-side model. The server selects the real
// provider and model; this model only talks to the AIST proxy and retries
// transient server failures.
type ServerModel struct {
	client       *OpenAICompatibleModel
	hooks        Hooks
	maxAttempts  int
	retryBackoff time.Duration
}

// ServerModelOptions configures a ServerModel.
type ServerModelOptions struct {
	BaseURL           string
	AccessTokenSource AccessTokenSource
	MaxTokens         int
	Timeout           time.Duration
	HeaderTimeout     time.Duration
	ChunkTimeout      time.Duration
	Hooks             Hooks
	MaxAttempts       int
	RetryBackoff      time.Duration
}

// NewServerModel creates a ServerModel.
func NewServerModel(opts ServerModelOptions) (*ServerModel, error) {
	client, err := NewOpenAICompatibleModel(ProviderSpec{
		ID:                "server",
		BaseURL:           opts.BaseURL,
		AccessTokenSource: opts.AccessTokenSource,
		ModelName:         "auto",
		Timeout:           opts.Timeout,
		HeaderTimeout:     opts.HeaderTimeout,
		ChunkTimeout:      opts.ChunkTimeout,
		MaxTokens:         opts.MaxTokens,
	})
	if err != nil {
		return nil, err
	}
	maxAttempts := opts.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultServerMaxAttempts
	}
	backoff := opts.RetryBackoff
	if backoff <= 0 {
		backoff = defaultServerBackoff
	}
	return &ServerModel{
		client:       client,
		hooks:        opts.Hooks,
		maxAttempts:  maxAttempts,
		retryBackoff: backoff,
	}, nil
}

// Name returns the logical model name used by hooks and diagnostics.
func (m *ServerModel) Name() string { return "server" }

// Call sends one request to the server proxy, retrying transient failures.
func (m *ServerModel) Call(ctx context.Context, req CompletionRequest) (resp *CompletionResponse, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if m.hooks != nil {
		m.hooks.BeforeCall(m.Name(), cancel)
	}
	defer func() {
		if m.hooks != nil {
			m.hooks.AfterCall(m.Name(), err)
		}
	}()

	var lastErr error
	for attempt := 0; attempt < m.maxAttempts; attempt++ {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		response, callErr := m.client.Call(ctx, req)
		if callErr == nil {
			return response, nil
		}
		lastErr = callErr
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		if attempt == m.maxAttempts-1 || !retryableServerError(callErr) {
			break
		}
		if backoffErr := waitBackoff(ctx, serverBackoff(m.retryBackoff, attempt)); backoffErr != nil {
			return nil, backoffErr
		}
	}
	if lastErr == nil {
		lastErr = errors.New("server request failed")
	}
	return nil, lastErr
}

func retryableServerError(err error) bool {
	if errors.Is(err, ErrAuthentication) {
		return false
	}
	var status *StatusError
	if errors.As(err, &status) {
		switch status.ErrorCode {
		case ErrorCodeQuotaExceeded, ErrorCodeNoAvailableModel, ErrorCodeInvalidRequest:
			return false
		}
		return status.StatusCode == http.StatusTooManyRequests || status.StatusCode >= 500
	}
	return true
}

func serverBackoff(base time.Duration, attempt int) time.Duration {
	backoff := base << attempt
	if backoff > maxServerBackoff {
		return maxServerBackoff
	}
	return backoff
}

func waitBackoff(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
