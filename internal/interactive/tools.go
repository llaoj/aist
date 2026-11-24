package interactive

import (
	"context"
	"fmt"

	"github.com/llaoj/aist/internal/llm"
	"github.com/llaoj/aist/internal/tools"
)

// runToolCalls executes one batch of tool calls in model order and reports
// whether the user rejected remaining confirmation-gated calls. Only an
// interrupted or broken confirmation prompt returns a non-nil error.
func (s *Session) runToolCalls(ctx context.Context, calls []llm.ToolCall) (rejectRemaining bool, err error) {
	maxStep := maxStepIndex(calls)

	for i, call := range calls {
		switch call.Function.Name {
		case tools.ToolShellExecute:
			if rejectRemaining {
				s.appendToolReport(ctx, call.ID, tools.ToolOutcomeRejected, "", resultField("Detail", rejectedRemainingToolResult))
				continue
			}
			tgt, cmd, canPrompt := s.prepareShellCommand(ctx, call, maxStep)
			if !canPrompt {
				continue
			}
			decision, promptErr := s.promptShellCommand(ctx, cmd)
			if promptErr != nil {
				s.appendRemainingToolResults(ctx, calls[i:], interruptedToolResult, tools.ToolOutcomeSkipped)
				return false, promptErr
			}
			switch decision {
			case DecisionApprove:
				s.executeShellCommand(ctx, cmd, tgt)
			case DecisionRejectRemaining:
				rejectRemaining = true
			}
		case tools.ToolMemorySearch:
			s.handleMemorySearch(ctx, call)
		case tools.ToolMemorySave:
			s.handleMemorySave(ctx, call)
		case tools.ToolFileEdit:
			if rejectRemaining {
				s.appendToolReport(ctx, call.ID, tools.ToolOutcomeRejected, "", resultField("Detail", rejectedRemainingToolResult))
				continue
			}
			change, args, canPrompt := s.prepareFileEdit(ctx, call)
			if !canPrompt {
				continue
			}
			decision, promptErr := s.promptFileEdit(ctx, call.ID, args, change)
			if promptErr != nil {
				s.appendRemainingToolResults(ctx, calls[i:], interruptedToolResult, tools.ToolOutcomeSkipped)
				return false, promptErr
			}
			switch decision {
			case DecisionApprove:
				s.applyFileEdit(ctx, call.ID, args, change)
			case DecisionRejectRemaining:
				rejectRemaining = true
			}
		default:
			s.appendToolResult(ctx, call.ID, fmt.Sprintf("Unknown tool %q. No action was taken.", call.Function.Name), tools.ToolOutcomeSkipped)
		}
	}

	return rejectRemaining, nil
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
