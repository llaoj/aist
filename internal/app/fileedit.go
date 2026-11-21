package app

import (
	"context"
	"errors"

	"github.com/llaoj/aist/internal/fileedit"
	"github.com/llaoj/aist/internal/i18n"
	"github.com/llaoj/aist/internal/interactive"
	"github.com/llaoj/aist/internal/ui"
)

// fileEditPrompter adapts terminal confirmation to file editing.
type fileEditPrompter struct {
	translator *i18n.I18n
	promptFn   func(prompt string, translator *i18n.I18n) (ui.ConfirmDecision, error)
}

// ConfirmFileEdit asks twice because writing a file is a modify operation.
func (p fileEditPrompter) ConfirmFileEdit(ctx context.Context, change *fileedit.Change) (interactive.Decision, error) {
	if err := ctx.Err(); err != nil {
		return interactive.DecisionSkip, err
	}
	label := change.LocationLabel()
	decision, err := p.ask(p.translator.T("file_edit.confirm", label))
	if err != nil || decision != interactive.DecisionApprove {
		return decision, err
	}
	if err := ctx.Err(); err != nil {
		return interactive.DecisionSkip, err
	}
	return p.ask(p.translator.T("file_edit.modify_warning", label))
}

func (p fileEditPrompter) ask(prompt string) (interactive.Decision, error) {
	promptFn := p.promptFn
	if promptFn == nil {
		promptFn = ui.PromptConfirm
	}
	result, err := promptFn(prompt, p.translator)
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
