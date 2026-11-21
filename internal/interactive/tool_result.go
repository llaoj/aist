package interactive

import (
	"context"
	"strings"

	"github.com/llaoj/aist/internal/tools"
)

// toolResultField is one single-line header of a tool result.
type toolResultField struct {
	Key   string
	Value string
}

func resultField(key, value string) toolResultField {
	return toolResultField{Key: key, Value: value}
}

// renderToolResult puts the short, important headers first and the bulky
// output last so that any prefix truncation (server audit) keeps the headers.
func renderToolResult(outcome tools.ToolOutcome, fields []toolResultField, output string) string {
	var b strings.Builder
	b.WriteString("Status: ")
	b.WriteString(string(outcome))
	for _, field := range fields {
		value := strings.TrimSpace(field.Value)
		if value == "" {
			continue
		}
		b.WriteString("\n")
		b.WriteString(field.Key)
		b.WriteString(": ")
		b.WriteString(value)
	}
	if output != "" {
		b.WriteString("\n\n[Output]\n")
		b.WriteString(output)
	}
	return b.String()
}

func (s *Session) appendToolReport(ctx context.Context, callID string, outcome tools.ToolOutcome, output string, fields ...toolResultField) {
	s.appendToolResult(ctx, callID, renderToolResult(outcome, fields, output), outcome)
}

// maxResultErrorBytes keeps error headers short; some executor errors embed
// command output that is already present in the [Output] section.
const maxResultErrorBytes = 500
