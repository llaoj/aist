// Command server runs the AIST server on top of PocketBase.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/llaoj/aist/internal/buildinfo"
	"github.com/llaoj/aist/internal/server"
	"github.com/llaoj/aist/internal/server/adminui"
	_ "github.com/llaoj/aist/migrations"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/spf13/cobra"
)

// BuildVersion, BuildCommit and BuildDate are injected at build time via ldflags.
var (
	BuildVersion = "unknown"
	BuildCommit  = "unknown"
	BuildDate    = "unknown"
)

const dataDir = "pb_data"

func main() {
	buildinfo.Set(BuildVersion, BuildCommit, BuildDate)

	// Standalone helpers run without starting PocketBase so they need neither a
	// data directory nor a bootstrapped DB.
	if args := os.Args[1:]; isStandaloneCommand(args) {
		if err := runStandalone(args); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
		return
	}

	cfg := server.LoadConfig()

	app := pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDataDir: dataDir,
	})

	srv := server.NewAPI(server.APIOptions{})
	srv.BindHooks(app)

	app.OnServe().Bind(&hook.Handler[*core.ServeEvent]{
		Func: func(e *core.ServeEvent) error {
			e.Server.Addr = cfg.Addr
			adminui.Bind(e)
			srv.RegisterRoutes(e)
			return e.Next()
		},
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}

func isStandaloneCommand(args []string) bool {
	if len(args) == 0 {
		return false
	}
	return args[0] == "version"
}

func runStandalone(args []string) error {
	root := &cobra.Command{
		Use:           "server",
		Short:         "AIST server",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newVersionCommand())
	root.SetArgs(args)
	return root.Execute()
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Version:    %s\n", buildinfo.Default.Version)
			fmt.Printf("Commit:     %s\n", buildinfo.Default.Commit)
			fmt.Printf("Build Date: %s\n", buildinfo.Default.BuildDate)
		},
	}
}
