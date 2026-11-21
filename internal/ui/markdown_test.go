package ui

import (
	"strings"
	"testing"
)

func TestRenderMarkdown(t *testing.T) {
	src := "# 标题\n\n" +
		"普通文字，含 **加粗** 和 `代码`。\n\n" +
		"| 名称 | 数量 |\n" +
		"|------|------|\n" +
		"| pod  | 3    |\n" +
		"| svc  | 1    |\n\n" +
		"```bash\n" +
		"rm -rf *\n" +
		"echo `date`\n" +
		"```\n\n" +
		"- 第一项\n" +
		"  - 嵌套\n" +
		"- 第二项\n\n" +
		"1. one\n" +
		"2. two\n\n" +
		"> quote line one\n" +
		"> quote line two\n\n" +
		"[链接](https://example.com)\n"

	out := RenderMarkdown(src)
	t.Logf("rendered output:\n%s", out)

	want := []string{
		"标题",
		"加粗",
		"代码",
		"rm -rf *",
		"echo `date`",
		"┌",
		"│ 名称 │ 数量 │",
		"│ pod  │ 3    │",
		"- 第一项",
		"- 嵌套",
		"- 第二项",
		"1. one",
		"2. two",
		"│ quote line one",
		"│ quote line two",
		"链接 (https://example.com)",
	}
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("missing %q in output:\n%s", w, out)
		}
	}
}
