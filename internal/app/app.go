package app

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/llaoj/aist/internal/auth"
	"github.com/llaoj/aist/internal/buildinfo"
	"github.com/llaoj/aist/internal/commandguard"
	"github.com/llaoj/aist/internal/config"
	"github.com/llaoj/aist/internal/executor"
	"github.com/llaoj/aist/internal/fileedit"
	"github.com/llaoj/aist/internal/i18n"
	"github.com/llaoj/aist/internal/interactive"
	"github.com/llaoj/aist/internal/llm"
	"github.com/llaoj/aist/internal/memory"
	"github.com/llaoj/aist/internal/preferences"
	"github.com/llaoj/aist/internal/prompt"
	"github.com/llaoj/aist/internal/remote"
	"github.com/llaoj/aist/internal/session"
	"github.com/llaoj/aist/internal/state"
	"github.com/llaoj/aist/internal/token"
)

// Options configures the application composition root.
type Options struct {
	// State is the client state root. When nil, EnsureRoot(StateDir) prepares
	// one; a non-nil State still has its layout ensured.
	State *state.Root

	// StateDir overrides ~/.aist when State is nil. Tests use it to avoid
	// writing the real user state directory.
	StateDir string

	// Account supplies the server configuration and access tokens. When nil,
	// a default auth.Manager backed by the state root's token.json is used.
	Account AccountProvider

	// MemoryLoader is an optional dependency injection point.
	MemoryLoader memory.Loader
}

// AccountProvider supplies the account configuration and access tokens.
type AccountProvider interface {
	// Startup forces a refresh and returns the account configuration.
	Startup(ctx context.Context) (*remote.AuthResponse, error)
	// AccessTokenSource returns the dynamic token provider for proxy calls.
	AccessTokenSource() llm.AccessTokenSource
	// ProxyBaseURL returns the proxy endpoint derived from the server URL.
	ProxyBaseURL() string
}

// runtime holds dependencies derived from an account configuration snapshot.
type runtime struct {
	translator        *i18n.I18n
	prompts           prompt.Set
	serverModel       *llm.ServerModel
	commandGuard      *commandguard.Guard
	memoryManager     *memory.Manager
	fileEditor        *fileedit.Editor
	filePrompter      fileEditPrompter
	commandRunner     executor.CommandRunner
	sessionRepository session.Repository
	compactor         session.Compactor
}

// App is the composition root for configuration, LLM and sessions.
type App struct {
	settings *config.Settings
	runtime  runtime
}

// New loads the account configuration from the server and assembles runtime
// dependencies.
func New(ctx context.Context, opts Options) (*App, error) {
	stateStore := opts.State
	if stateStore == nil {
		var err error
		stateStore, err = state.EnsureRoot(opts.StateDir)
		if err != nil {
			return nil, err
		}
	} else if err := stateStore.Paths.Ensure(); err != nil {
		return nil, err
	}

	prefs, err := preferences.NewStore(preferences.StoreOptions{State: stateStore}).LoadOrInit()
	if err != nil {
		return nil, err
	}

	account := opts.Account
	if account == nil {
		defaultAccount, err := newDefaultAccountProvider(stateStore)
		if err != nil {
			return nil, err
		}
		account = defaultAccount
	}

	authResponse, err := account.Startup(ctx)
	if err != nil {
		return nil, err
	}
	settings, err := toSettings(authResponse.Config)
	if err != nil {
		return nil, err
	}
	settings.ProxyBaseURL = account.ProxyBaseURL()
	settings.Normalize()
	if err := settings.Validate(); err != nil {
		return nil, fmt.Errorf("server returned invalid account configuration: %w", err)
	}

	runtimeDeps, err := buildRuntime(ctx, settings, prefs.Language, account.AccessTokenSource(), stateStore, opts)
	if err != nil {
		return nil, err
	}

	return &App{
		settings: settings,
		runtime:  runtimeDeps,
	}, nil
}

func newDefaultAccountProvider(stateStore *state.Root) (AccountProvider, error) {
	store := token.NewStore(token.StoreOptions{State: stateStore})
	credentials, err := store.Load()
	if err != nil {
		return nil, err
	}
	client := remote.NewClient(remote.ClientOptions{ServerURL: credentials.ServerURL})
	manager := auth.NewManager(auth.ManagerOptions{
		Store:  store,
		Client: client,
	})
	return managerAccount{
		manager:      manager,
		proxyBaseURL: client.ProxyBaseURL(),
	}, nil
}

type managerAccount struct {
	manager      *auth.Manager
	proxyBaseURL string
}

func (a managerAccount) Startup(ctx context.Context) (*remote.AuthResponse, error) {
	return a.manager.Startup(ctx)
}

func (a managerAccount) AccessTokenSource() llm.AccessTokenSource {
	return a.manager
}

func (a managerAccount) ProxyBaseURL() string {
	return a.proxyBaseURL
}

// Settings returns the current account configuration snapshot.
func (a *App) Settings() *config.Settings {
	return a.settings.Clone()
}

// Translator returns the application translator.
func (a *App) Translator() *i18n.I18n {
	return a.runtime.translator
}

// NewSession creates a new chat session for the given mode.
func (a *App) NewSession(mode interactive.Mode) (*interactive.Session, error) {
	return a.NewSessionWithID("", mode)
}

// NewSessionWithID creates a new session (empty ID) or restores an existing
// session and prepares the interactive client around it.
func (a *App) NewSessionWithID(id string, mode interactive.Mode) (*interactive.Session, error) {
	cfg := a.settings
	sessionManager, err := a.runtime.newSessionManager(cfg, id)
	if err != nil {
		return nil, err
	}
	var pipeReader io.Reader
	if mode == interactive.ModePipe {
		pipeReader = os.Stdin
	}
	translator := a.runtime.translator
	return interactive.NewSession(interactive.SessionOptions{
		LLM:                  a.runtime.serverModel,
		SessionManager:       sessionManager,
		Translator:           translator,
		Mode:                 mode,
		PromptSet:            a.runtime.prompts,
		Version:              buildinfo.Default.Version,
		CommandGuard:         a.runtime.commandGuard,
		MemoryManager:        a.runtime.memoryManager,
		FileEditor:           a.runtime.fileEditor,
		FileEditConfirmer:    a.runtime.filePrompter,
		InputReader:          terminalInput{translator: translator},
		Renderer:             terminalRenderer{translator: translator},
		CommandConfirmer:     terminalConfirmer{translator: translator},
		ShellCommandExecutor: shellCommandExecutor{commandRunner: a.runtime.commandRunner},
		PipeReader:           pipeReader,
		Tools:                toolDefinitions(),
	}), nil
}

// ListSessions returns persisted sessions ordered newest first.
func (a *App) ListSessions(ctx context.Context) ([]session.SessionInfo, error) {
	if a.runtime.sessionRepository == nil {
		return nil, fmt.Errorf("session storage is unavailable")
	}
	return a.runtime.sessionRepository.List(ctx)
}
