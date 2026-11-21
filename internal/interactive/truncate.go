package interactive

import (
	"fmt"
	"strings"
)

const (
	// MaxPipeDataBytes is the stdin limit used by pipe mode (~1.6 MB).
	MaxPipeDataBytes = 400000 * 4

	// MaxTurnChars caps one user or tool message so unbounded paste or command
	// output cannot blow past the context window in a single turn.
	MaxTurnChars = 8000

	// MaxPipeInputMessages is retained only for pipe input sizing. Session
	// compaction is token-based in internal/session.
	MaxPipeInputMessages = 30
)

// TruncateContent keeps the head and tail of a message within maxChars runes,
// inserting an omission marker in the middle. Truncating by rune avoids
// corrupting multi-byte characters.
func TruncateContent(content string, maxChars int) string {
	if maxChars <= 0 {
		return ""
	}

	runes := []rune(content)
	if len(runes) <= maxChars {
		return content
	}

	const markerBudget = 100
	if maxChars <= markerBudget {
		return string(runes[:maxChars])
	}

	headSize := int(float64(maxChars-markerBudget) * 0.6)
	tailSize := maxChars - markerBudget - headSize

	head := string(runes[:headSize])
	tail := string(runes[len(runes)-tailSize:])
	omitted := runes[headSize : len(runes)-tailSize]

	truncatedLines := strings.Count(string(omitted), "\n")
	return fmt.Sprintf("%s\n\n... [omitted %d lines] ...\n\n%s", head, truncatedLines, tail)
}
