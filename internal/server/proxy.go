package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

// proxyClient forwards requests to upstream providers; per-candidate deadlines
// are enforced through request contexts derived from the proxy budget.
var proxyClient = &http.Client{}

// handleChatCompletions authenticates the request, selects candidate models and
// forwards the OpenAI-compatible request upstream.
func (s *API) handleChatCompletions(e *core.RequestEvent) error {
	record := e.Auth

	body, err := io.ReadAll(e.Request.Body)
	if err != nil {
		if errors.Is(err, apis.ErrRequestEntityTooLarge) {
			return err
		}
		return respondProxyError(e, http.StatusBadRequest, proxyErrorCodeInvalidRequest, "failed to read request body", time.Time{})
	}

	if err := validateChatBody(body); err != nil {
		return respondProxyError(e, http.StatusBadRequest, proxyErrorCodeInvalidRequest, "invalid JSON body", time.Time{})
	}

	// Audit once per request, before the fallback loop retries upstreams.
	s.recordToolAudit(e, record, body)

	now := s.now()
	quotaExceeded, retryAt, err := checkWeeklyTokenQuota(e.App, record, now)
	if err != nil {
		return e.InternalServerError("failed to check token quota", err)
	}
	if quotaExceeded {
		return respondProxyError(e, http.StatusTooManyRequests, proxyErrorCodeQuotaExceeded, "weekly token quota exceeded", retryAt)
	}

	candidates, err := s.resolveCandidates(e.App, record.Id)
	if err != nil {
		return respondProxyError(e, http.StatusForbidden, proxyErrorCodeNoAvailableModel, err.Error(), time.Time{})
	}
	if len(candidates) == 0 {
		return respondProxyError(e, http.StatusForbidden, proxyErrorCodeNoAvailableModel, "no model is available for this account", time.Time{})
	}

	available := filterAvailableCandidates(candidates, now)
	if len(available) == 0 {
		return respondProxyError(e, http.StatusServiceUnavailable, proxyErrorCodeNoAvailableModel, "no model is currently available", earliestModelRetryAt(candidates, now))
	}

	proxyCtx, cancelProxy := context.WithTimeout(e.Request.Context(), s.proxyRequestTimeout)
	defer cancelProxy()

	sawTransient := false
	for _, candidate := range available {
		if proxyCtx.Err() != nil {
			break
		}
		res := s.forwardCandidate(proxyCtx, e, body, candidate)
		if res.err == nil {
			if candidate.hasAvailabilityState() {
				clearModelAvailability(e.App, candidate.ModelID)
			}
			recordDailyModelUsage(e.App, record.Id, candidate, res.body, s.now())
			e.Response.Header().Set("Content-Type", "application/json")
			e.Response.WriteHeader(res.status)
			_, _ = e.Response.Write(res.body)
			return nil
		}
		if e.Request.Context().Err() != nil {
			return e.Request.Context().Err()
		}
		failureNow := s.now()
		classified := classifyUpstreamError(res.status, res.header, res.body, res.err, failureNow)
		if classified.Kind == upstreamErrorKindBadRequest {
			return respondProxyError(e, http.StatusBadRequest, proxyErrorCodeInvalidRequest, classified.Summary, time.Time{})
		}
		if classified.Kind == upstreamErrorKindTransient {
			sawTransient = true
		}
		recordModelOutcome(e.App, candidate, classified, failureNow)
	}

	if !sawTransient {
		endNow := s.now()
		reloaded, err := s.resolveCandidates(e.App, record.Id)
		var nextRetry time.Time
		if err == nil {
			nextRetry = earliestModelRetryAt(reloaded, endNow)
		}
		return respondProxyError(e, http.StatusServiceUnavailable, proxyErrorCodeNoAvailableModel, "no model is currently available", nextRetry)
	}

	return respondProxyError(e, http.StatusBadGateway, proxyErrorCodeUpstreamUnavailable, "upstream model service is temporarily unavailable", time.Time{})
}

func (s *API) resolveCandidates(app core.App, userID string) ([]modelCandidate, error) {
	record, err := app.FindRecordById(usersCollection, userID)
	if err != nil || record == nil {
		return nil, errors.New("account not found")
	}
	candidates, err := loadUserCandidates(app, record)
	if err != nil {
		return nil, errors.New("account configuration is invalid")
	}
	return candidates, nil
}

