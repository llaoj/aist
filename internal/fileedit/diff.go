package fileedit

import (
	"fmt"
	"strings"

	"github.com/llaoj/aist/internal/target"
)

const maxSurroundingLines = 3

func RenderSnippetDiff(tgt target.Target, path, oldContent, newContent string) string {
	return renderDiff(diffHeaderPath(tgt, path), oldContent, newContent, false)
}

func renderFileDiff(tgt target.Target, path string, original, updated []byte) string {
	return renderDiff(diffHeaderPath(tgt, path), string(original), string(updated), true)
}

func diffHeaderPath(tgt target.Target, path string) string {
	if !tgt.IsLocal() {
		return tgt.String() + ":" + path
	}
	return path
}

func renderDiff(path, oldText, newText string, showLineNumbers bool) string {
	var sb strings.Builder
	sb.WriteString("--- ")
	sb.WriteString(path)
	sb.WriteString("\n+++ ")
	sb.WriteString(path)
	sb.WriteString("\n")

	if oldText == "" {
		newLines := splitBlock(newText)
		fmt.Fprintf(&sb, "@@ -0,0 +1,%d @@\n", len(newLines))
		writePrefixedLines(&sb, "+", newLines)
		return sb.String()
	}

	oldLines := splitBlock(oldText)
	newLines := splitBlock(newText)

	prefixLen := commonPrefixLen(oldLines, newLines)
	suffixLen := commonSuffixLen(oldLines[prefixLen:], newLines[prefixLen:])

	oldTrailingNewline := strings.HasSuffix(oldText, "\n")
	newTrailingNewline := strings.HasSuffix(newText, "\n")
	if oldTrailingNewline != newTrailingNewline {
		suffixLen = 0
		if prefixLen == len(oldLines) && prefixLen == len(newLines) && prefixLen > 0 {
			prefixLen--
		}
	}

	removed := oldLines[prefixLen : len(oldLines)-suffixLen]
	added := newLines[prefixLen : len(newLines)-suffixLen]
	before := tailLines(oldLines[:prefixLen], maxSurroundingLines)
	after := headLines(oldLines[len(oldLines)-suffixLen:], maxSurroundingLines)

	if showLineNumbers {
		startLine := 1 + prefixLen - len(before)
		oldCount := len(before) + len(removed) + len(after)
		newCount := len(before) + len(added) + len(after)
		fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n", startLine, oldCount, startLine, newCount)
	}

	writePrefixedLines(&sb, " ", before)
	writePrefixedLines(&sb, "-", removed)
	if oldTrailingNewline != newTrailingNewline && len(removed) > 0 && !oldTrailingNewline {
		sb.WriteString("\\ No newline at end of file\n")
	}
	writePrefixedLines(&sb, "+", added)
	if oldTrailingNewline != newTrailingNewline && len(added) > 0 && !newTrailingNewline {
		sb.WriteString("\\ No newline at end of file\n")
	}
	writePrefixedLines(&sb, " ", after)
	return sb.String()
}

func splitBlock(block string) []string {
	block = strings.TrimSuffix(block, "\n")
	if block == "" {
		return nil
	}
	return strings.Split(block, "\n")
}

func commonPrefixLen(a, b []string) int {
	limit := min(len(a), len(b))
	for i := 0; i < limit; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return limit
}

func commonSuffixLen(a, b []string) int {
	limit := min(len(a), len(b))
	for i := 0; i < limit; i++ {
		if a[len(a)-1-i] != b[len(b)-1-i] {
			return i
		}
	}
	return limit
}

func tailLines(lines []string, count int) []string {
	if len(lines) <= count {
		return lines
	}
	return lines[len(lines)-count:]
}

func headLines(lines []string, count int) []string {
	if len(lines) <= count {
		return lines
	}
	return lines[:count]
}

func writePrefixedLines(sb *strings.Builder, prefix string, lines []string) {
	for _, line := range lines {
		sb.WriteString(prefix)
		sb.WriteString(line)
		sb.WriteString("\n")
	}
}
