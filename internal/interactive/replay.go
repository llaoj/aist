package interactive

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/llaoj/aist/internal/commandguard"
	"github.com/llaoj/aist/internal/fileedit"
	"github.com/llaoj/aist/internal/session"
	"github.com/llaoj/aist/internal/target"
	"github.com/llaoj/aist/internal/tools"
	"github.com/llaoj/aist/internal/util"
)

type replayedToolCall struct {
	call    session.ToolCall
	maxStep int
}

// ReplayHistory renders the durable display view before an interactive loop,
// reproducing the live terminal layout without calling the model or re-running tools.
func (s *Session) ReplayHistory() {
	if s.sessionManager == nil {
		return
	}
	messages := s.sessionManager.DisplayMessages()
	if len(messages) == 0 {
		return
	}

	state := s.sessionManager.Current()
	latest := state.LatestCompaction()
	calls := make(map[string]replayedToolCall)
	toolsDispatched := false

	s.renderer.RenderHistoryHeader(s.translator.T("sessions.history_header"))
	for _, message := range messages {
		switch message.Role {
		case session.RoleUser:
			toolsDispatched = false
			if question, pipeBody, hasPipe := splitPipedUserMessage(message.Content); hasPipe {
				s.renderer.RenderNotice(NoticeLevelInfo, s.translator.T("pipe.input_loaded", countPipeLines(pipeBody)))
				s.renderer.RenderUserMessage(question)
			} else {
				s.renderer.RenderUserMessage(message.Content)
			}
		case session.RoleAssistant:
			s.renderer.RenderAssistantMessage(message.Content)
			if len(message.ToolCalls) > 0 {
				toolsDispatched = true
				maxStep := maxSessionStepIndex(message.ToolCalls)
				for _, call := range message.ToolCalls {
					calls[call.ID] = replayedToolCall{call: call, maxStep: maxStep}
				}
			} else if toolsDispatched {
				s.renderer.RenderTurnComplete()
				toolsDispatched = false
			}
		case session.RoleTool:
			entry, found := calls[message.ToolCallID]
			if !found {
				continue
			}
			s.replayToolMessage(message, entry, &toolsDispatched)
		}

		if latest != nil && message.Seq == latest.CoversToSeq {
			s.renderer.RenderNotice(NoticeLevelInfo, s.translator.T("sessions.compacted"))
		}
	}
	s.renderer.Sync()
}

func (s *Session) replayToolMessage(message session.Message, entry replayedToolCall, toolsDispatched *bool) {
	switch entry.call.Function.Name {
	case tools.ToolShellExecute:
		s.replayShellCommand(message, entry, toolsDispatched)
	case tools.ToolFileEdit:
		s.replayFileEdit(message, entry, toolsDispatched)
	case tools.ToolMemorySave:
		s.replayMemorySave(message, entry)
	case tools.ToolMemorySearch:
	default:
		display, err := tools.DecodeInvocation(entry.call.Function.Name, entry.call.Function.Arguments)
		if err != nil {
			return
		}
		s.renderer.RenderToolSummary(display.FormatResult(message.ToolOutcome, s.translator))
	}
}

func (s *Session) replayShellCommand(message session.Message, entry replayedToolCall, toolsDispatched *bool) {
	cmd, err := tools.ParseShellCommand(tools.RawToolCall{
		ID:            entry.call.ID,
		ToolName:      entry.call.Function.Name,
		ArgumentsJSON: entry.call.Function.Arguments,
	})
	if err != nil {
		return
	}

	stepIndex := max(cmd.StepIndex, 1)
	maxStep := max(entry.maxStep, stepIndex)
	stepLabel := fmt.Sprintf("%d/%d", stepIndex, maxStep)

	parsed := parseToolResult(message.Content)
	detail := parsed.Fields["Detail"]
	if detail == rejectedRemainingToolResult || detail == followUpLimitToolResult || detail == interruptedToolResult {
		return
	}

	tgt, targetErr := target.New(cmd.Target)
	if targetErr != nil {
		display := cmd
		display.Target = sanitizeTargetLabel(cmd.Target)
		s.renderer.RenderShellCommand(stepLabel, display)
		s.renderer.RenderNotice(NoticeLevelError, util.FirstNonEmpty(detail, fmt.Sprintf("Invalid target %q: %v. No command was executed.", display.Target, targetErr)))
		return
	}

	label := util.FirstNonEmpty(parsed.Fields["Target"], tgt.String())
	cmd.Target = label
	s.renderer.RenderShellCommand(stepLabel, cmd)

	if parsed.HasOutput || (!parsed.Structured && (message.ToolOutcome == session.ToolOutcomeSuccess || message.ToolOutcome == session.ToolOutcomeFailed || message.ToolOutcome == "")) {
		output := strings.TrimSpace(parsed.Output)
		if output == "" {
			output = "(no output)"
		}
		s.renderer.RenderExecutionOutputHeader(label)
		s.renderer.RenderExecutionOutput(output)

		exitCode, _ := strconv.Atoi(parsed.Fields["Exit Code"])
		var execErr error
		if errText := parsed.Fields["Error"]; errText != "" {
			execErr = errors.New(errText)
		} else if message.ToolOutcome == session.ToolOutcomeFailed {
			execErr = errors.New(s.translator.T("tool.status.failed"))
		}
		s.renderer.RenderExecutionFinished(exitCode, execErr)
		return
	}

	if pattern, matched := strings.CutPrefix(detail, "Command is blacklisted and cannot be executed. Blacklist pattern: "); matched {
		s.renderer.RenderBlacklistNotice(pattern)
		return
	}
	if block := s.commandGuard.Check(cmd.CommandLine); block.Blocked() {
		switch block.Kind {
		case commandguard.KindBlacklist:
			s.renderer.RenderBlacklistNotice(block.Pattern)
		case commandguard.KindTUIEditor:
			s.renderer.RenderNotice(NoticeLevelWarning, s.translator.T("executor.tui_editor_blocked", block.Program))
		default:
			s.renderer.RenderNotice(NoticeLevelWarning, s.translator.T("executor.tui_monitor_blocked", block.Program))
		}
		return
	}

	switch message.ToolOutcome {
	case session.ToolOutcomeRejected:
		s.renderer.RenderNotice(NoticeLevelInfo, s.translator.T("interactive.reject_remaining_hint"))
		*toolsDispatched = false
	case session.ToolOutcomeFailed:
		if errText := util.FirstNonEmpty(parsed.Fields["Error"], detail); errText != "" {
			s.renderer.RenderNotice(NoticeLevelError, errText)
		}
	default:
		s.renderer.RenderToolSummary(cmd.FormatResult(message.ToolOutcome, s.translator))
	}
}

