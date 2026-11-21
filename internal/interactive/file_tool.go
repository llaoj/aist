package interactive

import (
	"context"

	"github.com/llaoj/aist/internal/fileedit"
	"github.com/llaoj/aist/internal/llm"
	"github.com/llaoj/aist/internal/target"
	"github.com/llaoj/aist/internal/tools"
	"github.com/llaoj/aist/internal/util"
)

func (s *Session) handleFileEdit(ctx context.Context, call llm.ToolCall) (rejectRemaining bool, err error) {
	if s.fileEditor == nil {
		s.appendToolReport(ctx, call.ID, tools.ToolOutcomeFailed, "",
			resultField("Error", "file_edit is not configured"),
			resultField("Detail", "No changes were written."),
		)
		return false, nil
	}
	args, err := tools.ParseFileEditArgs(call.Function.Arguments)
	if err != nil {
		s.appendToolReport(ctx, call.ID, tools.ToolOutcomeFailed, "",
			resultField("Error", "invalid file_edit arguments: "+err.Error()),
			resultField("Detail", "No changes were written."),
		)
		return false, nil
	}
	report := func(outcome tools.ToolOutcome, errText, detail string) {
		s.appendToolReport(ctx, call.ID, outcome, "",
			resultField("Target", util.FirstNonEmpty(args.Target, target.LocalName)),
			resultField("Path", args.Path),
			resultField("Sudo", sudoLabel(args.UseSudo)),
			resultField("Error", errText),
			resultField("Detail", detail),
		)
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
		report(tools.ToolOutcomeFailed, err.Error(), "No changes were written.")
		return false, nil
	}

	s.renderer.RenderFileChange(change)

	location := change.LocationLabel()
	if s.fileEditConfirmer == nil {
		report(tools.ToolOutcomeSkipped, "", "File edit requires confirmation, but confirmation is unavailable. No changes were written.")
		return false, nil
	}
	decision, err := s.fileEditConfirmer.ConfirmFileEdit(ctx, change)
	if err != nil {
		return false, err
	}
	switch decision {
	case DecisionRejectRemaining:
		report(tools.ToolOutcomeRejected, "", "User rejected this file edit and all subsequent tool calls.")
		return true, nil
	case DecisionApprove:
	default:
		report(tools.ToolOutcomeSkipped, "", "User skipped this file edit. No changes were written.")
		return false, nil
	}

	if err := s.fileEditor.Apply(ctx, change); err != nil {
		report(tools.ToolOutcomeFailed, err.Error(), "The previous file was kept.")
		return false, nil
	}
	s.renderer.RenderNotice(NoticeLevelSuccess, "✓ Updated file: "+location)
	report(tools.ToolOutcomeSuccess, "", "File updated successfully after user confirmation.")
	return false, nil
}

func sudoLabel(useSudo bool) string {
	if useSudo {
		return "true"
	}
	return ""
}
