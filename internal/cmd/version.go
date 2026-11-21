package cmd

import (
	"fmt"

	"github.com/llaoj/aist/internal/buildinfo"
	"github.com/llaoj/aist/internal/i18n"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Run: func(cmd *cobra.Command, args []string) {
		translator := i18n.New("en")
		info := buildinfo.Default

		fmt.Println(translator.T("version.app_name"))
		fmt.Println(translator.T("version.version", info.Version))
		fmt.Println(translator.T("version.commit", info.Commit))
		fmt.Println(translator.T("version.build_date", info.BuildDate))
	},
}

func init() {
	markSkipStateInit(versionCmd)
	rootCmd.AddCommand(versionCmd)
}
