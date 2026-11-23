package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/llaoj/aist/internal/executor"
	"github.com/llaoj/aist/internal/fileedit"
	"github.com/llaoj/aist/internal/i18n"
	"github.com/llaoj/aist/internal/interactive"
	"github.com/llaoj/aist/internal/tools"
	"github.com/llaoj/aist/internal/ui"
)

// terminalRenderer is the terminal implementation of interactive.Renderer.
type terminalRenderer struct {
	translator *i18n.I18n
}

func (r terminalRenderer) RenderWelcome(version string) {
	ui.PrintWelcome(
		r.translator.T("interactive.welcome"),
		version,
		r.translator.T("interactive.exit_hint"),
	)
}

func (r terminalRenderer) RenderUserMessage(text string) {
	fmt.Printf("%s:\n%s\n", r.translator.T("interactive.user_label"), strings.TrimRight(text, "\n"))
	ui.PrintRegionEnd()
}

func (r terminalRenderer) RenderAssistantMessage(content string) {
	if content == "" {
		return
	}
	content = ui.RenderThinkContent(content, r.translator.T("interactive.think_content"))
	content = strings.TrimSpace(content)
	content = ui.RemoveBlankLines(ui.RenderMarkdown(content))
	if content == "" {
		return
	}
	fmt.Printf("AI:\n%s\n", content)
	ui.PrintRegionEnd()
}

func (r terminalRenderer) RenderShellCommand(stepIndex string, cmd tools.ShellCommand) {
	ui.PrintShellCommand(stepIndex, cmd, r.translator)
}

func (r terminalRenderer) RenderFileChange(change *fileedit.Change) {
	fmt.Printf("%s:\n", r.translator.T("interactive.file_change"))
	if change.Reason != "" {
		fmt.Printf("%s: %s\n", r.translator.T("interactive.file_change_reason"), change.Reason)
	}
	fmt.Printf("%s: %s\n", r.translator.T("interactive.file_change_target"), change.LocationLabel())
	if diff := strings.TrimRight(change.Diff, "\n"); diff != "" {
		fmt.Println(ui.ColorizeDiff(diff))
	}
	ui.PrintRegionEnd()
}

func (r terminalRenderer) RenderExecutionOutput(text string) {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return
	}
	fmt.Println(text)
}

func (r terminalRenderer) RenderExecutionOutputHeader(string) {
	fmt.Printf("%s:\n", r.translator.T("interactive.execution_output"))
}

func (r terminalRenderer) RenderExecutionFinished(exitCode int, err error) {
	fmt.Println()
	if err != nil {
		color.Red(r.translator.T("executor.execute_failed", r.renderExecutionError(err)))
	} else if exitCode != 0 {
		color.Green(r.translator.T("executor.execute_success_exit_code", exitCode))
	} else {
		color.Green(r.translator.T("executor.execute_success"))
	}
	ui.PrintRegionEnd()
}

func (r terminalRenderer) renderExecutionError(err error) string {
	var timeoutErr *executor.TimeoutError
	if errors.As(err, &timeoutErr) {
		return r.translator.T("executor.command_timeout", timeoutErr.Timeout)
	}
	var stuckErr *executor.StuckExitError
	if errors.As(err, &stuckErr) {
		return r.translator.T("executor.command_stuck_exit", stuckErr.Grace)
	}
	var sshErr *executor.SSHUnreachableError
	if errors.As(err, &sshErr) {
		return r.translator.T("executor.ssh_unreachable", sshErr.Host)
	}
	if errors.Is(err, context.Canceled) {
		return r.translator.T("executor.command_canceled")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return r.translator.T("executor.command_deadline_exceeded")
	}
	return err.Error()
}

func (r terminalRenderer) RenderNotice(level interactive.NoticeLevel, message string) {
	message = strings.TrimRight(message, "\n")
	switch level {
	case interactive.NoticeLevelError:
		color.Red("%s", r.translator.T("error.general", message))
	case interactive.NoticeLevelWarning:
		color.Yellow("%s", message)
	case interactive.NoticeLevelSuccess:
		color.Green("%s", message)
	default:
		fmt.Println(message)
	}
	ui.PrintRegionEnd()
}

func (r terminalRenderer) RenderInterruptedNotice() {
	color.Yellow(r.translator.T("interactive.esc_interrupt"))
	ui.PrintRegionEnd()
}

func (r terminalRenderer) RenderBlacklistNotice(pattern string) {
	color.Red(r.translator.T("executor.blacklisted", pattern))
	fmt.Println(r.translator.T("executor.blacklist_hint"))
	ui.PrintRegionEnd()
}

func (r terminalRenderer) RenderTurnComplete() {
	color.Green(r.translator.T("interactive.turn_complete"))
	ui.PrintRegionEnd()
}

func (r terminalRenderer) RenderHistoryHeader(text string) {
	color.HiBlack("─────── %s ───────", text)
	ui.PrintRegionEnd()
}

func (r terminalRenderer) RenderToolSummary(text string) {
	color.HiBlack("[%s] %s", r.translator.T("tool.history.prefix"), text)
	ui.PrintRegionEnd()
}

func (r terminalRenderer) Sync() {
	_ = os.Stdout.Sync()
}

// terminalInput is the bubbletea-backed user input adapter.
type terminalInput struct {
	translator *i18n.I18n
}

func (i terminalInput) ReadInput() (string, error) {
	input, err := ui.PromptEditor(i.translator.T("interactive.input_prompt"), i.translator.T("interactive.input_key_hint"))
	if errors.Is(err, ui.ErrInterrupted) {
		return "", interactive.ErrInterrupted
	}
	return input, err
}

// terminalConfirmer adapts the selection widget to command confirmation policy.
type terminalConfirmer struct {
	translator *i18n.I18n
	promptFn   func(prompt string, translator *i18n.I18n) (ui.ConfirmDecision, error)
}

func (c terminalConfirmer) ConfirmCommand(ctx context.Context, cmdType tools.CommandType) (interactive.Decision, error) {
	if err := ctx.Err(); err != nil {
		return interactive.DecisionSkip, err
	}

	decision, err := c.ask(c.translator.T("executor.execute_prompt"))
	if err != nil {
		return decision, err
	}
	if decision != interactive.DecisionApprove {
		return decision, nil
	}

	if cmdType == tools.CommandTypeModify {
		decision, err = c.ask(c.translator.T("executor.modify_warning"))
		if err != nil {
			return decision, err
		}
		if decision != interactive.DecisionApprove {
			return decision, nil
		}
		return interactive.DecisionApprove, nil
	}
	return decision, nil
}

func (c terminalConfirmer) ask(prompt string) (interactive.Decision, error) {
	promptFn := c.promptFn
	if promptFn == nil {
		promptFn = ui.PromptCommandConfirm
	}
	result, err := promptFn(prompt, c.translator)
	if err != nil {
		if errors.Is(err, ui.ErrInterrupted) {
			return interactive.DecisionSkip, interactive.ErrInterrupted
		}
		return interactive.DecisionSkip, err
	}
	switch result {
	case ui.ConfirmApprove:
		return interactive.DecisionApprove, nil
	case ui.ConfirmRejectRemaining:
		return interactive.DecisionRejectRemaining, nil
	default:
		return interactive.DecisionSkip, nil
	}
}