type forwardResult struct {
	status int
	header http.Header
	body   []byte
	err    error
}

func (s *API) forwardCandidate(proxyCtx context.Context, e *core.RequestEvent, body []byte, candidate modelCandidate) forwardResult {
	provider, err := e.App.FindRecordById(upstreamProvidersCollection, candidate.ProviderID)
	if err != nil || provider == nil || !provider.GetBool("enabled") {
		return forwardResult{err: errors.New("provider unavailable")}
	}

	apiKey, err := resolveProviderAPIKey(e.App, provider)
	if err != nil {
		return forwardResult{err: err}
	}
	baseURL := strings.TrimRight(provider.GetString("base_url"), "/")

	forwardBody, err := rewriteForwardBody(body, baseURL, candidate.Model, candidate.MaxOutputTokens, candidate.ExtraBody)
	if err != nil {
		return forwardResult{err: err}
	}

	attemptCtx, cancelAttempt := context.WithTimeout(proxyCtx, s.upstreamModelTimeout)
	defer cancelAttempt()

	req, err := http.NewRequestWithContext(
		attemptCtx,
		http.MethodPost,
		baseURL+"/chat/completions",
		bytes.NewReader(forwardBody),
	)
	if err != nil {
		return forwardResult{err: err}
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := proxyClient.Do(req)
	if err != nil {
		return forwardTransportFailure(e, attemptCtx, candidate, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return forwardTransportFailure(e, attemptCtx, candidate, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		e.App.Logger().Warn("upstream returned non-2xx status",
			"provider", candidate.Provider,
			"model", candidate.Model,
			"status", resp.StatusCode,
			"detail", summarizeUpstreamError(resp.StatusCode, respBody),
		)
		return forwardResult{
			status: resp.StatusCode,
			header: resp.Header,
			body:   respBody,
			err:    fmt.Errorf("upstream returned status %d", resp.StatusCode),
		}
	}
	return forwardResult{
		status: resp.StatusCode,
		header: resp.Header,
		body:   respBody,
	}
}

func forwardTransportFailure(e *core.RequestEvent, attemptCtx context.Context, candidate modelCandidate, err error) forwardResult {
	if e.Request.Context().Err() != nil {
		return forwardResult{err: err}
	}
	if errors.Is(attemptCtx.Err(), context.DeadlineExceeded) {
		e.App.Logger().Warn("upstream request timed out",
			"provider", candidate.Provider,
			"model", candidate.Model,
		)
		return forwardResult{err: context.DeadlineExceeded}
	}
	e.App.Logger().Warn("upstream transport failed",
		"provider", candidate.Provider,
		"model", candidate.Model,
	)
	return forwardResult{err: err}
}

func validateChatBody(body []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return err
	}
	if fields == nil {
		return errors.New("request body must be a JSON object")
	}
	return nil
}

const geminiSkipThoughtSignature = "skip_thought_signature_validator"

// rewriteForwardBody merges model-level extra_body fields, replaces "model"
// with the bare model name, clamps max_tokens to the candidate's output limit,
// and fills missing Gemini thought_signature fields when forwarding to Google.
func rewriteForwardBody(body []byte, baseURL, modelName string, maxOutputTokens int, extraBody map[string]json.RawMessage) ([]byte, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, errors.New("request body must be a JSON object")
	}
	for key, value := range extraBody {
		if key == "model" || key == "messages" {
			continue
		}
		fields[key] = value
	}
	model, err := json.Marshal(modelName)
	if err != nil {
		return nil, err
	}
	fields["model"] = model

	if maxOutputTokens > 0 {
		maxTokens := 0
		if raw, ok := fields["max_tokens"]; ok {
			_ = json.Unmarshal(raw, &maxTokens)
		}
		if maxTokens <= 0 || maxTokens > maxOutputTokens {
			value, err := json.Marshal(maxOutputTokens)
			if err != nil {
				return nil, err
			}
			fields["max_tokens"] = value
		}
	}

	if isGoogleGeminiUpstream(baseURL, modelName) {
		if err := ensureGeminiThoughtSignatures(fields); err != nil {
			return nil, err
		}
	}

	return json.Marshal(fields)
}

func isGoogleGeminiUpstream(baseURL, modelName string) bool {
	if parsed, err := url.Parse(baseURL); err == nil {
		host := strings.ToLower(parsed.Hostname())
		if host == "generativelanguage.googleapis.com" || strings.HasSuffix(host, ".googleapis.com") {
			return true
		}
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(modelName)), "gemini-")
}

