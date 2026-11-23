package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/llaoj/aist/internal/interactive"
	"github.com/llaoj/aist/internal/remote"
)

func TestNewSessionSkipsEmptyAndPersistsAfterConversation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "test-model",
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "ok"}},
			},
			"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 2, "total_tokens": 12},
		})
	}))
	defer upstream.Close()

	stateDir := filepath.Join(t.TempDir(), ".aist")
	accountConfig := testAccountConfig()
	accountConfig.Session = &remote.SessionConfig{MaxSessions: 5}
	account := testAccount(accountConfig)
	account.proxyBaseURL = upstream.URL

	a, err := New(context.Background(), Options{
		StateDir: stateDir,
		Account:  account,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := a.NewSession(interactive.ModeInteractive); err != nil {
		t.Fatalf("NewSession empty: %v", err)
	}

	summaries, err := a.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions after empty: %v", err)
	}
	if len(summaries) != 0 {
		t.Fatalf("summaries after empty = %+v, want 0", summaries)
	}

	activeManager, err := a.runtime.newSessionManager(a.settings, "")
	if err != nil {
		t.Fatalf("newSessionManager: %v", err)
	}
	if _, err := activeManager.AppendUser(context.Background(), "check disk"); err != nil {
		t.Fatalf("AppendUser: %v", err)
	}

	summaries, err = a.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("summaries = %+v, want 1", summaries)
	}
	id := summaries[0].ID
	for _, name := range []string{"session.json", "messages.jsonl"} {
		if _, err := os.Stat(filepath.Join(stateDir, "sessions", id, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}

	restored, err := a.NewSessionWithID(id, interactive.ModeInteractive)
	if err != nil {
		t.Fatalf("NewSessionWithID: %v", err)
	}
	if restored == nil {
		t.Fatal("restored session is nil")
	}
}
