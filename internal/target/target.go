// Package target validates execution targets supplied by the user or Memory.
//
// A target is only a host name, IP address or SSH alias. The package deliberately
// does not read known_hosts, ssh_config, hosts files or inventories: SSH itself
// resolves aliases at execution time.
package target

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// LocalName is the label used for commands executed on the current machine.
const LocalName = "local"

const maxLength = 255

// Target is a validated execution target. Its zero value is the local machine.
type Target struct {
	name string
}

// Local returns the target for the current machine.
func Local() Target { return Target{} }

// New validates a raw target. Empty values and "local"/"localhost" select the
// current machine. Other values are passed to SSH unchanged after validation.
func New(raw string) (Target, error) {
	name := strings.TrimSpace(raw)
	if name == "" || strings.EqualFold(name, LocalName) || strings.EqualFold(name, "localhost") {
		return Local(), nil
	}
	if name == "." || name == ".." {
		return Target{}, errors.New("target must not be '.' or '..'")
	}
	if len(name) > maxLength {
		return Target{}, fmt.Errorf("target is too long (max %d characters)", maxLength)
	}
	if strings.HasPrefix(name, "-") {
		return Target{}, errors.New("target must not start with '-'")
	}
	for _, r := range name {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return Target{}, errors.New("target must not contain whitespace or control characters")
		}
		if !isAllowedRune(r) {
			return Target{}, fmt.Errorf("target contains unsupported character %q", r)
		}
	}
	if strings.HasPrefix(name, "@") || strings.HasSuffix(name, "@") {
		return Target{}, errors.New("target must include a host after user@")
	}
	return Target{name: name}, nil
}

// IsLocal reports whether the target runs on the current machine.
func (t Target) IsLocal() bool { return t.name == "" }

// Host returns the host or SSH alias. It is empty for the local target.
func (t Target) Host() string { return t.name }

// String returns the target label used in output and confirmations.
func (t Target) String() string {
	if t.IsLocal() {
		return LocalName
	}
	return t.name
}

// RenderPath renders a file path qualified by target and privilege mode.
func (t Target) RenderPath(path string, useSudo bool) string {
	label := path
	if !t.IsLocal() {
		label = fmt.Sprintf("[%s] %s", t.String(), path)
	}
	if useSudo {
		label += " (sudo)"
	}
	return label
}

func isAllowedRune(r rune) bool {
	switch r {
	case '.', '-', '_', ':', '@', '%', '[', ']':
		return true
	}
	return r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
}
