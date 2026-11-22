// Package remote talks to the AIST server for authentication and token refresh.
//
// The server owns provider and model configuration. The client only sends
// credentials or a PocketBase auth token and receives non-secret settings; real
// provider API keys never reach the client.
package remote

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultServerURL is the built-in server used when nothing else is configured.
// Override it at build time with -ldflags or at runtime with AIST_SERVER.
var DefaultServerURL = "https://aistrun.cc"

const (
	usersAuthBasePath    = "/api/collections/users"
	authMethodsPath      = usersAuthBasePath + "/auth-methods"
	authWithPasswordPath = usersAuthBasePath + "/auth-with-password"
	requestOTPPath       = usersAuthBasePath + "/request-otp"
	authWithOTPPath      = usersAuthBasePath + "/auth-with-otp"
	authRefreshPath      = usersAuthBasePath + "/auth-refresh"
	accountUsagePath     = "/v1/account/usage"
	proxyPath            = "/v1/proxy"
)

// APIError is a non-2xx response from the server.
type APIError struct {
	StatusCode int
	Message    string
	MfaID      string
	Data       json.RawMessage
}

// Error implements error.
func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("server returned status %d", e.StatusCode)
	}
	return fmt.Sprintf("server returned status %d: %s", e.StatusCode, e.Message)
}

// IsUnauthorized reports whether err is an authentication or authorization error.
func IsUnauthorized(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden
}

// IsBadRequest reports whether err is a bad request error.
func IsBadRequest(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.StatusCode == http.StatusBadRequest
}

// IsNotFound reports whether err is a 404 Not Found error.
func IsNotFound(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.StatusCode == http.StatusNotFound
}

// MFARequired returns the MFA challenge id when the server requires a second
// factor.
func MFARequired(err error) (string, bool) {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return "", false
	}
	if apiErr.StatusCode != http.StatusUnauthorized || strings.TrimSpace(apiErr.MfaID) == "" {
		return "", false
	}
	return apiErr.MfaID, true
}

// AccountConfig is the non-secret configuration delivered by the login or
// refresh response.
type AccountConfig struct {
	SchemaVersion int            `json:"schema_version"`
	Limits        LimitsInfo     `json:"limits"`
	Session       *SessionConfig `json:"session,omitempty"`
	Blacklist     []string       `json:"blacklist,omitempty"`
}

// LimitsInfo describes the effective model limits chosen by the server.
type LimitsInfo struct {
	ContextWindow   int `json:"context_window"`
	MaxOutputTokens int `json:"max_output_tokens"`
}

// SessionConfig mirrors the session settings sent by the server.
type SessionConfig struct {
	MaxSessions int              `json:"max_sessions,omitempty"`
	Compaction  CompactionConfig `json:"compaction,omitempty"`
}

// CompactionConfig mirrors the compaction settings sent by the server.
type CompactionConfig struct {
	KeepTokens   int     `json:"keep_tokens,omitempty"`
	TriggerRatio float64 `json:"trigger_ratio,omitempty"`
}

// PasswordAuthMethods describes the password login settings.
type PasswordAuthMethods struct {
	Enabled        bool     `json:"enabled"`
	IdentityFields []string `json:"identityFields"`
}

// OTPAuthMethods describes the OTP login settings.
type OTPAuthMethods struct {
	Enabled  bool  `json:"enabled"`
	Duration int64 `json:"duration"`
}

// MFAAuthMethods describes the MFA settings.
type MFAAuthMethods struct {
	Enabled  bool  `json:"enabled"`
	Duration int64 `json:"duration"`
}

// AuthMethods describes the login methods enabled for the users collection.
type AuthMethods struct {
	Password PasswordAuthMethods `json:"password"`
	OTP      OTPAuthMethods      `json:"otp"`
	MFA      MFAAuthMethods      `json:"mfa"`
}

// AuthRecord is the subset of the user record returned by PocketBase auth.
type AuthRecord struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

// AuthMeta is the auth response metadata.
type AuthMeta struct {
	Config AccountConfig `json:"config"`
}

// AuthResponse is returned by PocketBase auth and auth-refresh.
type AuthResponse struct {
	Token  string        `json:"token"`
	Record AuthRecord    `json:"record"`
	Meta   AuthMeta      `json:"meta"`
	Config AccountConfig `json:"-"`
}

