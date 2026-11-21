package executor

import "strings"

// nonInteractiveEnv disables implicit pagers and editors so commands cannot
// silently enter a full-screen program when attached to a PTY.
var nonInteractiveEnv = []string{
	"PAGER=cat",
	"GIT_PAGER=cat",
	"SYSTEMD_PAGER=cat",
	"MANPAGER=cat",
	"EDITOR=false",
	"VISUAL=false",
	"GIT_EDITOR=false",
	"SUDO_EDITOR=false",
}

func withNonInteractiveEnv(base []string) []string {
	overridden := make(map[string]bool, len(nonInteractiveEnv))
	for _, kv := range nonInteractiveEnv {
		if idx := strings.IndexByte(kv, '='); idx > 0 {
			overridden[kv[:idx]] = true
		}
	}

	out := make([]string, 0, len(base)+len(nonInteractiveEnv))
	for _, kv := range base {
		if idx := strings.IndexByte(kv, '='); idx > 0 && overridden[kv[:idx]] {
			continue
		}
		out = append(out, kv)
	}
	return append(out, nonInteractiveEnv...)
}

func nonInteractiveEnvPrefix() string {
	return "export " + strings.Join(nonInteractiveEnv, " ") + "; "
}
