package fileedit

import (
	"fmt"
	"strings"
)

func renderDiff(path string, original, updated []byte, oldContent, newContent string, offset int) string {
	oldLines := splitBlock(oldContent)
	newLines := splitBlock(newContent)

	var sb strings.Builder
	sb.WriteString("--- ")
	sb.WriteString(path)
	sb.WriteString("\n+++ ")
	sb.WriteString(path)
	sb.WriteString("\n")

	if oldContent == "" {
		fmt.Fprintf(&sb, "@@ -0,0 +1,%d @@\n", len(newLines))
		for _, line := range newLines {
			sb.WriteString("+")
			sb.WriteString(line)
			sb.WriteString("\n")
		}
		return sb.String()
	}

	before := contextBefore(original, offset, 3)
	after := contextAfter(original, offset+len(oldContent), 3)
	startLine := 1 + strings.Count(string(original[:offset]), "\n") - len(before)
	oldCount := len(before) + len(oldLines) + len(after)
	newCount := len(before) + len(newLines) + len(after)
	fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n", startLine, oldCount, startLine, newCount)

	for _, line := range before {
		sb.WriteString(" ")
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	for _, line := range oldLines {
		sb.WriteString("-")
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	for _, line := range newLines {
		sb.WriteString("+")
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	for _, line := range after {
		sb.WriteString(" ")
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	return sb.String()
}

func splitBlock(block string) []string {
	block = strings.TrimSuffix(block, "\n")
	if block == "" {
		return nil
	}
	return strings.Split(block, "\n")
}

func contextBefore(original []byte, offset, count int) []string {
	if offset <= 0 || count <= 0 {
		return nil
	}
	prefix := strings.TrimSuffix(string(original[:offset]), "\n")
	if prefix == "" {
		return nil
	}
	lines := strings.Split(prefix, "\n")
	if len(lines) > count {
		lines = lines[len(lines)-count:]
	}
	return lines
}

func contextAfter(original []byte, offset, count int) []string {
	if offset >= len(original) || count <= 0 {
		return nil
	}
	rest := string(original[offset:])
	rest = strings.TrimPrefix(rest, "\n")
	if rest == "" {
		return nil
	}
	lines := strings.Split(rest, "\n")
	if len(lines) > count {
		lines = lines[:count]
	}
	return lines
}
