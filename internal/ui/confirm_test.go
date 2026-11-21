package ui

import (
	"testing"

	"github.com/llaoj/aist/internal/i18n"
)

func TestConfirmDecisionForSelection(t *testing.T) {
	decisions := []ConfirmDecision{ConfirmApprove, ConfirmSkip, ConfirmRejectRemaining}
	tests := []struct {
		name  string
		index int
		want  ConfirmDecision
	}{
		{name: "approve", index: 0, want: ConfirmApprove},
		{name: "skip", index: 1, want: ConfirmSkip},
		{name: "reject remaining", index: 2, want: ConfirmRejectRemaining},
		{name: "negative index", index: -1, want: ConfirmUnknown},
		{name: "out of range", index: 3, want: ConfirmUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := confirmDecisionForSelection(tt.index, decisions); got != tt.want {
				t.Fatalf("confirmDecisionForSelection(%d) = %v, want %v", tt.index, got, tt.want)
			}
		})
	}
}

func TestCommandConfirmOptions(t *testing.T) {
	options := commandConfirmOptions(i18n.New("en"))
	want := []ConfirmDecision{ConfirmApprove, ConfirmSkip, ConfirmRejectRemaining}
	assertOptionDecisions(t, options, want)
}

func TestDefaultConfirmOptions(t *testing.T) {
	options := defaultConfirmOptions(i18n.New("en"))
	want := []ConfirmDecision{ConfirmApprove, ConfirmSkip, ConfirmRejectRemaining}
	assertOptionDecisions(t, options, want)
}

func assertOptionDecisions(t *testing.T, options []confirmOption, want []ConfirmDecision) {
	t.Helper()
	if len(options) != len(want) {
		t.Fatalf("option count = %d, want %d", len(options), len(want))
	}
	for i, option := range options {
		if option.label == "" {
			t.Fatalf("option %d has empty label", i)
		}
		if option.decision != want[i] {
			t.Fatalf("option %d decision = %v, want %v", i, option.decision, want[i])
		}
	}
}
