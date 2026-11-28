package ui

import (
	"fmt"

	"charm.land/lipgloss/v2"
	"github.com/fatih/color"
)

func PrintWelcome(welcome, version, exitHint string) {
	titleLine := fmt.Sprintf("%s %s", welcome, version)
	sep := SeparatorLineWidth(max(lipgloss.Width(titleLine), lipgloss.Width(exitHint)))
	color.Cyan(sep + "\n")
	color.New(color.FgCyan).Printf("%s\n", titleLine)
	color.Cyan(exitHint + "\n")
	color.Cyan(sep + "\n")
	PrintRegionEnd()
}
