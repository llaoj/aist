package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/llaoj/aist/internal/app"
	"github.com/llaoj/aist/internal/interactive"
	sessionpkg "github.com/llaoj/aist/internal/session"
	sessionfile "github.com/llaoj/aist/internal/session/file"
	"github.com/llaoj/aist/internal/state"
	"github.com/llaoj/aist/internal/token"
	"github.com/spf13/cobra"
)

func TestSkipsStateInit(t *testing.T) {
	root := &cobra.Command{Use: "aist"}
	help := &cobra.Command{Use: "help"}
	complete := &cobra.Command{Use: cobra.ShellCompRequestCmd + " [command-line]"}
	normal := &cobra.Command{Use: "run"}
	root.AddCommand(help, complete, normal)

	if !skipsStateInit(help) {
		t.Fatal("help should skip state initialization")
	}
	if !skipsStateInit(complete) {
		t.Fatal("__complete should skip state initialization")
	}
	if skipsStateInit(normal) {
		t.Fatal("normal command should initialize state")
	}
}

func TestRootContinueFlagRegistered(t *testing.T) {
	flag := rootCmd.Flags().Lookup("continue")
	if flag == nil {
		t.Fatal("expected --continue flag on rootCmd")
	}
	if flag.Shorthand != "c" {
		t.Fatalf("shorthand = %q, want c", flag.Shorthand)
	}
}

func TestOpenSessionContinueBehavior(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token":  "refreshed-jwt",
			"record": map[string]any{"id": "u1", "email": "alice@example.com"},
			"meta": map[string]any{
				"config": map[string]any{
					"schema_version": 1,
					"limits":         map[string]any{"context_window": 128000, "max_output_tokens": 8192},
				},
			},
		})
	}))
	defer srv.Close()

	stateRoot, err := state.EnsureRoot(t.TempDir())
	if err != nil {
		t.Fatalf("EnsureRoot: %v", err)
	}
	if err := token.NewStore(token.StoreOptions{State: stateRoot}).Save(token.Credentials{
		ServerURL: srv.URL,
		Token:     "jwt",
		Email:     "alice@example.com",
	}); err != nil {
		t.Fatalf("Save token: %v", err)
	}

	ctx := context.Background()
	application, err := app.New(ctx, app.Options{State: stateRoot})
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}

	// No saved sessions: -c falls back cleanly to a new session.
	chatSession, resumed, err := openSession(ctx, application, interactive.ModeInteractive, true, false)
	if err != nil {
		t.Fatalf("openSession empty: %v", err)
	}
	if chatSession == nil || resumed {
		t.Fatalf("empty openSession = (%v, %v), want non-nil session and resumed=false", chatSession != nil, resumed)
	}

	// Create two persisted sessions; -c should resume the most recent one.
	repo := sessionfile.NewRepository(sessionfile.RepositoryOptions{State: stateRoot})
	older := &sessionpkg.Session{
		ID:        "sess-older",
		Title:     "older",
		CreatedAt: time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC),
	}
	newer := &sessionpkg.Session{
		ID:        "sess-newer",
		Title:     "newer",
		CreatedAt: time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC),
	}
	if err := repo.Create(ctx, older); err != nil {
		t.Fatalf("Create older: %v", err)
	}
	if err := repo.AppendMessage(ctx, older.ID, sessionpkg.Message{ID: "m1", Seq: 1, Role: sessionpkg.RoleUser, Content: "old"}); err != nil {
		t.Fatalf("AppendMessage older: %v", err)
	}
	if err := repo.Create(ctx, newer); err != nil {
		t.Fatalf("Create newer: %v", err)
	}
	if err := repo.AppendMessage(ctx, newer.ID, sessionpkg.Message{ID: "m2", Seq: 1, Role: sessionpkg.RoleUser, Content: "new"}); err != nil {
		t.Fatalf("AppendMessage newer: %v", err)
	}

	chatSession, resumed, err = openSession(ctx, application, interactive.ModeInteractive, true, false)
	if err != nil {
		t.Fatalf("openSession latest: %v", err)
	}
	if chatSession == nil || !resumed {
		t.Fatalf("latest openSession = (%v, %v), want non-nil session and resumed=true", chatSession != nil, resumed)
	}
}
