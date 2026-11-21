package ui

import "strings"

const (
	separatorChar  = "-"
	separatorCount = 35
)

func SeparatorLine() string {
	return strings.Repeat(separatorChar, separatorCount)
}
