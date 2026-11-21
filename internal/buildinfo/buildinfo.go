package buildinfo

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
