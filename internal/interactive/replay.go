package interactive

import (
	"github.com/llaoj/aist/internal/session"
	"github.com/llaoj/aist/internal/tools"
)

// ReplayHistory renders the durable display view before an interactive loop.
//
// It never calls the model or re-runs tools. Tool text is rebuilt from the
// original tool call arguments and the persisted language-neutral status, so
// switching the configured language also updates old session history.
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
	calls := make(map[string]session.ToolCall)

	s.renderer.RenderHistoryHeader(s.translator.T("sessions.history_header"))
	for _, message := range messages {
		switch message.Role {
		case session.RoleSystem:
			// System prompts are implementation detail, not conversation.
		case session.RoleUser:
			s.renderer.RenderUserMessage(message.Content)
		case session.RoleAssistant:
			s.renderer.RenderAssistantMessage("", message.Content)
			for _, call := range message.ToolCalls {
				calls[call.ID] = call
			}
		case session.RoleTool:
			call, ok := calls[message.ToolCallID]
			if !ok {
				continue
			}
			display, err := tools.DecodeInvocation(call.Function.Name, call.Function.Arguments)
			if err != nil {
				continue
			}
			s.renderer.RenderToolSummary(display.FormatResult(message.ToolOutcome, s.translator))
		}

		if latest != nil && message.Seq == latest.CoversToSeq {
			s.renderer.RenderNotice(NoticeLevelInfo, s.translator.T("sessions.compacted"))
		}
	}
	s.renderer.Sync()
}
