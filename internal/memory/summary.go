package memory

import (
	"strings"
	"unicode"
)

// DefaultSummaryLimit is the rune budget for a derived one-line summary.
const DefaultSummaryLimit = 140

// EffectiveSummary returns the stored summary, or a local fallback derived
// from the body. It never calls a model or accesses the network.
func (m Entry) EffectiveSummary(limit int) string {
	if summary := strings.TrimSpace(m.Summary); summary != "" {
		return singleLine(summary)
	}
	return DeriveSummary(m, limit)
}

// DeriveSummary builds a one-line summary from the first meaningful body line.
// It strips common Markdown markers and truncates by rune, never by byte.
func DeriveSummary(m Entry, limit int) string {
	if limit <= 0 {
		limit = DefaultSummaryLimit
	}

	for _, line := range strings.Split(m.Content, "\n") {
		line = cleanSummaryLine(line)
		if line == "" {
			continue
		}
		return truncateRunes(line, limit)
	}

	if name := strings.TrimSpace(m.Name); name != "" {
		return truncateRunes(name, limit)
	}
	return ""
}

func cleanSummaryLine(line string) string {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return ""
	}

	line = stripListMarker(line)

	replacer := strings.NewReplacer(
		"**", "",
		"__", "",
		"`", "",
		"~~", "",
	)
	line = replacer.Replace(line)
	line = singleLine(line)
	line = strings.TrimSpace(line)
	return strings.TrimSuffix(line, ".")
}

func stripListMarker(line string) string {
	for _, marker := range []string{"- ", "* ", "+ ", "> "} {
		if strings.HasPrefix(line, marker) {
			return strings.TrimSpace(strings.TrimPrefix(line, marker))
		}
	}
	return line
}

func singleLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func truncateRunes(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	if limit <= 1 {
		return string(runes[:limit])
	}
	return strings.TrimRightFunc(string(runes[:limit-1]), unicode.IsSpace) + "…"
}
