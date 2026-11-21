package ui

import "strings"

func RenderThinkContent(content, thinkPrefix string) string {
	const thinkOpen = `<think>`
	const thinkClose = `</think>`

	if !strings.Contains(content, thinkOpen) {
		return content
	}

	start := strings.Index(content, thinkOpen)
	if start == -1 {
		return content
	}

	// Search for the closing tag only after the opening tag so a stray
	// "</think>" earlier in the text can never invert the slice bounds.
	relEnd := strings.Index(content[start+len(thinkOpen):], thinkClose)
	if relEnd == -1 {
		return content
	}
	end := start + len(thinkOpen) + relEnd

	thinkContent := content[start+len(thinkOpen) : end]
	restContent := content[end+len(thinkClose):]

	var sb strings.Builder
	sb.WriteString(thinkPrefix)
	sb.WriteString("\n")
	sb.WriteString(strings.TrimSpace(thinkContent))
	sb.WriteString("\n\n")
	sb.WriteString(strings.TrimPrefix(restContent, "\n"))

	return sb.String()
}
