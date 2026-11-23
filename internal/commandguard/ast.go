package commandguard

import (
	"path"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

const (
	maxScriptDepth  = 8
	maxCommandBytes = 64 * 1024
)

type word struct {
	text   string
	static bool
}

type command struct {
	words []word
}

type parsed struct {
	commands []command
}

func parseCommandLine(line string) parsed {
	var out parsed
	if len(line) > maxCommandBytes {
		out.addFallback(line, 0)
		return out
	}
	out.addScript(line, 0)
	return out
}

func (p *parsed) addScript(script string, depth int) {
	if depth > maxScriptDepth {
		return
	}
	file, ok := parseShell(script)
	if !ok {
		p.addFallback(script, depth)
		return
	}
	syntax.Walk(file, func(node syntax.Node) bool {
		if call, isCall := node.(*syntax.CallExpr); isCall {
			p.addCall(call, depth)
		}
		return true
	})
}

func parseShell(script string) (*syntax.File, bool) {
	for _, lang := range []syntax.LangVariant{syntax.LangBash, syntax.LangZsh, syntax.LangPOSIX} {
		file, err := syntax.NewParser(syntax.Variant(lang)).Parse(strings.NewReader(script), "command")
		if err == nil {
			return file, true
		}
	}
	return nil, false
}

func (p *parsed) addCall(call *syntax.CallExpr, depth int) {
	if len(call.Args) == 0 {
		return
	}
	words := make([]word, 0, len(call.Args))
	for _, arg := range call.Args {
		text, ok := staticWord(arg)
		words = append(words, word{text: text, static: ok})
	}
	if words[0].static {
		words[0].text = path.Base(words[0].text)
	}
	cmd := command{words: words}
	p.commands = append(p.commands, cmd)
	p.addNested(cmd.words, depth)
	if inner, ok := unwrapCommand(cmd.words); ok {
		p.addNested(inner, depth)
	}
}

func (p *parsed) addNested(words []word, depth int) {
	if depth >= maxScriptDepth || len(words) == 0 || !words[0].static {
		return
	}
	switch words[0].text {
	case "sh", "bash", "zsh", "dash", "ksh", "mksh":
		if script, ok := shellScriptArg(words[1:]); ok {
			p.addScript(script, depth+1)
		}
	case "eval":
		if script, ok := staticJoin(words[1:]); ok {
			p.addScript(script, depth+1)
		}
	}
}

func shellScriptArg(args []word) (string, bool) {
	for i := 0; i < len(args); i++ {
		if !args[i].static {
			return "", false
		}
		arg := args[i].text
		if arg == "--" {
			return "", false
		}
		if strings.HasPrefix(arg, "--") {
			continue
		}
		if (!strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "+")) || arg == "-" {
			return "", false
		}
		switch arg {
		case "-o", "+o", "-O", "--rcfile", "--init-file":
			i++
			continue
		}
		if strings.Contains(arg[1:], "c") {
			if i+1 < len(args) && args[i+1].static {
				return args[i+1].text, true
			}
			return "", false
		}
	}
	return "", false
}

func staticJoin(words []word) (string, bool) {
	parts := make([]string, len(words))
	for i, item := range words {
		if !item.static {
			return "", false
		}
		parts[i] = item.text
	}
	return strings.Join(parts, " "), true
}

func staticWord(w *syntax.Word) (string, bool) {
	if w == nil {
		return "", false
	}
	var b strings.Builder
	for _, part := range w.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			b.WriteString(p.Value)
		case *syntax.SglQuoted:
			b.WriteString(p.Value)
		case *syntax.DblQuoted:
			for _, sub := range p.Parts {
				lit, ok := sub.(*syntax.Lit)
				if !ok {
					return "", false
				}
				b.WriteString(lit.Value)
			}
		default:
			return "", false
		}
	}
	return b.String(), true
}

func (p *parsed) addFallback(script string, depth int) {
	for _, segment := range splitFallback(script) {
		fields := strings.Fields(segment)
		if len(fields) == 0 {
			continue
		}
		words := make([]word, 0, len(fields))
		for _, field := range fields {
			words = append(words, word{text: strings.Trim(field, `"'`), static: true})
		}
		words[0].text = path.Base(words[0].text)
		cmd := command{words: words}
		p.commands = append(p.commands, cmd)
		p.addNested(cmd.words, depth)
	}
}

func splitFallback(script string) []string {
	return strings.FieldsFunc(script, func(r rune) bool {
		switch r {
		case ';', '|', '&', '\n':
			return true
		}
		return false
	})
}
