package interactive

import (
	"context"
	"fmt"

	"github.com/llaoj/aist/internal/llm"
	"github.com/llaoj/aist/internal/tools"
)

// dispatchToolCalls executes one batch of tool calls in model order and reports
// whether the user rejected confirmation-gated calls to return to user input
// immediately. Tools that do not require user confirmation still execute.
func (s *Session) dispatchToolCalls(ctx context.Context, calls []llm.ToolCall) (rejectRemaining bool, err error) {
	maxStep := maxStepIndex(calls)
	rejectConfirmed := false

	for i, call := range calls {
		switch call.Function.Name {
		case tools.ToolShellExecute:
			if rejectConfirmed {
				s.appendToolReport(ctx, call.ID, tools.ToolOutcomeRejected, "", resultField("Detail", rejectedRemainingToolResult))
				continue
			}
			cmd, parseErr := tools.ParseShellCommand(toRawToolCall(call))
			if parseErr != nil {
				s.appendToolReport(ctx, call.ID, tools.ToolOutcomeFailed, "",
					resultField("Error", "invalid shell_execute arguments: "+parseErr.Error()),
					resultField("Detail", "No command was executed."),
				)
				continue
			}
			rejected, runErr := s.runShellCommand(ctx, cmd, maxStep)
			if runErr != nil {
				s.appendRemainingToolResults(ctx, calls[i:], interruptedToolResult, tools.ToolOutcomeSkipped)
				return false, runErr
			}
			if rejected {
				rejectConfirmed = true
			}
		case tools.ToolMemorySearch:
			s.handleMemorySearch(ctx, call)
		case tools.ToolMemorySave:
			s.handleMemorySave(ctx, call)
		case tools.ToolFileEdit:
			if rejectConfirmed {
				s.appendToolReport(ctx, call.ID, tools.ToolOutcomeRejected, "", resultField("Detail", rejectedRemainingToolResult))
				continue
			}
			rejected, editErr := s.handleFileEdit(ctx, call)
			if editErr != nil {
				s.appendRemainingToolResults(ctx, calls[i:], interruptedToolResult, tools.ToolOutcomeSkipped)
				return false, editErr
			}
			if rejected {
				rejectConfirmed = true
			}
		default:
			s.appendToolResult(ctx, call.ID, fmt.Sprintf("Unknown tool %q. No action was taken.", call.Function.Name), tools.ToolOutcomeSkipped)
		}
	}

	return rejectConfirmed, nil
}

const (
	interruptedToolResult       = "Tool call was interrupted before execution. No action was taken."
	rejectedRemainingToolResult = "Skipped because the user rejected the remaining tool calls."
	followUpLimitToolResult     = "Skipped because the per-turn tool call limit was reached. No action was taken."
)

func (s *Session) appendRemainingToolResults(ctx context.Context, calls []llm.ToolCall, detail string, outcome tools.ToolOutcome) {
	persistCtx := context.WithoutCancel(ctx)
	for _, call := range calls {
		s.appendToolReport(persistCtx, call.ID, outcome, "", resultField("Detail", detail))
	}
}

func toRawToolCall(call llm.ToolCall) tools.RawToolCall {
	return tools.RawToolCall{
		ID:            call.ID,
		ToolName:      call.Function.Name,
		ArgumentsJSON: call.Function.Arguments,
	}
}

func maxStepIndex(calls []llm.ToolCall) int {
	max := 0
	for _, call := range calls {
		if call.Function.Name != tools.ToolShellExecute {
			continue
		}
		cmd, err := tools.ParseShellCommand(toRawToolCall(call))
		if err == nil && cmd.StepIndex > max {
			max = cmd.StepIndex
		}
	}
	return max
}
