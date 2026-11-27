package app

import (
	"context"
	"errors"
	"os"
	"sync"

	"github.com/llaoj/aist/internal/executor"
	"github.com/llaoj/aist/internal/i18n"
	"github.com/llaoj/aist/internal/interactive"
	"github.com/llaoj/aist/internal/ui"
)

type callHooks struct {
	mu         sync.Mutex
	translator *i18n.I18n
	stop       func() error
}

func (h *callHooks) BeforeCall(cancel context.CancelFunc) {
	stop := ui.StartSpinner(
		h.translator.T("interactive.thinking"),
		h.translator.T("interactive.thinking_key_hint"),
		cancel,
	)
	h.mu.Lock()
	h.stop = stop
	h.mu.Unlock()
}

func (h *callHooks) AfterCall() error {
	h.mu.Lock()
	stop := h.stop
	h.stop = nil
	h.mu.Unlock()
	if stop != nil {
		return stop()
	}
	return nil
}

// shellExecutor adapts executor.CommandRunner to the interactive shell port.
type shellCommandExecutor struct {
	commandRunner executor.CommandRunner
}

func (e shellCommandExecutor) Execute(ctx context.Context, req interactive.ShellCommandRequest) (interactive.ShellCommandResult, error) {
	if e.commandRunner == nil {
		return interactive.ShellCommandResult{Target: req.Target.String(), Command: req.Command}, errors.New("executor is unavailable")
	}
	result, err := e.commandRunner.Execute(ctx, executor.CommandRequest{
		Target:  req.Target,
		Command: req.Command,
		Display: os.Stdout,
	})
	return interactive.ShellCommandResult{
		Target:         result.Target,
		Command:        result.Command,
		Output:         result.Output,
		OutputStreamed: result.OutputStreamed,
		ExitCode:       result.ExitCode,
		Duration:       result.Duration,
	}, err
}
