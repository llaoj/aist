package ui

import (
	"strings"

	"github.com/fatih/color"
)

// Diff styles. fatih/color disables colour automatically when stdout is not a
// terminal, so redirected output stays plain.
var (
	diffFileHeader = color.New(color.Bold, color.FgHiCyan)
	diffHunk       = color.New(color.FgCyan)
	diffAdded      = color.New(color.FgGreen)
	diffRemoved    = color.New(color.FgRed)
	diffMeta       = color.New(color.FgHiBlack)
)

// ColorizeDiff applies git-style colours to a unified diff: red for removed
// lines, green for added lines, cyan for hunk headers. Only presentation
// changes; the underlying text keeps its original line structure.
func ColorizeDiff(diff string) string {
	if diff == "" {
		return ""
	}

	trailing := strings.HasSuffix(diff, "\n")
	lines := strings.Split(strings.TrimSuffix(diff, "\n"), "\n")

	var b strings.Builder
	headerPhase := true
	for i, line := range lines {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(colorizeDiffLine(line, headerPhase))
		if !strings.HasPrefix(line, "--- ") {
			headerPhase = false
		}
	}
	if trailing {
		b.WriteString("\n")
	}
	return b.String()
}

// colorizeDiffLine colours one line. File headers (---/+++) are only detected
// before the first hunk, so a removed line whose content starts with "--" is
// not mistaken for a header.
func colorizeDiffLine(line string, headerPhase bool) string {
	if headerPhase && (strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ")) {
		return diffFileHeader.Sprint(line)
	}
	switch {
	case strings.HasPrefix(line, "@@ "):
		return diffHunk.Sprint(line)
	case strings.HasPrefix(line, "\\ "):
		return diffMeta.Sprint(line)
	case strings.HasPrefix(line, "+"):
		return diffAdded.Sprint(line)
	case strings.HasPrefix(line, "-"):
		return diffRemoved.Sprint(line)
	default:
		return line
	}
}