// ensureGeminiThoughtSignatures injects Google's skip_thought_signature_validator
// sentinel into the first tool_call of any historical assistant message whose
// tool_calls lack a thought_signature. Gemini 3.x requires the signature on the
// first parallel functionCall only and rejects signatures on subsequent calls
// in the same assistant turn.
func ensureGeminiThoughtSignatures(fields map[string]json.RawMessage) error {
	rawMessages, ok := fields["messages"]
	if !ok || len(rawMessages) == 0 {
		return nil
	}
	var messages []map[string]json.RawMessage
	if err := json.Unmarshal(rawMessages, &messages); err != nil {
		return nil
	}

	changed := false
	skipSigRaw, err := json.Marshal(geminiSkipThoughtSignature)
	if err != nil {
		return err
	}

	for i, msg := range messages {
		if msg == nil {
			continue
		}
		rawCalls, ok := msg["tool_calls"]
		if !ok || len(rawCalls) == 0 {
			continue
		}
		var toolCalls []map[string]json.RawMessage
		if err := json.Unmarshal(rawCalls, &toolCalls); err != nil || len(toolCalls) == 0 || toolCalls[0] == nil {
			continue
		}

		if hasAnyGeminiThoughtSignature(toolCalls) {
			continue
		}

		firstCall := toolCalls[0]
		var extraContent map[string]json.RawMessage
		if rawExtra, exists := firstCall["extra_content"]; exists && len(rawExtra) > 0 {
			_ = json.Unmarshal(rawExtra, &extraContent)
		}
		if extraContent == nil {
			extraContent = make(map[string]json.RawMessage)
		}

		var googleExt map[string]json.RawMessage
		if rawGoogle, exists := extraContent["google"]; exists && len(rawGoogle) > 0 {
			_ = json.Unmarshal(rawGoogle, &googleExt)
		}
		if googleExt == nil {
			googleExt = make(map[string]json.RawMessage)
		}

		googleExt["thought_signature"] = skipSigRaw
		encodedGoogle, err := json.Marshal(googleExt)
		if err != nil {
			return err
		}
		extraContent["google"] = encodedGoogle
		encodedExtra, err := json.Marshal(extraContent)
		if err != nil {
			return err
		}
		firstCall["extra_content"] = encodedExtra
		toolCalls[0] = firstCall

		encodedCalls, err := json.Marshal(toolCalls)
		if err != nil {
			return err
		}
		msg["tool_calls"] = encodedCalls
		messages[i] = msg
		changed = true
	}

	if !changed {
		return nil
	}
	encodedMessages, err := json.Marshal(messages)
	if err != nil {
		return err
	}
	fields["messages"] = encodedMessages
	return nil
}

func hasAnyGeminiThoughtSignature(toolCalls []map[string]json.RawMessage) bool {
	for _, tc := range toolCalls {
		if tc == nil {
			continue
		}
		var extraContent map[string]json.RawMessage
		if rawExtra, exists := tc["extra_content"]; !exists || len(rawExtra) == 0 || json.Unmarshal(rawExtra, &extraContent) != nil {
			continue
		}
		var googleExt map[string]json.RawMessage
		if rawGoogle, exists := extraContent["google"]; !exists || len(rawGoogle) == 0 || json.Unmarshal(rawGoogle, &googleExt) != nil {
			continue
		}
		var sig string
		if rawSig, exists := googleExt["thought_signature"]; exists && len(rawSig) > 0 && json.Unmarshal(rawSig, &sig) == nil {
			if strings.TrimSpace(sig) != "" {
				return true
			}
		}
	}
	return false
}

// toRawJSON converts a JSON field value (types.JSONRaw, []byte, string) to bytes.
func toRawJSON(value any) ([]byte, error) {
	switch v := value.(type) {
	case types.JSONRaw:
		return []byte(v), nil
	case []byte:
		return v, nil
	case string:
		if v == "" {
			return nil, nil
		}
		return []byte(v), nil
	default:
		return nil, fmt.Errorf("unsupported JSON field type %T", value)
	}
}
