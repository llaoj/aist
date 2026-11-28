package ui

import "strings"

const (
	separatorChar  = "-"
	separatorCount = 35
)

func SeparatorLine() string {
	return SeparatorLineWidth(separatorCount)
}

func SeparatorLineWidth(width int) string {
	if width < separatorCount {
		width = separatorCount
	}
	return strings.Repeat(separatorChar, width)
}
