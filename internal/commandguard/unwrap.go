package commandguard

import (
	"path"
	"strings"
)

// unwrapCommand strips leading environment assignments and common command
// wrappers so callers can inspect the program that actually runs.
func unwrapCommand(words []word) ([]word, bool) {
	cur := words
	unwrapped := false

	for len(cur) > 0 {
		for len(cur) > 0 && isEnvAssignment(cur[0]) {
			cur = cur[1:]
			unwrapped = true
		}
		if len(cur) == 0 {
			return nil, false
		}
		if !cur[0].static {
			return nil, false
		}

		switch path.Base(cur[0].text) {
		case "sudo":
			cur = stripSudo(cur[1:])
			unwrapped = true
		case "env":
			cur = stripEnv(cur[1:])
			unwrapped = true
		case "timeout":
			cur = stripTimeout(cur[1:])
			unwrapped = true
		case "nice":
			cur = stripNice(cur[1:])
			unwrapped = true
		case "nohup":
			cur = cur[1:]
			unwrapped = true
		case "stdbuf":
			cur = stripStdbuf(cur[1:])
			unwrapped = true
		case "command":
			if hasQueryFlag(cur[1:]) {
				return nil, false
			}
			cur = stripOptions(cur[1:], nil)
			unwrapped = true
		case "builtin":
			cur = cur[1:]
			unwrapped = true
		case "exec":
			cur = stripExec(cur[1:])
			unwrapped = true
		case "xargs":
			next, ok := stripXargs(cur[1:])
			if !ok {
				return nil, false
			}
			cur = next
			unwrapped = true
		default:
			if !unwrapped {
				return nil, false
			}
			cur[0].text = path.Base(cur[0].text)
			return cur, true
		}
	}
	return nil, false
}

func commandCandidates(words []word) [][]word {
	candidates := [][]word{words}
	if inner, ok := unwrapCommand(words); ok {
		candidates = append(candidates, inner)
	}
	return candidates
}

func stripSudo(args []word) []word {
	cur := args
	for len(cur) > 0 {
		if !cur[0].static {
			return nil
		}
		tok := cur[0].text
		if tok == "--" {
			return cur[1:]
		}
		if strings.HasPrefix(tok, "-") {
			cur = cur[1:]
			if sudoFlagTakesSeparateValue(tok) && len(cur) > 0 {
				cur = cur[1:]
			}
			continue
		}
		if isEnvAssignment(cur[0]) {
			cur = cur[1:]
			continue
		}
		break
	}
	return cur
}

func stripEnv(args []word) []word {
	cur := args
	for len(cur) > 0 {
		if !cur[0].static {
			return nil
		}
		tok := cur[0].text
		if tok == "--" {
			return cur[1:]
		}
		if strings.HasPrefix(tok, "-") {
			cur = cur[1:]
			if envFlagTakesSeparateValue(tok) && len(cur) > 0 {
				cur = cur[1:]
			}
			continue
		}
		if isEnvAssignment(cur[0]) {
			cur = cur[1:]
			continue
		}
		break
	}
	return cur
}

func stripTimeout(args []word) []word {
	cur := args
	for len(cur) > 0 {
		if !cur[0].static {
			return nil
		}
		tok := cur[0].text
		if tok == "--" {
			cur = cur[1:]
			break
		}
		if !strings.HasPrefix(tok, "-") || tok == "-" {
			break
		}
		cur = cur[1:]
		if timeoutFlagTakesValue(tok) && len(cur) > 0 {
			cur = cur[1:]
		}
	}
	if len(cur) == 0 {
		return nil
	}
	return cur[1:]
}

func stripNice(args []word) []word {
	cur := args
	for len(cur) > 0 {
		if !cur[0].static {
			return nil
		}
		tok := cur[0].text
		if tok == "--" {
			return cur[1:]
		}
		if !strings.HasPrefix(tok, "-") || tok == "-" {
			break
		}
		cur = cur[1:]
		if tok == "-n" || tok == "--adjustment" {
			if len(cur) > 0 {
				cur = cur[1:]
			}
		}
	}
	return cur
}

func stripStdbuf(args []word) []word {
	return stripOptions(args, func(tok string) bool {
		switch tok {
		case "-i", "-o", "-e":
			return true
		default:
			return false
		}
	})
}

func stripExec(args []word) []word {
	cur := args
	for len(cur) > 0 {
		if !cur[0].static {
			return nil
		}
		tok := cur[0].text
		if tok == "--" {
			return cur[1:]
		}
		if !strings.HasPrefix(tok, "-") || tok == "-" {
			break
		}
		cur = cur[1:]
		if tok == "-a" && len(cur) > 0 {
			cur = cur[1:]
		}
	}
	return cur
}

func stripXargs(args []word) ([]word, bool) {
	for i, item := range args {
		if !item.static {
			return nil, false
		}
		if item.text == "--" {
			return args[i+1:], true
		}
		if strings.HasPrefix(item.text, "-") && item.text != "-" {
			return nil, false
		}
		return args[i:], true
	}
	return nil, true
}

func stripOptions(args []word, takesValue func(string) bool) []word {
	cur := args
	for len(cur) > 0 {
		if !cur[0].static {
			return nil
		}
		tok := cur[0].text
		if tok == "--" {
			return cur[1:]
		}
		if !strings.HasPrefix(tok, "-") || tok == "-" {
			break
		}
		cur = cur[1:]
		if takesValue != nil && takesValue(tok) && len(cur) > 0 {
			cur = cur[1:]
		}
	}
	return cur
}

func hasQueryFlag(args []word) bool {
	for _, item := range args {
		if !item.static {
			continue
		}
		if item.text == "--" {
			return false
		}
		if !strings.HasPrefix(item.text, "-") || item.text == "-" {
			continue
		}
		if strings.ContainsAny(item.text[1:], "vV") {
			return true
		}
	}
	return false
}

func isEnvAssignment(item word) bool {
	if !item.static {
		return false
	}
	eq := strings.IndexByte(item.text, '=')
	if eq <= 0 || strings.HasPrefix(item.text, "-") {
		return false
	}
	return !strings.ContainsRune(item.text[:eq], '/')
}

func sudoFlagTakesSeparateValue(flag string) bool {
	switch flag {
	case "-u", "-g", "-C", "-D", "-h", "-p", "-R", "-T", "-U", "-r", "-t",
		"--user", "--group", "--close-from", "--chdir", "--host", "--prompt",
		"--chroot", "--command-timeout", "--other-user", "--role", "--type":
		return true
	default:
		return false
	}
}

func envFlagTakesSeparateValue(flag string) bool {
	switch flag {
	case "-u", "--unset", "-C", "--chdir", "-S", "--split-string":
		return true
	default:
		return false
	}
}

func timeoutFlagTakesValue(flag string) bool {
	switch flag {
	case "-k", "--kill-after", "-s", "--signal":
		return true
	default:
		return false
	}
}
