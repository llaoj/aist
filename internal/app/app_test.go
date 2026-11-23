package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/llaoj/aist/internal/config"
	"github.com/llaoj/aist/internal/interactive"
	"github.com/llaoj/aist/internal/llm"
	"github.com/llaoj/aist/internal/memory"
	"github.com/llaoj/aist/internal/preferences"
	"github.com/llaoj/aist/internal/remote"
	"github.com/llaoj/aist/internal/state"
	"github.com/llaoj/aist/internal/token"
)

type fakeTokenSource struct{}

func (fakeTokenSource) AccessToken(context.Context) (string, error) { return "access-token", nil }
func (fakeTokenSource) RefreshAccessToken(context.Context) (string, error) {
	return "access-token", nil
}

type fakeAccount struct {
	response     *remote.AuthResponse
	err          error
	proxyBaseURL string
}

func (a fakeAccount) Startup(context.Context) (*remote.AuthResponse, error) {
	return a.response, a.err
}

func (a fakeAccount) AccessTokenSource() llm.AccessTokenSource {
	return fakeTokenSource{}
}

func (a fakeAccount) ProxyBaseURL() string {
	return a.proxyBaseURL
}

func testAccount(config remote.AccountConfig) fakeAccount {
	return fakeAccount{
		response: &remote.AuthResponse{
			Token: "access-token",
			Record: remote.AuthRecord{
				ID:    "user1",
				Email: "alice@example.com",
			},
			Config: config,
		},
		proxyBaseURL: "https://api.example.com/v1/proxy",
	}
}

func testAccountConfig() remote.AccountConfig {
	return remote.AccountConfig{
		SchemaVersion: 1,
		Limits: remote.LimitsInfo{
			ContextWindow:   128000,
			MaxOutputTokens: 8192,
		},
	}
}

func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "aist-app-test-home-")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("HOME", home)
	code := m.Run()
	_ = os.RemoveAll(home)
	os.Exit(code)
}

