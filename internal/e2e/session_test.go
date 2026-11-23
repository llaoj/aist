package e2e

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/llaoj/aist/internal/interactive"
	"github.com/llaoj/aist/internal/llm"
	"github.com/llaoj/aist/internal/session"
	sessionfile "github.com/llaoj/aist/internal/session/file"
)

type compactionAdapter struct {
	requests []session.CompactionRequest
}

func (s *compactionAdapter) Compact(_ context.Context, req session.CompactionRequest) (session.CompactionResult, error) {
	s.requests = append(s.requests, req)
	return session.CompactionResult{
		Summary: `<compressed_history priority="high">kept facts</compressed_history>`,
		Model:   "scripted-summary",
		Usage:   session.Usage{TotalTokens: 3},
	}, nil
}

func TestSessionPersistsResumesAndCompacts(t *testing.T) {
	repo := sessionfile.NewRepository(sessionfile.RepositoryOptions{Root: filepath.Join(t.TempDir(), ".aist")})
	compactor := &compactionAdapter{}
	service := session.NewManager(session.ManagerOptions{
		Repository: repo,
		Compactor:  compactor,
		Settings: session.Settings{
			ContextWindowLimit:     100,
			RetainTailTokens:       1,
			CompactionTriggerRatio: 0.1,
			CompactionEnabled:      true,
		},
	})
	if _, err := service.Create(context.Background()); err != nil {
		t.Fatalf("Create: %v", err)
	}

	model := &scriptedModel{responses: []*llm.CompletionResponse{
		{Content: "first answer", Usage: llm.Usage{TotalTokens: 4000}},
		{Content: "second answer", Usage: llm.Usage{TotalTokens: 4200}},
		{Content: "third answer", Usage: llm.Usage{TotalTokens: 4400}},
	}}
	interactiveSession := interactive.NewSession(interactive.SessionOptions{
		LLM:            model,
		SessionManager: service,
		Mode:           interactive.ModeInteractive,
		PromptSet:      staticPrompts{},
		InputReader:    interruptedInput{},
	})
	for _, question := range []string{"first question", "second question", "third question"} {
		if err := interactiveSession.Run(question); err != nil {
			t.Fatalf("Run(%q): %v", question, err)
		}
	}

	id := service.Current().ID
	loaded, err := repo.Load(context.Background(), id)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Messages) < 6 {
		t.Fatalf("messages = %+v", loaded.Messages)
	}
	if len(loaded.Compactions) == 0 {
		t.Fatal("expected token-triggered compaction")
	}
	if loaded.Compactions[0].Model != "scripted-summary" {
		t.Fatalf("compaction model = %q, want scripted-summary", loaded.Compactions[0].Model)
	}
	if len(compactor.requests) == 0 {
		t.Fatal("compactor was not called")
	}

	lastRequest := model.requests[len(model.requests)-1]
	foundSummary := false
	for _, message := range lastRequest.Messages {
		if strings.Contains(llm.DerefPtr(message.Content), "<compressed_history") {
			foundSummary = true
		}
	}
	if !foundSummary {
		t.Fatalf("model request missing compaction summary: %+v", lastRequest.Messages)
	}

	restored := session.NewManager(session.ManagerOptions{
		Repository: repo,
		Settings:   session.Settings{CompactionEnabled: false},
	})
	if _, err := restored.Load(context.Background(), id); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if len(restored.DisplayMessages()) != len(loaded.Messages) {
		t.Fatalf("restored display = %d want %d", len(restored.DisplayMessages()), len(loaded.Messages))
	}
}
