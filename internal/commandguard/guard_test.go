package commandguard

import (
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

func TestCheckBlacklist(t *testing.T) {
	guard := New([]string{"rm *", "dd *", "kubectl delete *", "shutdown"})

	cases := []struct {
		name    string
		command string
		blocked bool
		pattern string
	}{
		{"rm with args", "rm -rf /", true, "rm *"},
		{"rm without args", "rm", false, ""},
		{"rm by absolute path", "/usr/bin/rm -rf /", true, "rm *"},
		{"sudo rm with args", "sudo rm -rf /", true, "rm *"},
		{"sudo with user flag and path", "/usr/bin/sudo -u root VAR=1 /bin/rm -rf /", true, "rm *"},
		{"sudo shutdown", "sudo -n shutdown -h now", true, "shutdown"},
		{"timeout wrapped rm", "timeout 10 rm -rf /", true, "rm *"},
		{"xargs wrapped rm", "xargs rm -rf /", true, "rm *"},
		{"nested command substitution rm", "echo $(rm -rf /)", true, "rm *"},
		{"nested backtick rm", "echo `rm -rf /`", true, "rm *"},
		{"nested subshell rm", "(rm -rf /)", true, "rm *"},
		{"nested sh -c rm", "sh -c 'rm -rf /'", true, "rm *"},
		{"nested sudo sh -c rm", "sudo sh -c 'rm -rf /'", true, "rm *"},
		{"nested eval rm", "eval 'rm -rf /'", true, "rm *"},
		{"dd with args", "dd if=/dev/zero of=/dev/sda", true, "dd *"},
		{"kubectl delete with resource", "kubectl delete pod nginx", true, "kubectl delete *"},
		{"kubectl delete without args", "kubectl delete", false, ""},
		{"shutdown bare", "shutdown", true, "shutdown"},
		{"shutdown with args", "shutdown -h now", true, "shutdown"},
		{"unrelated command", "ls -la", false, ""},
		{"heredoc body is data", "cat <<EOF\nrm -rf /\nEOF", false, ""},
		{"empty command", "", false, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			block := guard.Check(tc.command)
			if block.Blocked() != tc.blocked {
				t.Fatalf("Check(%q) = %+v, want blocked=%v", tc.command, block, tc.blocked)
			}
			if tc.blocked && block.Pattern != tc.pattern {
				t.Fatalf("Check(%q) pattern = %q, want %q", tc.command, block.Pattern, tc.pattern)
			}
		})
	}
}

func TestMatchPatternShape(t *testing.T) {
	cases := []struct {
		name     string
		pattern  string
		command  []word
		expected bool
	}{
		{"trailing wildcard needs an extra word", "rm *", staticWords("rm"), false},
		{"trailing wildcard matches one word", "rm *", staticWords("rm", "-rf"), true},
		{"no wildcard allows trailing words", "rm -rf", staticWords("rm", "-rf", "/tmp"), true},
		{"no wildcard requires prefix match", "rm -rf", staticWords("rm", "-r", "/tmp"), false},
		{"empty pattern never matches", "", staticWords("rm"), false},
		{"dynamic word does not match exact pattern", "rm -rf", []word{{text: "rm", static: true}, {text: "", static: false}}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := matchPattern(tc.pattern, tc.command); got != tc.expected {
				t.Fatalf("matchPattern(%q, %v) = %v, want %v", tc.pattern, tc.command, got, tc.expected)
			}
		})
	}
}

func TestNewCopiesPatterns(t *testing.T) {
	patterns := []string{"rm *"}
	guard := New(patterns)
	patterns[0] = "ls"
	if block := guard.Check("rm -rf /"); block.Kind != KindBlacklist {
		t.Fatalf("New must copy patterns: %+v", block)
	}
	if block := guard.Check("ls"); block.Blocked() {
		t.Fatalf("mutating source slice affected guard: %+v", block)
	}
}

