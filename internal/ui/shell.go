package ui

import (
	"github.com/fatih/color"
	"github.com/llaoj/aist/internal/i18n"
	"github.com/llaoj/aist/internal/tools"
)

// CommandTypeLabel returns the localized label and color for a command type.
func CommandTypeLabel(cmdType tools.CommandType, translator *i18n.I18n) (string, *color.Color) {
	if cmdType == tools.CommandTypeQuery {
		return translator.T("executor.query_command"), color.New(color.FgGreen)
	}
	return translator.T("executor.modify_command"), color.New(color.FgRed)
}

// PrintShellCommand prints the command type, step and command line.
func PrintShellCommand(stepLabel string, cmd tools.ShellCommand, translator *i18n.I18n) {
	label, colorFn := CommandTypeLabel(cmd.Type, translator)
	target := cmd.Target
	if target == "" {
		target = "local"
	}
	colorFn.Printf("%s\n", label)
	colorFn.Printf("%s. %s\n", stepLabel, cmd.Reason)
	colorFn.Printf("[%s] $ %s\n", target, cmd.CommandLine)
	PrintRegionEnd()
}
