package memory

import (
	"strings"
	"testing"
)

func TestDeriveSummaryUsesFirstBodyLine(t *testing.T) {
	mem := Entry{
		Name:    "prod",
		Content: "\n- **Kubernetes** cluster `prod` with 3 nodes.\n- second line",
	}
	got := DeriveSummary(mem, 0)
	want := "Kubernetes cluster prod with 3 nodes"
	if got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
}

func TestDeriveSummaryTruncatesByRune(t *testing.T) {
	mem := Entry{Name: "prod", Content: "- 这是一个很长的中文摘要需要被截断"}
	got := DeriveSummary(mem, 8)
	if runes := []rune(got); len(runes) > 8 {
		t.Fatalf("summary = %q has %d runes", got, len(runes))
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("summary = %q, want ellipsis", got)
	}
}

func TestDeriveSummaryFallsBackToName(t *testing.T) {
	if got := DeriveSummary(Entry{Name: "prod"}, 0); got != "prod" {
		t.Fatalf("summary = %q, want prod", got)
	}
}

func TestEffectiveSummaryPrefersStoredValue(t *testing.T) {
	mem := Entry{Name: "prod", Summary: "  stored summary  ", Content: "- body line"}
	if got := mem.EffectiveSummary(0); got != "stored summary" {
		t.Fatalf("summary = %q", got)
	}
}