func (s *Session) replayFileEdit(message session.Message, entry replayedToolCall, toolsDispatched *bool) {
	args, err := tools.ParseFileEditArgs(entry.call.Function.Arguments)
	if err != nil {
		return
	}
	parsed := parseToolResult(message.Content)
	detail := parsed.Fields["Detail"]
	if detail == rejectedRemainingToolResult || detail == followUpLimitToolResult || detail == interruptedToolResult {
		return
	}
	tgt, err := target.New(args.Target)
	if err != nil {
		return
	}
	if message.ToolOutcome == session.ToolOutcomeFailed && detail == "No changes were written." {
		return
	}

	change := &fileedit.Change{
		Target:  tgt,
		Path:    args.Path,
		Reason:  args.Reason,
		UseSudo: args.UseSudo,
		Diff:    fileedit.RenderSnippetDiff(tgt, args.Path, args.OldContent, args.NewContent),
	}
	s.renderer.RenderFileChange(change)

	switch message.ToolOutcome {
	case session.ToolOutcomeSuccess:
		s.renderer.RenderNotice(NoticeLevelSuccess, s.translator.T("file_edit.updated", change.LocationLabel()))
	case session.ToolOutcomeRejected:
		s.renderer.RenderNotice(NoticeLevelInfo, s.translator.T("interactive.reject_remaining_hint"))
		*toolsDispatched = false
	case session.ToolOutcomeSkipped:
		s.renderer.RenderToolSummary(args.FormatResult(message.ToolOutcome, s.translator))
	case session.ToolOutcomeFailed:
		if errText := parsed.Fields["Error"]; errText != "" {
			s.renderer.RenderNotice(NoticeLevelError, errText)
		}
	}
}

func (s *Session) replayMemorySave(message session.Message, entry replayedToolCall) {
	if message.ToolOutcome != session.ToolOutcomeSuccess {
		return
	}
	args, err := tools.ParseMemorySaveArgs(entry.call.Function.Arguments)
	if err != nil {
		return
	}
	key := "memory.saved_updated"
	switch {
	case strings.Contains(message.Content, " created successfully "):
		key = "memory.saved_created"
	case strings.Contains(message.Content, " replaced successfully "):
		key = "memory.saved_replaced"
	}
	notice := s.translator.T(key, args.Name)
	summary := strings.TrimSpace(args.Summary)
	if _, persistedSummary, hasSummary := strings.Cut(message.Content, " Summary: "); hasSummary {
		summary = strings.TrimSpace(persistedSummary)
	}
	if summary != "" {
		notice += " — " + summary
	}
	s.renderer.RenderNotice(NoticeLevelSuccess, notice)
}

func maxSessionStepIndex(calls []session.ToolCall) int {
	maxStep := 0
	for _, call := range calls {
		if call.Function.Name != tools.ToolShellExecute {
			continue
		}
		cmd, err := tools.ParseShellCommand(tools.RawToolCall{
			ID:            call.ID,
			ToolName:      call.Function.Name,
			ArgumentsJSON: call.Function.Arguments,
		})
		if err == nil && cmd.StepIndex > maxStep {
			maxStep = cmd.StepIndex
		}
	}
	return maxStep
}
