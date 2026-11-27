package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// openAIUpstreamAdapter forwards requests to standard OpenAI-compatible
// /chat/completions endpoints without vendor-specific message rewriting.
type openAIUpstreamAdapter struct{}

func (openAIUpstreamAdapter) BuildRequest(ctx context.Context, baseURL, apiKey string, candidate modelCandidate, openAIBody []byte) (*http.Request, error) {
	forwardBody, err := rewriteOpenAIBody(openAIBody, candidate.Model, candidate.MaxOutputTokens, candidate.ExtraBody)
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(forwardBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	return req, nil
}

func (openAIUpstreamAdapter) NormalizeResponse(_ string, respBody []byte) ([]byte, error) {
	return respBody, nil
}

func rewriteOpenAIBody(body []byte, modelName string, maxOutputTokens int, extraBody map[string]json.RawMessage) ([]byte, error) {
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

	requested := 0
	_ = json.Unmarshal(fields["max_tokens"], &requested)
	if maxTokens := clampMaxTokens(requested, maxOutputTokens); maxTokens != requested {
		value, err := json.Marshal(maxTokens)
		if err != nil {
			return nil, err
		}
		fields["max_tokens"] = value
	}

	return json.Marshal(fields)
}
