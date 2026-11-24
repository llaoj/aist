package interactive

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/llaoj/aist/internal/commandguard"
	"github.com/llaoj/aist/internal/llm"
	"github.com/llaoj/aist/internal/target"
	"github.com/llaoj/aist/internal/tools"
	"github.com/llaoj/aist/internal/util"
)

func (s *Session) prepareShellCommand(ctx context.Context, call llm.ToolCall, maxStep int) (target.Target, tools.ShellCommand, bool) {
	cmd, parseErr := tools.ParseShellCommand(toRawToolCall(call))
	if parseErr != nil {
		s.appendToolReport(ctx, call.ID, tools.ToolOutcomeFailed, "",
			resultField("Error", "invalid shell_execute arguments: "+parseErr.Error()),
			resultField("Detail", "No command was executed."),
		)
		return target.Target{}, tools.ShellCommand{}, false
	}

	stepLabel := fmt.Sprintf("%d/%d", cmd.StepIndex, maxStep)

	tgt, targetErr := target.New(cmd.Target)
	if targetErr != nil {
		display := cmd
		display.Target = sanitizeTargetLabel(cmd.Target)
		s.renderer.RenderShellCommand(stepLabel, display)
		detail := fmt.Sprintf("Invalid target %q: %v. No command was executed.", display.Target, targetErr)
		s.renderer.RenderNotice(NoticeLevelError, detail)
		s.appendShellReport(ctx, cmd, display.Target, tools.ToolOutcomeSkipped, detail)
		return target.Target{}, tools.ShellCommand{}, false
	}

	label := tgt.String()
	cmd.Target = label
	s.renderer.RenderShellCommand(stepLabel, cmd)

	if block := s.commandGuard.Check(cmd.CommandLine); block.Blocked() {
		switch block.Kind {
		case commandguard.KindBlacklist:
			s.renderer.RenderBlacklistNotice(block.Pattern)
			s.appendShellReport(ctx, cmd, label, tools.ToolOutcomeSkipped,
				"Command is blacklisted and cannot be executed. Blacklist pattern: "+block.Pattern)
		case commandguard.KindTUIEditor:
			notice := s.translator.T("executor.tui_editor_blocked", block.Program)
			s.renderer.RenderNotice(NoticeLevelWarning, notice)
			s.appendShellReport(ctx, cmd, label, tools.ToolOutcomeSkipped, fmt.Sprintf(
				"Blocked because %q is a full-screen interactive text editor. Use the file_edit tool instead (file_edit supports both remote SSH target and sudo=true).",
				block.Program,
			))
		default:
			notice := s.translator.T("executor.tui_monitor_blocked", block.Program)
			s.renderer.RenderNotice(NoticeLevelWarning, notice)
			s.appendShellReport(ctx, cmd, label, tools.ToolOutcomeSkipped, fmt.Sprintf(
				"Blocked because %q is a full-screen interactive pager or monitor. Re-run with non-interactive flags instead (for example --no-pager, cat/head/tail, or top -b -n 1 on Linux / top -l 1 on macOS).",
				block.Program,
			))
		}
		return target.Target{}, tools.ShellCommand{}, false
	}

	if s.commandConfirmer == nil {
		s.appendShellReport(ctx, cmd, label, tools.ToolOutcomeSkipped, "Command execution skipped: confirmation is unavailable.")
		return target.Target{}, tools.ShellCommand{}, false
	}

	return tgt, cmd, true
}

func (s *Session) promptShellCommand(ctx context.Context, cmd tools.ShellCommand) (Decision, error) {
	decision, err := s.commandConfirmer.ConfirmCommand(ctx, cmd.Type)
	if err != nil {
		return DecisionUnknown, err
	}
	if decision != DecisionUnknown {
		s.autoToolCalls = 0
	}

	switch decision {
	case DecisionUnknown:
		s.appendShellReport(ctx, cmd, cmd.Target, tools.ToolOutcomeSkipped, "Command execution skipped: no confirmation decision.")
	case DecisionSkip:
		s.appendShellReport(ctx, cmd, cmd.Target, tools.ToolOutcomeSkipped, "Command execution skipped by user.")
	case DecisionRejectRemaining:
		s.appendShellReport(ctx, cmd, cmd.Target, tools.ToolOutcomeRejected, "User rejected execution of this and all subsequent commands.")
	}
	return decision, nil
}

func (s *Session) executeShellCommand(ctx context.Context, cmd tools.ShellCommand, tgt target.Target) {
	label := cmd.Target
	if s.shellCommandExecutor == nil {
		s.appendShellReport(ctx, cmd, label, tools.ToolOutcomeFailed, "Command execution failed: executor is unavailable.")
		return
	}

	s.renderer.RenderExecutionOutputHeader(label)
	result, execErr := s.shellCommandExecutor.Execute(ctx, ShellCommandRequest{Target: tgt, Command: cmd.CommandLine})
	output := strings.TrimSpace(result.Output)
	if output == "" {
		output = "(no output)"
		s.renderer.RenderExecutionOutput(output)
	} else if !result.OutputStreamed {
		s.renderer.RenderExecutionOutput(output)
	}
	s.renderer.RenderExecutionFinished(result.ExitCode, execErr)

	resultLabel := strings.TrimSpace(result.Target)
	if resultLabel == "" {
		resultLabel = label
	}

	outcome := tools.ToolOutcomeSuccess
	fields := []toolResultField{resultField("Target", resultLabel)}
	if execErr != nil {
		outcome = tools.ToolOutcomeFailed
		fields = append(fields, resultField("Error", util.TruncatePrefix(execErr.Error(), maxResultErrorBytes)))
	} else {
		fields = append(fields, resultField("Exit Code", strconv.Itoa(result.ExitCode)))
	}
	fields = append(fields, resultField("Command", cmd.CommandLine))

	s.appendToolResult(ctx, cmd.ToolCallID, TruncateContent(renderToolResult(outcome, fields, output), MaxTurnChars), outcome)
}

func (s *Session) appendShellReport(ctx context.Context, cmd tools.ShellCommand, label string, outcome tools.ToolOutcome, detail string) {
	s.appendToolReport(ctx, cmd.ToolCallID, outcome, "",
		resultField("Target", label),
		resultField("Detail", detail),
		resultField("Command", cmd.CommandLine),
	)
}

func sanitizeTargetLabel(raw string) string {
	label := strings.TrimSpace(raw)
	if label == "" {
		return "invalid"
	}
	label = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '?'
		}
		return r
	}, label)
	if runes := []rune(label); len(runes) > 80 {
		label = string(runes[:80]) + "..."
	}
	return label
}