func TestCheckTUIPrograms(t *testing.T) {
	guard := New(nil)
	cases := []struct {
		name    string
		command string
		kind    Kind
		program string
	}{
		{"vim direct", "vim /etc/hosts", KindTUIEditor, "vim"},
		{"sudo nano", "sudo -u root nano /etc/nginx/nginx.conf", KindTUIEditor, "nano"},
		{"nvim by path", "/usr/local/bin/nvim app.yaml", KindTUIEditor, "nvim"},
		{"visudo", "sudo visudo", KindTUIEditor, "visudo"},
		{"crontab edit", "sudo crontab -u root -e", KindTUIEditor, "crontab -e"},
		{"crontab dynamic options", "crontab $FLAGS", KindTUIEditor, "crontab -e"},
		{"crontab list allowed", "crontab -l", KindNone, ""},
		{"piped less", "journalctl -u nginx | less", KindTUIMonitor, "less"},
		{"htop", "sudo htop", KindTUIMonitor, "htop"},
		{"watch", "watch -n 1 df -h", KindTUIMonitor, "watch"},
		{"ranger", "ranger /etc", KindTUIMonitor, "ranger"},
		{"bare tmux", "tmux", KindTUIMonitor, "tmux"},
		{"tmux attach", "tmux attach -t work", KindTUIMonitor, "tmux"},
		{"tmux dynamic subcommand", "tmux $SUB", KindTUIMonitor, "tmux"},
		{"tmux list allowed", "tmux ls", KindNone, ""},
		{"tmux detached allowed", "tmux new -d -s work", KindNone, ""},
		{"screen list allowed", "screen -ls", KindNone, ""},
		{"bare top blocked", "top", KindTUIMonitor, "top"},
		{"top dynamic flags blocked", "top $FLAGS", KindTUIMonitor, "top"},
		{"linux batch top allowed", "top -b -n 1 | head -20", KindNone, ""},
		{"linux combined batch top allowed", "top -bn1", KindNone, ""},
		{"macos sample top allowed", "top -l 1 -n 10", KindNone, ""},
		{"macos combined sample top allowed", "top -l1", KindNone, ""},
		{"top without sample count blocked", "top -l", KindTUIMonitor, "top"},
		{"timeout wrapped vim", "timeout 10 vim /etc/hosts", KindTUIEditor, "vim"},
		{"nice wrapped vim", "nice -n 5 vim /etc/hosts", KindTUIEditor, "vim"},
		{"nohup wrapped vim", "nohup vim /etc/hosts", KindTUIEditor, "vim"},
		{"stdbuf wrapped vim", "stdbuf -oL vim /etc/hosts", KindTUIEditor, "vim"},
		{"command wrapped vim", "command -p vim /etc/hosts", KindTUIEditor, "vim"},
		{"builtin wrapped vim", "builtin vim /etc/hosts", KindTUIEditor, "vim"},
		{"exec wrapped vim", "exec vim /etc/hosts", KindTUIEditor, "vim"},
		{"xargs wrapped vim", "xargs vim /etc/hosts", KindTUIEditor, "vim"},
		{"flagged xargs not unwrapped", "xargs -n1 vim /etc/hosts", KindNone, ""},
		{"command substitution vim", "echo $(vim /etc/hosts)", KindTUIEditor, "vim"},
		{"backtick vim", "echo `vim /etc/hosts`", KindTUIEditor, "vim"},
		{"subshell vim", "(vim /etc/hosts)", KindTUIEditor, "vim"},
		{"control flow vim", "for f in a b; do vim \"$f\"; done", KindTUIEditor, "vim"},
		{"bash -c vim", "bash -c 'vim /etc/hosts'", KindTUIEditor, "vim"},
		{"bash with option before -c", "bash -o pipefail -c 'vim /etc/hosts'", KindTUIEditor, "vim"},
		{"sudo bash -c vim", "sudo bash -c 'vim /etc/hosts'", KindTUIEditor, "vim"},
		{"eval vim", "eval 'vim /etc/hosts'", KindTUIEditor, "vim"},
		{"bash -c echo allowed", "bash -c 'echo ok'", KindNone, ""},
		{"dynamic command name not classified", "$CMD /etc/hosts", KindNone, ""},
		{"heredoc body not a command", "cat <<EOF\nvim /etc/hosts\nEOF", KindNone, ""},
		{"quoted vim argument allowed", "echo 'vim'", KindNone, ""},
		{"grep vim allowed", "grep vim file", KindNone, ""},
		{"vim version check allowed", "vim --version", KindNone, ""},
		{"normal sudo rm allowed", "sudo rm /usr/local/bin/atomcode", KindNone, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			block := guard.Check(tc.command)
			if block.Kind != tc.kind {
				t.Fatalf("Check(%q).Kind = %v, want %v", tc.command, block.Kind, tc.kind)
			}
			if block.Program != tc.program {
				t.Fatalf("Check(%q).Program = %q, want %q", tc.command, block.Program, tc.program)
			}
		})
	}
}

func TestBlacklistWinsOverTUI(t *testing.T) {
	guard := New([]string{"rm *"})
	block := guard.Check("sudo rm -rf / && vim /etc/hosts")
	if block.Kind != KindBlacklist || block.Pattern != "rm *" {
		t.Fatalf("block = %+v", block)
	}
}

func TestParseExtractsRedirects(t *testing.T) {
	cases := []struct {
		name    string
		command string
		target  string
		static  bool
		op      syntax.RedirOperator
	}{
		{"stdout", "cmd > /tmp/x", "/tmp/x", true, syntax.RdrOut},
		{"append", "cmd >> /tmp/x", "/tmp/x", true, syntax.AppOut},
		{"all output", "cmd &> /tmp/x", "/tmp/x", true, syntax.RdrAll},
		{"dynamic target", `cmd > "$FILE"`, "", false, syntax.RdrOut},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parsed := parseCommandLine(tc.command)
			if len(parsed.redirects) != 1 {
				t.Fatalf("redirects = %+v", parsed.redirects)
			}
			if parsed.redirects[0].target != tc.target || parsed.redirects[0].static != tc.static || parsed.redirects[0].op != tc.op {
				t.Fatalf("redirect = %+v", parsed.redirects[0])
			}
		})
	}
}

func TestUnparseableCommandFallsBackWithoutPanic(t *testing.T) {
	guard := New([]string{"rm *"})
	if block := guard.Check("echo 'unterminated"); block.Blocked() {
		t.Fatalf("unterminated echo blocked: %+v", block)
	}
	if block := guard.Check("vim 'unterminated"); block.Kind != KindTUIEditor {
		t.Fatalf("unterminated vim not blocked: %+v", block)
	}
}

func staticWords(values ...string) []word {
	words := make([]word, len(values))
	for i, value := range values {
		words[i] = word{text: value, static: true}
	}
	return words
}
