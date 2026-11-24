package interactive

import (
	"context"

	"github.com/llaoj/aist/internal/fileedit"
	"github.com/llaoj/aist/internal/llm"
	"github.com/llaoj/aist/internal/target"
	"github.com/llaoj/aist/internal/tools"
	"github.com/llaoj/aist/internal/util"
)

func (s *Session) prepareFileEdit(ctx context.Context, call llm.ToolCall) (*fileedit.Change, tools.FileEditArgs, bool) {
	if s.fileEditor == nil {
		s.appendToolReport(ctx, call.ID, tools.ToolOutcomeFailed, "",
			resultField("Error", "file_edit is not configured"),
			resultField("Detail", "No changes were written."),
		)
		return nil, tools.FileEditArgs{}, false
	}
	args, err := tools.ParseFileEditArgs(call.Function.Arguments)
	if err != nil {
		s.appendToolReport(ctx, call.ID, tools.ToolOutcomeFailed, "",
			resultField("Error", "invalid file_edit arguments: "+err.Error()),
			resultField("Detail", "No changes were written."),
		)
		return nil, tools.FileEditArgs{}, false
	}

	change, err := s.fileEditor.Prepare(ctx, fileedit.EditRequest{
		Target:     args.Target,
		Path:       args.Path,
		OldContent: args.OldContent,
		NewContent: args.NewContent,
		Reason:     args.Reason,
		UseSudo:    args.UseSudo,
	})
	if err != nil {
		s.appendFileEditReport(ctx, call.ID, args, tools.ToolOutcomeFailed, err.Error(), "No changes were written.")
		return nil, tools.FileEditArgs{}, false
	}

	s.renderer.RenderFileChange(change)

	if s.fileEditConfirmer == nil {
		s.appendFileEditReport(ctx, call.ID, args, tools.ToolOutcomeSkipped, "", "File edit requires confirmation, but confirmation is unavailable. No changes were written.")
		return nil, tools.FileEditArgs{}, false
	}
	return change, args, true
}

func (s *Session) promptFileEdit(ctx context.Context, callID string, args tools.FileEditArgs, change *fileedit.Change) (Decision, error) {
	decision, err := s.fileEditConfirmer.ConfirmFileEdit(ctx, change)
	if err != nil {
		return DecisionUnknown, err
	}
	if decision != DecisionUnknown {
		s.autoToolCalls = 0
	}

	switch decision {
	case DecisionRejectRemaining:
		s.appendFileEditReport(ctx, callID, args, tools.ToolOutcomeRejected, "", "User rejected this file edit and all subsequent tool calls.")
	case DecisionSkip:
		s.appendFileEditReport(ctx, callID, args, tools.ToolOutcomeSkipped, "", "User skipped this file edit. No changes were written.")
	case DecisionApprove:
	default:
		s.appendFileEditReport(ctx, callID, args, tools.ToolOutcomeSkipped, "", "File edit skipped: no confirmation decision. No changes were written.")
	}
	return decision, nil
}

func (s *Session) applyFileEdit(ctx context.Context, callID string, args tools.FileEditArgs, change *fileedit.Change) {
	if err := s.fileEditor.Apply(ctx, change); err != nil {
		s.appendFileEditReport(ctx, callID, args, tools.ToolOutcomeFailed, err.Error(), "The previous file was kept.")
		return
	}
	s.renderer.RenderNotice(NoticeLevelSuccess, "✓ Updated file: "+change.LocationLabel())
	s.appendFileEditReport(ctx, callID, args, tools.ToolOutcomeSuccess, "", "File updated successfully after user confirmation.")
}

func (s *Session) appendFileEditReport(ctx context.Context, callID string, args tools.FileEditArgs, outcome tools.ToolOutcome, errText, detail string) {
	s.appendToolReport(ctx, callID, outcome, "",
		resultField("Target", util.FirstNonEmpty(args.Target, target.LocalName)),
		resultField("Path", args.Path),
		resultField("Sudo", sudoLabel(args.UseSudo)),
		resultField("Error", errText),
		resultField("Detail", detail),
	)
}

func sudoLabel(useSudo bool) string {
	if useSudo {
		return "true"
	}
	return ""
}
