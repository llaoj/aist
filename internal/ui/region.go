package ui

import (
	"fmt"
	"strings"
)

// RemoveBlankLines removes blank lines while preserving the remaining line
// breaks. It is used to keep assistant answers dense.
func RemoveBlankLines(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// PrintRegionEnd prints the single blank line that terminates a console region.
// Regions must not print leading blank lines themselves.
func PrintRegionEnd() {
	fmt.Println()
}
