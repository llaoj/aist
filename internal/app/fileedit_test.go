package app

import (
	"context"
	"testing"

	"github.com/llaoj/aist/internal/fileedit"
	"github.com/llaoj/aist/internal/i18n"
	"github.com/llaoj/aist/internal/interactive"
	"github.com/llaoj/aist/internal/ui"
)

func TestFileEditPrompterAsksTwice(t *testing.T) {
	cases := []struct {
		name        string
		answers     []ui.ConfirmDecision
		want        interactive.Decision
		wantPrompts int
	}{
		{name: "approve both", answers: []ui.ConfirmDecision{ui.ConfirmApprove, ui.ConfirmApprove}, want: interactive.DecisionApprove, wantPrompts: 2},
		{name: "skip first", answers: []ui.ConfirmDecision{ui.ConfirmSkip}, want: interactive.DecisionSkip, wantPrompts: 1},
		{name: "skip second", answers: []ui.ConfirmDecision{ui.ConfirmApprove, ui.ConfirmSkip}, want: interactive.DecisionSkip, wantPrompts: 2},
		{name: "reject second", answers: []ui.ConfirmDecision{ui.ConfirmApprove, ui.ConfirmRejectRemaining}, want: interactive.DecisionRejectRemaining, wantPrompts: 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var prompts []string
			prompter := fileEditPrompter{
				translator: i18n.New("zh"),
				promptFn: func(prompt string, _ *i18n.I18n) (ui.ConfirmDecision, error) {
					answer := tc.answers[len(prompts)]
					prompts = append(prompts, prompt)
					return answer, nil
				},
			}
			change := &fileedit.Change{Path: "/etc/hosts"}
			decision, err := prompter.ConfirmFileEdit(context.Background(), change)
			if err != nil {
				t.Fatalf("ConfirmFileEdit: %v", err)
			}
			if decision != tc.want {
				t.Fatalf("decision = %v, want %v", decision, tc.want)
			}
			if len(prompts) != tc.wantPrompts {
				t.Fatalf("prompts = %q, want %d prompts", prompts, tc.wantPrompts)
			}
			label := change.LocationLabel()
			if want := i18n.New("zh").T("file_edit.confirm", label); prompts[0] != want {
				t.Fatalf("first prompt = %q, want %q", prompts[0], want)
			}
			if tc.wantPrompts == 2 {
				if want := i18n.New("zh").T("file_edit.modify_warning", label); prompts[1] != want {
					t.Fatalf("second prompt = %q, want %q", prompts[1], want)
				}
			}
		})
	}
}
