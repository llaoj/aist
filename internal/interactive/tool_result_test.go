package interactive

import (
	"testing"

	"github.com/llaoj/aist/internal/tools"
)

func TestRenderToolResultPutsHeadersBeforeOutput(t *testing.T) {
	got := renderToolResult(tools.ToolOutcomeSuccess, []toolResultField{
		resultField("Target", "web1"),
		resultField("Error", "  "),
		resultField("Exit Code", "0"),
	}, "line1\nline2")
	want := "Status: success\nTarget: web1\nExit Code: 0\n\n[Output]\nline1\nline2"
	if got != want {
		t.Fatalf("renderToolResult = %q, want %q", got, want)
	}
}

func TestRenderToolResultWithoutOutput(t *testing.T) {
	got := renderToolResult(tools.ToolOutcomeSkipped, []toolResultField{resultField("Detail", "skipped by user")}, "")
	if want := "Status: skipped\nDetail: skipped by user"; got != want {
		t.Fatalf("renderToolResult = %q, want %q", got, want)
	}
}