func TestNewAndSession(t *testing.T) {
	stateRoot, err := state.EnsureRoot(t.TempDir())
	if err != nil {
		t.Fatalf("EnsureRoot: %v", err)
	}
	if err := preferences.NewStore(preferences.StoreOptions{State: stateRoot}).Save(preferences.Data{
		Language: config.LanguageChinese,
	}); err != nil {
		t.Fatalf("Save preferences: %v", err)
	}

	a, err := New(context.Background(), Options{
		State:   stateRoot,
		Account: testAccount(testAccountConfig()),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := a.Translator().T("interactive.goodbye"); got != "再见！" {
		t.Fatalf("translator goodbye = %q", got)
	}
	if !strings.Contains(a.runtime.prompts.System(), "Chinese (Simplified)") {
		t.Fatalf("prompt did not follow local Chinese preference:\n%s", a.runtime.prompts.System())
	}
	if _, err := a.NewSession(interactive.ModeInteractive); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
}

func TestNewEnsuresProvidedState(t *testing.T) {
	customDir := filepath.Join(t.TempDir(), ".aist")
	stateRoot := state.NewFromPaths(state.NewPaths(customDir))
	if _, err := New(context.Background(), Options{
		State:   stateRoot,
		Account: testAccount(testAccountConfig()),
	}); err != nil {
		t.Fatalf("New: %v", err)
	}
	dirs := []string{
		stateRoot.Paths.Root,
		stateRoot.Paths.FileEditDir,
		stateRoot.Paths.BackupsDir("fileedit"),
		stateRoot.Paths.SessionsDir,
		stateRoot.Paths.MemoryDir,
	}
	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("Stat %s: %v", dir, err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Fatalf("%s mode = %o", dir, info.Mode().Perm())
		}
	}
}

func TestNewFailsWithoutAccount(t *testing.T) {
	_, err := New(context.Background(), Options{
		StateDir: t.TempDir(),
		Account:  fakeAccount{err: context.Canceled},
	})
	if err == nil {
		t.Fatal("expected account error")
	}
}

func TestNewRejectsInvalidConfig(t *testing.T) {
	accountConfig := remote.AccountConfig{
		SchemaVersion: 2,
		Limits:        remote.LimitsInfo{ContextWindow: 1000, MaxOutputTokens: 100},
	}
	if _, err := New(context.Background(), Options{
		StateDir: t.TempDir(),
		Account:  testAccount(accountConfig),
	}); err == nil {
		t.Fatal("expected invalid config error")
	}
}

func TestNewRejectsMissingProxy(t *testing.T) {
	account := testAccount(testAccountConfig())
	account.proxyBaseURL = ""
	if _, err := New(context.Background(), Options{
		StateDir: t.TempDir(),
		Account:  account,
	}); err == nil {
		t.Fatal("expected missing proxy error")
	}
}

func TestSettingsSnapshotIsIsolated(t *testing.T) {
	cfg := testAccountConfig()
	cfg.Blacklist = []string{"rm *"}
	a, err := New(context.Background(), Options{
		StateDir: t.TempDir(),
		Account:  testAccount(cfg),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	snapshot := a.Settings()
	snapshot.ContextWindow = 42
	snapshot.Blacklist[0] = "mutated"
	if a.Settings().ContextWindow != 128000 || a.Settings().Blacklist[0] != "rm *" {
		t.Fatal("mutating Settings() altered the stored snapshot")
	}
}

type staticMemoryLoader struct{}

func (staticMemoryLoader) Load(context.Context) (memory.LoadResult, error) {
	return memory.LoadResult{
		Memories: []memory.Entry{{
			Name:    "prod-k8s",
			Type:    "k8s",
			Summary: "Kubernetes cluster",
			Content: "- master: 10.0.0.1",
		}},
		Report: memory.Report{
			Loaded:   1,
			Bytes:    64,
			MaxBytes: 1024,
		},
	}, nil
}

func TestNewLoadsMemoryAndReportsInjection(t *testing.T) {
	a, err := New(context.Background(), Options{
		StateDir:     t.TempDir(),
		Account:      testAccount(testAccountConfig()),
		MemoryLoader: staticMemoryLoader{},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if a.runtime.memoryManager == nil {
		t.Fatal("MemoryManager is nil")
	}
	if report := a.runtime.memoryManager.Report(); report.Loaded != 1 {
		t.Fatalf("loaded = %d", report.Loaded)
	}
	if !strings.Contains(a.runtime.prompts.System(), "prod-k8s") {
		t.Fatalf("memory catalog not injected into system prompt: %s", a.runtime.prompts.System())
	}
}

func TestNewInjectsBlacklistIntoSystemPrompts(t *testing.T) {
	cfg := testAccountConfig()
	cfg.Blacklist = []string{"rm *", "shutdown"}

	a, err := New(context.Background(), Options{
		StateDir: t.TempDir(),
		Account:  testAccount(cfg),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if out := a.runtime.prompts.System(); !strings.Contains(out, "- rm *\n- shutdown\n") {
		t.Fatalf("prompt missing account blacklist:\n%s", out)
	}
}

func TestToolDefinitionsIncludeMemoryTools(t *testing.T) {
	names := map[string]bool{}
	for _, def := range toolDefinitions() {
		names[def.Name] = true
		if len(def.InputSchema) == 0 {
			t.Fatalf("tool %s has no schema", def.Name)
		}
	}
	for _, want := range []string{"shell_execute", "memory_search", "memory_save", "file_edit"} {
		if !names[want] {
			t.Fatalf("missing tool %s: %+v", want, names)
		}
	}
}

func TestNewDefaultAccountUsesCustomStateDir(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	t.Setenv(token.EnvServer, "")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `{
			"token": "refreshed-jwt",
			"record": {"id": "user1", "email": "alice@example.com"},
			"meta": {"config": {"schema_version": 1, "limits": {"context_window": 128000, "max_output_tokens": 8192}}}
		}`)
	}))
	defer srv.Close()

	customDir := filepath.Join(t.TempDir(), "custom-state")
	stateRoot, err := state.EnsureRoot(customDir)
	if err != nil {
		t.Fatalf("EnsureRoot: %v", err)
	}
	store := token.NewStore(token.StoreOptions{State: stateRoot})
	if err := store.Save(token.Credentials{
		ServerURL: srv.URL,
		Token:     "initial-jwt",
		Email:     "alice@example.com",
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	a, err := New(context.Background(), Options{StateDir: customDir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if a.Settings().ContextWindow != 128000 {
		t.Fatalf("context window = %d", a.Settings().ContextWindow)
	}
	if _, err := os.Stat(stateRoot.Paths.PreferencesFile); err != nil {
		t.Fatalf("PreferencesFile missing in custom state dir: %v", err)
	}

	saved, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if saved.Token != "refreshed-jwt" {
		t.Fatalf("saved token = %q, want refreshed-jwt", saved.Token)
	}
	if _, err := os.Stat(filepath.Join(homeDir, state.DefaultRootName)); !os.IsNotExist(err) {
		t.Fatalf("default home state dir was touched: %v", err)
	}
}
