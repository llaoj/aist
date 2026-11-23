package app

import (
	"context"
	"os"

	"github.com/llaoj/aist/internal/commandguard"
	"github.com/llaoj/aist/internal/config"
	"github.com/llaoj/aist/internal/executor"
	"github.com/llaoj/aist/internal/fileedit"
	"github.com/llaoj/aist/internal/i18n"
	"github.com/llaoj/aist/internal/llm"
	"github.com/llaoj/aist/internal/memory"
	memoryfile "github.com/llaoj/aist/internal/memory/file"
	memstore "github.com/llaoj/aist/internal/memory/store"
	"github.com/llaoj/aist/internal/prompt"
	sessionfile "github.com/llaoj/aist/internal/session/file"
	"github.com/llaoj/aist/internal/state"
	"github.com/llaoj/aist/internal/tools"
)

func buildRuntime(
	ctx context.Context,
	cfg *config.Settings,
	language string,
	tokenSource llm.AccessTokenSource,
	stateStore *state.Root,
	opts Options,
) (runtime, error) {
	translator := i18n.New(language)
	serverModel, err := buildServerModel(cfg, tokenSource, &callHooks{translator: translator})
	if err != nil {
		return runtime{}, err
	}
	home, _ := os.UserHomeDir()
	memoryManager, memoryCatalog := buildMemoryManager(ctx, opts, translator, stateStore)
	blacklist := cfg.EffectiveBlacklist()
	prompts := prompt.NewBuilder(prompt.BuilderOptions{
		Language:  language,
		Memory:    memoryCatalog,
		Blacklist: blacklist,
	})
	rt := runtime{
		translator:    translator,
		prompts:       prompts,
		serverModel:   serverModel,
		commandGuard:  commandguard.New(blacklist),
		memoryManager: memoryManager,
		fileEditor: fileedit.NewEditor(fileedit.EditorOptions{
			HomeDir: home,
			State:   stateStore,
			Runner:  executor.NewScriptRunner(executor.ScriptRunnerOptions{}),
		}),
		filePrompter:  fileEditPrompter{translator: translator},
		commandRunner: executor.NewCommandRouter(executor.NewLocal(), executor.NewSSH(executor.SSHOptions{})),
	}
	rt.sessionRepository = sessionfile.NewRepository(sessionfile.RepositoryOptions{State: stateStore})
	rt.compactor = llmCompactor{serverModel: serverModel, prompts: prompts}
	return rt, nil
}

func buildMemoryManager(ctx context.Context, opts Options, translator *i18n.I18n, stateStore *state.Root) (*memory.Manager, *memory.CatalogProvider) {
	loader := opts.MemoryLoader
	if loader == nil {
		loader = memoryfile.NewLoader(memoryfile.LoadOptions{
			Dir:      stateStore.Paths.MemoryDir,
			MaxBytes: memory.DefaultMaxBytes,
		})
	}

	memorySnapshot := memstore.NewSnapshot(loader)
	_ = memorySnapshot.Reload(ctx)

	catalogProvider := memory.NewCatalogProvider(memorySnapshot.Snapshot(), memory.CatalogProviderOptions{
		MaxBytes: memory.DefaultMaxBytes,
	})
	memoryManager := memory.NewManager(memory.ManagerOptions{
		Snapshot: memorySnapshot,
		Writer: memoryfile.NewWriter(memoryfile.WriterOptions{
			Dir:      stateStore.Paths.MemoryDir,
			MaxBytes: memory.DefaultMaxBytes,
		}),
		Notifier: memoryNotifier{translator: translator},
		MaxBytes: memory.DefaultMaxBytes,
	})
	return memoryManager, catalogProvider
}

func buildServerModel(
	cfg *config.Settings,
	tokenSource llm.AccessTokenSource,
	hooks llm.Hooks,
) (*llm.ServerModel, error) {
	return llm.NewServerModel(llm.ServerModelOptions{
		BaseURL:           cfg.EffectiveProxyBaseURL(),
		AccessTokenSource: tokenSource,
		MaxTokens:         cfg.EffectiveMaxOutputTokens(),
		Timeout:           config.DefaultTimeout,
		HeaderTimeout:     config.DefaultHeaderTimeout,
		ChunkTimeout:      config.DefaultChunkTimeout,
		Hooks:             hooks,
	})
}

func toolDefinitions() []llm.ToolDefinition {
	defs := []tools.Definition{
		tools.ShellExecuteDefinition(),
		tools.MemorySearchDefinition(),
		tools.MemorySaveDefinition(),
		tools.FileEditDefinition(),
	}
	out := make([]llm.ToolDefinition, 0, len(defs))
	for _, def := range defs {
		out = append(out, llm.ToolDefinition{
			Name:        def.Name,
			Description: def.Description,
			InputSchema: def.InputSchema,
		})
	}
	return out
}
