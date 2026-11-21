// Package commandguard blocks shell commands before execution. It combines the
// user-configured blacklist with built-in full-screen TUI rules and parses each
// command line into a shell AST.
package commandguard

import (
	"path"
	"strings"
)

// Kind classifies why a command was blocked.
type Kind int

const (
	KindNone Kind = iota
	KindBlacklist
	KindTUIEditor
	KindTUIMonitor
)

// Block describes a blocked command. It is zero when the command is allowed.
type Block struct {
	Kind    Kind
	Pattern string
	Program string
}

// Blocked reports whether the command was blocked.
func (b Block) Blocked() bool { return b.Kind != KindNone }

// Guard checks commands against a blacklist and the built-in TUI policy.
type Guard struct {
	patterns []string
}

// New creates a Guard from blacklist patterns.
func New(patterns []string) *Guard {
	return &Guard{patterns: append([]string(nil), patterns...)}
}

// Check parses command line once and returns the first matching block.
func (g *Guard) Check(commandLine string) Block {
	parsed := parseCommandLine(commandLine)

	for _, pattern := range g.patterns {
		for _, cmd := range parsed.commands {
			for _, candidate := range commandCandidates(cmd.words) {
				if matchPattern(pattern, candidate) {
					return Block{Kind: KindBlacklist, Pattern: pattern}
				}
			}
		}
	}

	for _, cmd := range parsed.commands {
		for _, candidate := range commandCandidates(cmd.words) {
			if program, ok := tuiEditor(candidate); ok {
				return Block{Kind: KindTUIEditor, Program: program}
			}
			if program, ok := tuiMonitor(candidate); ok {
				return Block{Kind: KindTUIMonitor, Program: program}
			}
		}
	}
	return Block{}
}

func matchPattern(pattern string, command []word) bool {
	patParts := strings.Fields(pattern)
	if len(patParts) == 0 {
		return false
	}
	patParts[0] = path.Base(patParts[0])

	hasTrailingWildcard := patParts[len(patParts)-1] == "*"
	if hasTrailingWildcard {
		patParts = patParts[:len(patParts)-1]
	}

	if hasTrailingWildcard {
		if len(command) <= len(patParts) {
			return false
		}
	} else if len(command) < len(patParts) {
		return false
	}

	for i, part := range patParts {
		if !command[i].static || command[i].text != part {
			return false
		}
	}
	return true
}

func commandProgram(words []word) string {
	if len(words) == 0 || !words[0].static {
		return ""
	}
	return words[0].text
}

func commandArgs(words []word) []word {
	if len(words) <= 1 {
		return nil
	}
	return words[1:]
}

func tuiEditor(words []word) (string, bool) {
	program := commandProgram(words)
	args := commandArgs(words)
	if program == "" || hasVersionOrHelpFlag(args) {
		return "", false
	}
	switch program {
	case "vi", "vim", "nvim", "view", "vimdiff", "rvim", "rview",
		"nano", "pico", "emacs", "visudo", "micro", "helix", "hx", "kak", "joe":
		return program, true
	case "crontab":
		if hasShortFlag(args, 'e') || hasDynamic(args) {
			return "crontab -e", true
		}
	}
	return "", false
}

func tuiMonitor(words []word) (string, bool) {
	program := commandProgram(words)
	args := commandArgs(words)
	if program == "" || hasVersionOrHelpFlag(args) {
		return "", false
	}
	switch program {
	case "less", "more", "most", "htop", "btop", "atop", "nmon", "glances",
		"iftop", "watch", "mc", "ranger", "nnn", "lf", "broot":
		return program, true
	case "top":
		if !hasShortFlag(args, 'b') && !hasShortFlagNumber(args, 'l') {
			return program, true
		}
	case "tmux", "screen", "byobu":
		if !isNonInteractiveMultiplexer(program, args) {
			return program, true
		}
	}
	return "", false
}

func isNonInteractiveMultiplexer(program string, args []word) bool {
	switch program {
	case "tmux":
		if hasShortFlag(args, 'd') {
			return true
		}
		for _, item := range args {
			if !item.static {
				continue
			}
			switch item.text {
			case "ls", "list-sessions", "list-windows", "list-panes", "has-session",
				"kill-session", "kill-server", "send-keys", "capture-pane",
				"display-message", "show-options", "set-option", "wait-for":
				return true
			}
		}
	case "screen", "byobu":
		if hasShortFlag(args, 'd') {
			return true
		}
		for _, item := range args {
			if !item.static {
				continue
			}
			switch item.text {
			case "-ls", "-list", "--list", "-wipe", "--wipe":
				return true
			}
		}
	}
	return false
}

func hasShortFlag(args []word, target rune) bool {
	for _, item := range args {
		if !item.static {
			continue
		}
		if item.text == "--" {
			return false
		}
		if strings.HasPrefix(item.text, "--") || !strings.HasPrefix(item.text, "-") || item.text == "-" {
			continue
		}
		for _, ch := range item.text[1:] {
			if ch == target {
				return true
			}
		}
	}
	return false
}

func hasShortFlagNumber(args []word, target rune) bool {
	for i, item := range args {
		if !item.static {
			continue
		}
		if item.text == "--" {
			return false
		}
		if strings.HasPrefix(item.text, "--") || !strings.HasPrefix(item.text, "-") || item.text == "-" {
			continue
		}
		flags := item.text[1:]
		for idx := 0; idx < len(flags); idx++ {
			if rune(flags[idx]) != target {
				continue
			}
			rest := flags[idx+1:]
			if rest != "" && isDigits(rest) {
				return true
			}
			if rest == "" && i+1 < len(args) && args[i+1].static && isDigits(args[i+1].text) {
				return true
			}
		}
	}
	return false
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func hasVersionOrHelpFlag(args []word) bool {
	for _, item := range args {
		if !item.static {
			continue
		}
		switch item.text {
		case "--version", "--help":
			return true
		}
	}
	return false
}

func hasDynamic(args []word) bool {
	for _, item := range args {
		if !item.static {
			return true
		}
	}
	return false
}
