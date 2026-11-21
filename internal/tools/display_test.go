package tools

import (
	"strings"
	"testing"

	"github.com/llaoj/aist/internal/i18n"
)

func TestToolDisplaysInBothLanguages(t *testing.T) {
	en := i18n.New("en")
	zh := i18n.New("zh")

	shell := ShellCommand{Target: "web1", CommandLine: "df -h"}
	memorySearch := MemorySearchArgs{Query: "k8s master"}
	memorySave := MemorySaveArgs{Name: "prod-k8s", Operation: "append"}
	fileEdit := FileEditArgs{Path: "/etc/nginx/nginx.conf"}

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"shell en", shell.FormatInvocation(en), "[web1] $ df -h"},
		{"shell zh", shell.FormatInvocation(zh), "[web1] $ df -h"},
		{"local target en", (ShellCommand{CommandLine: "uptime"}).FormatInvocation(en), "[local] $ uptime"},
		{"local target zh", (ShellCommand{CommandLine: "uptime"}).FormatInvocation(zh), "[本机] $ uptime"},
		{"memory search en", memorySearch.FormatInvocation(en), `Memory search "k8s master"`},
		{"memory search zh", memorySearch.FormatInvocation(zh), "搜索记忆「k8s master」"},
		{"memory save en", memorySave.FormatInvocation(en), `Memory save "prod-k8s" (append)`},
		{"memory save zh", memorySave.FormatInvocation(zh), "保存记忆「prod-k8s」（追加）"},
		{"file edit en", fileEdit.FormatInvocation(en), "File edit /etc/nginx/nginx.conf"},
		{"file edit zh", fileEdit.FormatInvocation(zh), "编辑文件 /etc/nginx/nginx.conf"},
		{"remote sudo file edit en", (FileEditArgs{Target: "web1", Path: "/etc/nginx/nginx.conf", UseSudo: true}).FormatInvocation(en), "File edit [web1] /etc/nginx/nginx.conf (sudo)"},
		{"remote sudo file edit zh", (FileEditArgs{Target: "web1", Path: "/etc/nginx/nginx.conf", UseSudo: true}).FormatInvocation(zh), "编辑文件 [web1] /etc/nginx/nginx.conf (sudo)"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
}

func TestToolResultsLocalizeStatus(t *testing.T) {
	en := i18n.New("en")
	zh := i18n.New("zh")
	shell := ShellCommand{Target: "web1", CommandLine: "systemctl restart nginx"}

	if got := shell.FormatResult(ToolOutcomeSuccess, en); got != "[web1] $ systemctl restart nginx · success" {
		t.Fatalf("en result = %q", got)
	}
	if got := shell.FormatResult(ToolOutcomeFailed, zh); got != "[web1] $ systemctl restart nginx · 失败" {
		t.Fatalf("zh result = %q", got)
	}
	if got := shell.FormatResult("", zh); !strings.Contains(got, "[web1] $") || strings.Contains(got, "失败") {
		t.Fatalf("empty status result = %q", got)
	}
}

func TestParseInvocationRoundTrip(t *testing.T) {
	display, err := DecodeInvocation(ToolShellExecute, `{"command":"df -h","target":"web1"}`)
	if err != nil {
		t.Fatalf("ParseInvocation: %v", err)
	}
	if display.ToolName() != ToolShellExecute {
		t.Fatalf("tool name = %q", display.ToolName())
	}
	if got := display.FormatResult(ToolOutcomeSuccess, i18n.New("zh")); got != "[web1] $ df -h · 成功" {
		t.Fatalf("result = %q", got)
	}

	unknown, err := DecodeInvocation("future_tool", `{}`)
	if err != nil {
		t.Fatalf("unknown ParseInvocation: %v", err)
	}
	if got := unknown.FormatInvocation(i18n.New("en")); got != `Unknown tool "future_tool"` {
		t.Fatalf("unknown invocation = %q", got)
	}
}
