package ui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/llaoj/aist/internal/i18n"
	"github.com/llaoj/aist/internal/interactive"
)

// ConfirmDecision represents the user's choice in a confirmation prompt.
type ConfirmDecision = interactive.Decision

const (
	ConfirmUnknown         = interactive.DecisionUnknown
	ConfirmApprove         = interactive.DecisionApprove
	ConfirmSkip            = interactive.DecisionSkip
	ConfirmRejectRemaining = interactive.DecisionRejectRemaining
)

type confirmOption struct {
	decision ConfirmDecision
	label    string
}

// PromptConfirm displays a confirmation prompt and returns the result.
func PromptConfirm(prompt string, translator *i18n.I18n) (ConfirmDecision, error) {
	return promptConfirm(prompt, translator.T("confirm.key_hint"), defaultConfirmOptions(translator))
}

// PromptCommandConfirm displays a shell-command confirmation.
func PromptCommandConfirm(prompt string, translator *i18n.I18n) (ConfirmDecision, error) {
	return promptConfirm(prompt, translator.T("confirm.key_hint"), commandConfirmOptions(translator))
}

func defaultConfirmOptions(translator *i18n.I18n) []confirmOption {
	return []confirmOption{
		{decision: ConfirmApprove, label: translator.T("confirm.yes")},
		{decision: ConfirmSkip, label: translator.T("confirm.no")},
		{decision: ConfirmRejectRemaining, label: translator.T("confirm.reject_remaining")},
	}
}

func commandConfirmOptions(translator *i18n.I18n) []confirmOption {
	return []confirmOption{
		{decision: ConfirmApprove, label: translator.T("executor.confirm.run")},
		{decision: ConfirmSkip, label: translator.T("executor.confirm.skip")},
		{decision: ConfirmRejectRemaining, label: translator.T("executor.confirm.reject_remaining")},
	}
}

func promptConfirm(prompt, keyHint string, options []confirmOption) (ConfirmDecision, error) {
	labels := make([]string, 0, len(options))
	decisions := make([]ConfirmDecision, 0, len(options))
	for _, option := range options {
		labels = append(labels, option.label)
		decisions = append(decisions, option.decision)
	}

	model := newSelectModel(prompt, keyHint, labels)
	p := tea.NewProgram(model)
	final, err := p.Run()
	fmt.Println()

	if err != nil {
		return ConfirmSkip, fmt.Errorf("confirmation error: %w", err)
	}

	m := final.(selectModel)
	if m.interrupted {
		return ConfirmSkip, ErrInterrupted
	}
	return confirmDecisionForSelection(m.selected, decisions), nil
}

func confirmDecisionForSelection(index int, decisions []ConfirmDecision) ConfirmDecision {
	if index < 0 || index >= len(decisions) {
		return ConfirmUnknown
	}
	return decisions[index]
}
