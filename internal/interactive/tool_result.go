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

type parsedToolResult struct {
	Fields     map[string]string
	Output     string
	HasOutput  bool
	Structured bool
}

var knownToolResultKeys = map[string]bool{
	"Status":    true,
	"Target":    true,
	"Exit Code": true,
	"Error":     true,
	"Detail":    true,
	"Command":   true,
	"Path":      true,
	"Sudo":      true,
}

func parseToolResult(content string) parsedToolResult {
	headerBlock, output, hasOutput := strings.Cut(content, "\n\n[Output]\n")
	if !strings.HasPrefix(headerBlock, "Status: ") {
		return parsedToolResult{Output: content}
	}

	fields := make(map[string]string)
	currentKey := ""
	for _, line := range strings.Split(headerBlock, "\n") {
		key, val, hasColon := strings.Cut(line, ": ")
		if hasColon && knownToolResultKeys[key] {
			currentKey = key
			fields[key] = val
			continue
		}
		if currentKey != "" {
			fields[currentKey] += "\n" + line
		}
	}

	return parsedToolResult{
		Fields:     fields,
		Output:     output,
		HasOutput:  hasOutput,
		Structured: true,
	}
}

// maxResultErrorBytes keeps error headers short; some executor errors embed
// command output that is already present in the [Output] section.
const maxResultErrorBytes = 500
