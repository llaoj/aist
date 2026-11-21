package ui

import (
	"github.com/fatih/color"
)

func PrintWelcome(welcome, version, exitHint string) {
	color.Cyan(SeparatorLine() + "\n")
	color.New(color.FgCyan).Printf("%s %s\n", welcome, version)
	color.Cyan(exitHint + "\n")
	color.Cyan(SeparatorLine() + "\n")
	PrintRegionEnd()
}
