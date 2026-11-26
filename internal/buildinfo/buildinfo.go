package buildinfo

import (
	"fmt"
	"runtime"
	"strings"
)

// Info holds build metadata injected at link time.
type Info struct {
	Version   string
	Commit    string
	BuildDate string
}

// Default is the process-wide build information.
var Default = Info{
	Version:   "unknown",
	Commit:    "unknown",
	BuildDate: "unknown",
}

// Set updates the process-wide build information.
func Set(version, commit, buildDate string) {
	Default.Version = version
	Default.Commit = commit
	Default.BuildDate = buildDate
}

// UserAgent returns the HTTP User-Agent string identifying this aist client.
func UserAgent() string {
	version := strings.TrimSpace(Default.Version)
	if version == "" {
		version = "unknown"
	}
	return fmt.Sprintf("aist/%s (%s; %s)", version, runtime.GOOS, runtime.GOARCH)
}
