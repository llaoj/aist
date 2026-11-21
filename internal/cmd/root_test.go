package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestSkipsStateInit(t *testing.T) {
	root := &cobra.Command{Use: "aist"}
	help := &cobra.Command{Use: "help"}
	complete := &cobra.Command{Use: cobra.ShellCompRequestCmd + " [command-line]"}
	normal := &cobra.Command{Use: "run"}
	root.AddCommand(help, complete, normal)

	if !skipsStateInit(help) {
		t.Fatal("help should skip state initialization")
	}
	if !skipsStateInit(complete) {
		t.Fatal("__complete should skip state initialization")
	}
	if skipsStateInit(normal) {
		t.Fatal("normal command should initialize state")
	}
}
