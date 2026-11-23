package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/llaoj/aist/internal/config"
	"github.com/llaoj/aist/internal/interactive"
	"github.com/llaoj/aist/internal/llm"
	"github.com/llaoj/aist/internal/prompt"
	"github.com/llaoj/aist/internal/session"
)

func (r runtime) newSessionManager(cfg *config.Settings, id string) (*session.Manager, error) {
	if r.sessionRepository == nil {
		return nil, errors.New("session storage is unavailable")
	}
	contextWindow, maxOutput := modelLimits(cfg)
	cwd, _ := os.Getwd()
	sessionManager := session.NewManager(session.ManagerOptions{
		Repository: r.sessionRepository,
		Compactor:  r.compactor,
		Settings: session.Settings{
			RetainSessions:         cfg.Session.MaxSessions,
			RetainTailTokens:       cfg.Session.Compaction.KeepTokens,
			CompactionTriggerRatio: cfg.Session.Compaction.TriggerRatio,
			CompactionEnabled:      true,
			FallbackContextWindow:  config.DefaultContextWindow,
			ContextWindowLimit:     contextWindow,
			MaxOutputTokens:        maxOutput,
			Cwd:                    cwd,
		},
	})
	if id == "" {
		if _, err := sessionManager.Create(context.Background()); err != nil {
			return nil, err
		}
		return sessionManager, nil
	}
	if _, err := sessionManager.Load(context.Background(), id); err != nil {
		return nil, err
	}
	return sessionManager, nil
}

func modelLimits(cfg *config.Settings) (contextWindow, maxOutput int) {
	return cfg.EffectiveContextWindow(), cfg.EffectiveMaxOutputTokens()
}

type llmCompactor struct {
	serverModel *llm.ServerModel
	prompts     prompt.Set
}

func (s llmCompactor) Compact(ctx context.Context, req session.CompactionRequest) (session.CompactionResult, error) {
	if s.serverModel == nil {
		return session.CompactionResult{}, errors.New("no LLM available for summarization")
	}
	raw, err := json.MarshalIndent(interactive.ToLLMMessages(req.Messages), "", "  ")
	if err != nil {
		return session.CompactionResult{}, err
	}
	parts := make([]string, 0, 2)
	if strings.TrimSpace(req.PriorSummary) != "" {
		parts = append(parts, "Prior high-priority summary (carry forward its conclusions and constraints):\n"+req.PriorSummary)
	}
	parts = append(parts, "Conversation messages to summarize (JSON):\n"+string(raw))

	response, err := s.serverModel.Call(ctx, llm.CompletionRequest{
		Messages: []llm.ChatMessage{
			{Role: "system", Content: llm.StringPtr(compactionPrompt(s.prompts))},
			{Role: "user", Content: llm.StringPtr(strings.Join(parts, "\n\n"))},
		},
		Temperature: llm.Float64Ptr(0.0),
		TopP:        llm.Float64Ptr(0.1),
	})
	if err != nil {
		return session.CompactionResult{}, err
	}
	return session.CompactionResult{
		Summary: response.Content,
		Model:   response.Model,
		Usage:   interactive.ToSessionUsage(response.Usage),
	}, nil
}

func compactionPrompt(provider prompt.Set) string {
	if provider == nil {
		return ""
	}
	return provider.Compaction()
}
