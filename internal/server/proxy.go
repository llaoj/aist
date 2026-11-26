package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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

	forwardBody, err := rewriteModel(body, candidate.Model, candidate.MaxOutputTokens)
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

// rewriteModel replaces the request body's "model" field with the bare model
// name and clamps max_tokens to the candidate's output limit.
func rewriteModel(body []byte, modelName string, maxOutputTokens int) ([]byte, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, errors.New("request body must be a JSON object")
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

	return json.Marshal(fields)
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
