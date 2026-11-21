package main

import (
	"fmt"
	"os"

	"github.com/llaoj/aist/internal/buildinfo"
	"github.com/llaoj/aist/internal/cmd"
)

// BuildVersion, BuildCommit and BuildDate are injected at build time via ldflags
var (
	BuildVersion = "unknown"
	BuildCommit  = "unknown"
	BuildDate    = "unknown"
)

func main() {
	buildinfo.Set(BuildVersion, BuildCommit, BuildDate)

	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
