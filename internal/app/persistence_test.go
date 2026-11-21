package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/llaoj/aist/internal/interactive"
	"github.com/llaoj/aist/internal/remote"
)

func TestNewSessionPersistsAndResumes(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), ".aist")
	accountConfig := testAccountConfig()
	accountConfig.Session = &remote.SessionConfig{MaxSessions: 5}
	a, err := New(context.Background(), Options{
		StateDir: stateDir,
		Account:  testAccount(accountConfig),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := a.NewSession(interactive.ModeInteractive); err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	summaries, err := a.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("summaries = %+v", summaries)
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