// AccountUsage describes the current UTC-week token consumption and quota.
type AccountUsage struct {
	WeekStart        string    `json:"week_start"`
	ResetAt          time.Time `json:"reset_at"`
	Requests         int       `json:"requests"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	UsedTokens       int       `json:"used_tokens"`
	WeeklyTokenQuota int       `json:"weekly_token_quota"`
	RemainingTokens  *int      `json:"remaining_tokens,omitempty"`
}

// ClientOptions configures a Client.
type ClientOptions struct {
	ServerURL  string
	Timeout    time.Duration
	HTTPClient *http.Client
}

// Client talks to the AIST server.
type Client struct {
	serverURL string
	http      *http.Client
}

// NewClient creates a remote client.
func NewClient(opts ClientOptions) *Client {
	serverURL := strings.TrimRight(strings.TrimSpace(opts.ServerURL), "/")
	if serverURL == "" {
		serverURL = DefaultServerURL
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		timeout := opts.Timeout
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
		httpClient = &http.Client{Timeout: timeout}
	}
	return &Client{
		serverURL: serverURL,
		http:      httpClient,
	}
}

// ServerURL returns the normalized base URL of the target server.
func (c *Client) ServerURL() string {
	return c.serverURL
}

// ProxyBaseURL returns the proxy endpoint derived from the server URL.
func (c *Client) ProxyBaseURL() string {
	return c.serverURL + proxyPath
}

// AuthMethods returns the login methods enabled for the users collection.
func (c *Client) AuthMethods(ctx context.Context) (*AuthMethods, error) {
	data, err := c.request(ctx, http.MethodGet, authMethodsPath, nil, "")
	if err != nil {
		return nil, err
	}
	var out AuthMethods
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("failed to parse auth methods: %w", err)
	}
	return &out, nil
}

// AuthWithPassword authenticates with email and password.
func (c *Client) AuthWithPassword(ctx context.Context, email, password string) (*AuthResponse, error) {
	payload := map[string]string{
		"identity": email,
		"password": password,
	}
	data, err := c.request(ctx, http.MethodPost, authWithPasswordPath, payload, "")
	if err != nil {
		return nil, err
	}
	return decodeAuthResponse(data)
}

// RequestOTP asks the server to send a one-time password to email.
func (c *Client) RequestOTP(ctx context.Context, email string) (string, error) {
	payload := map[string]string{"email": email}
	data, err := c.request(ctx, http.MethodPost, requestOTPPath, payload, "")
	if err != nil {
		return "", err
	}
	var out struct {
		OTPID string `json:"otpId"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("failed to parse OTP response: %w", err)
	}
	if strings.TrimSpace(out.OTPID) == "" {
		return "", errors.New("server returned an empty OTP id")
	}
	return out.OTPID, nil
}

// AuthWithOTP authenticates with a one-time password. mfaID is optional.
func (c *Client) AuthWithOTP(ctx context.Context, otpID, code, mfaID string) (*AuthResponse, error) {
	path := authWithOTPPath
	if strings.TrimSpace(mfaID) != "" {
		path += "?mfaId=" + url.QueryEscape(mfaID)
	}
	payload := map[string]string{
		"otpId":    otpID,
		"password": code,
	}
	data, err := c.request(ctx, http.MethodPost, path, payload, "")
	if err != nil {
		return nil, err
	}
	return decodeAuthResponse(data)
}

// Refresh exchanges a PocketBase auth token for a fresh auth token and account
// configuration.
func (c *Client) Refresh(ctx context.Context, token string) (*AuthResponse, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("auth token is required")
	}
	data, err := c.request(ctx, http.MethodPost, authRefreshPath, nil, token)
	if err != nil {
		return nil, err
	}
	return decodeAuthResponse(data)
}

// AccountUsage fetches the authenticated user's current UTC-week token usage and quota.
func (c *Client) AccountUsage(ctx context.Context, token string) (*AccountUsage, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("auth token is required")
	}
	data, err := c.request(ctx, http.MethodGet, accountUsagePath, nil, token)
	if err != nil {
		return nil, err
	}
	var out AccountUsage
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("failed to parse account usage: %w", err)
	}
	return &out, nil
}

// TokenExpiry returns the exp claim of a JWT-like auth token.
func TokenExpiry(token string) time.Time {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp <= 0 {
		return time.Time{}
	}
	return time.Unix(claims.Exp, 0)
}

func decodeAuthResponse(data []byte) (*AuthResponse, error) {
	var out AuthResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("failed to parse auth response: %w", err)
	}
	if strings.TrimSpace(out.Token) == "" {
		return nil, errors.New("server returned an empty auth token")
	}
	out.Config = out.Meta.Config
	return &out, nil
}

func (c *Client) request(ctx context.Context, method, path string, payload any, bearer string) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to encode request: %w", err)
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.serverURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach %s: %w", c.serverURL, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, newAPIError(resp.StatusCode, data)
	}
	return data, nil
}

func newAPIError(statusCode int, data []byte) *APIError {
	var payload struct {
		Message string          `json:"message"`
		MfaID   string          `json:"mfaId"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &payload); err == nil {
		message := strings.TrimSpace(payload.Message)
		if message == "" {
			message = strings.TrimSpace(string(data))
		}
		return &APIError{
			StatusCode: statusCode,
			Message:    message,
			MfaID:      payload.MfaID,
			Data:       payload.Data,
		}
	}
	return &APIError{
		StatusCode: statusCode,
		Message:    strings.TrimSpace(string(data)),
	}
}
