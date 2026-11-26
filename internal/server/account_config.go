package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

const (
	accountSchemaVersion   = 1
	defaultContextWindow   = 128000
	defaultMaxOutputTokens = 8192
)

type userSettings struct {
	Session   *userSessionSettings `json:"session,omitempty"`
	Blacklist []string             `json:"blacklist,omitempty"`
}

type userSessionSettings struct {
	MaxSessions int                    `json:"max_sessions,omitempty"`
	Compaction  userCompactionSettings `json:"compaction,omitempty"`
}

type userCompactionSettings struct {
	KeepTokens   int     `json:"keep_tokens,omitempty"`
	TriggerRatio float64 `json:"trigger_ratio,omitempty"`
}

type clientAccountConfig struct {
	SchemaVersion int                 `json:"schema_version"`
	Session       *clientSession      `json:"session,omitempty"`
	Blacklist     []string            `json:"blacklist,omitempty"`
	Limits        clientAccountLimits `json:"limits"`
}

type clientSession struct {
	MaxSessions int              `json:"max_sessions,omitempty"`
	Compaction  clientCompaction `json:"compaction,omitempty"`
}

type clientCompaction struct {
	KeepTokens   int     `json:"keep_tokens,omitempty"`
	TriggerRatio float64 `json:"trigger_ratio,omitempty"`
}

type clientAccountLimits struct {
	ContextWindow   int `json:"context_window"`
	MaxOutputTokens int `json:"max_output_tokens"`
}

type modelCandidate struct {
	ModelID           string
	ProviderID        string
	Provider          string
	Model             string
	Priority          int
	ContextWindow     int
	MaxOutputTokens   int
	ExtraBody         map[string]json.RawMessage
	UnavailableUntil  time.Time
	UnavailableReason string
	LastError         string
}

func decodeModelExtraBody(raw any) (map[string]json.RawMessage, error) {
	if raw == nil {
		return nil, nil
	}
	data, err := toRawJSON(raw)
	if err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &fields); err != nil {
		return nil, fmt.Errorf("invalid model extra_body: %w", err)
	}
	if fields == nil {
		return nil, errors.New("model extra_body must be a JSON object")
	}
	if len(fields) == 0 {
		return nil, nil
	}
	return fields, nil
}

func decodeUserSettings(raw any) (userSettings, error) {
	if raw == nil {
		return userSettings{}, nil
	}
	data, err := toRawJSON(raw)
	if err != nil {
		return userSettings{}, err
	}
	if len(data) == 0 {
		return userSettings{}, nil
	}
	var settings userSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return userSettings{}, fmt.Errorf("invalid user settings: %w", err)
	}
	return settings, nil
}

func buildClientAccountConfig(app core.App, record *core.Record) (clientAccountConfig, error) {
	settings, err := decodeUserSettings(record.Get("settings"))
	if err != nil {
		return clientAccountConfig{}, err
	}
	candidates, err := loadUserCandidates(app, record)
	if err != nil {
		return clientAccountConfig{}, err
	}

	cfg := clientAccountConfig{
		SchemaVersion: accountSchemaVersion,
		Blacklist:     append([]string(nil), settings.Blacklist...),
		Limits:        effectiveLimits(candidates),
	}
	if session := settings.Session; session != nil &&
		(session.MaxSessions > 0 || session.Compaction.KeepTokens > 0 || session.Compaction.TriggerRatio > 0) {
		cfg.Session = &clientSession{
			MaxSessions: session.MaxSessions,
			Compaction: clientCompaction{
				KeepTokens:   session.Compaction.KeepTokens,
				TriggerRatio: session.Compaction.TriggerRatio,
			},
		}
	}
	return cfg, nil
}

func loadUserCandidates(app core.App, record *core.Record) ([]modelCandidate, error) {
	ids := record.GetStringSlice("allowed_models")
	if len(ids) == 0 {
		return nil, nil
	}
	models, err := app.FindRecordsByIds("upstream_models", ids)
	if err != nil {
		return nil, err
	}

	providerIDs := make([]string, 0, len(models))
	seenProviders := make(map[string]bool, len(models))
	for _, model := range models {
		providerID := model.GetString("provider")
		if providerID == "" || seenProviders[providerID] {
			continue
		}
		seenProviders[providerID] = true
		providerIDs = append(providerIDs, providerID)
	}
	providers, err := app.FindRecordsByIds("upstream_providers", providerIDs)
	if err != nil {
		return nil, err
	}
	providerByID := make(map[string]*core.Record, len(providers))
	for _, provider := range providers {
		providerByID[provider.Id] = provider
	}

	candidates := make([]modelCandidate, 0, len(models))
	for _, model := range models {
		if !model.GetBool("enabled") {
			continue
		}
		provider := providerByID[model.GetString("provider")]
		if provider == nil || !provider.GetBool("enabled") {
			continue
		}
		extraBody, extraErr := decodeModelExtraBody(model.Get("extra_body"))
		if extraErr != nil {
			app.Logger().Warn("ignoring invalid model extra_body",
				"model", model.GetString("name"),
				"error", extraErr.Error(),
			)
		}
		candidates = append(candidates, modelCandidate{
			ModelID:           model.Id,
			ProviderID:        provider.Id,
			Provider:          provider.GetString("name"),
			Model:             model.GetString("name"),
			Priority:          model.GetInt("priority"),
			ContextWindow:     model.GetInt("context_window"),
			MaxOutputTokens:   model.GetInt("max_output_tokens"),
			ExtraBody:         extraBody,
			UnavailableUntil:  model.GetDateTime("unavailable_until").Time(),
			UnavailableReason: model.GetString("unavailable_reason"),
			LastError:         model.GetString("last_error"),
		})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Priority != candidates[j].Priority {
			return candidates[i].Priority < candidates[j].Priority
		}
		if candidates[i].Provider != candidates[j].Provider {
			return candidates[i].Provider < candidates[j].Provider
		}
		return candidates[i].Model < candidates[j].Model
	})
	return candidates, nil
}

func effectiveLimits(candidates []modelCandidate) clientAccountLimits {
	contextWindow := defaultContextWindow
	maxOutputTokens := defaultMaxOutputTokens

	minContext, minOutput := minCandidateLimits(candidates)
	if minContext > 0 {
		contextWindow = minContext
	}
	if minOutput > 0 {
		maxOutputTokens = minOutput
	}
	return clientAccountLimits{ContextWindow: contextWindow, MaxOutputTokens: maxOutputTokens}
}

func minCandidateLimits(candidates []modelCandidate) (contextWindow, maxOutputTokens int) {
	for _, candidate := range candidates {
		if candidate.ContextWindow > 0 && (contextWindow == 0 || candidate.ContextWindow < contextWindow) {
			contextWindow = candidate.ContextWindow
		}
		if candidate.MaxOutputTokens > 0 && (maxOutputTokens == 0 || candidate.MaxOutputTokens < maxOutputTokens) {
			maxOutputTokens = candidate.MaxOutputTokens
		}
	}
	return contextWindow, maxOutputTokens
}
