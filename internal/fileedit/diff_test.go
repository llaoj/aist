package fileedit

import (
	"strings"
	"testing"

	"github.com/llaoj/aist/internal/target"
)

func TestRenderSnippetDiffCollapsesCommonContextWithoutFakeLineNumbers(t *testing.T) {
	oldContent := "line1\nline2\nline3\nline4\nworker_connections 768;\nkeepalive_timeout 65;\n}\n"
	newContent := "line1\nline2\nline3\nline4\nworker_connections 4096;\nkeepalive_timeout 65;\n}\n"
	web1, err := target.New("web1")
	if err != nil {
		t.Fatalf("target.New: %v", err)
	}

	got := RenderSnippetDiff(web1, "/etc/nginx/nginx.conf", oldContent, newContent)
	want := strings.Join([]string{
		"--- web1:/etc/nginx/nginx.conf",
		"+++ web1:/etc/nginx/nginx.conf",
		" line2",
		" line3",
		" line4",
		"-worker_connections 768;",
		"+worker_connections 4096;",
		" keepalive_timeout 65;",
		" }",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("RenderSnippetDiff:\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderSnippetDiffNewFileUsesZeroHunkHeader(t *testing.T) {
	got := RenderSnippetDiff(target.Local(), "/etc/app.conf", "", "port=8080\nworkers=4\n")
	want := strings.Join([]string{
		"--- /etc/app.conf",
		"+++ /etc/app.conf",
		"@@ -0,0 +1,2 @@",
		"+port=8080",
		"+workers=4",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("RenderSnippetDiff new file:\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderFileDiffComputesRealLineNumbersAndMidLineChanges(t *testing.T) {
	original := []byte("l1\nl2\nl3\nl4\nHost web1\n  HostName 10.0.0.1\n  User deploy\nl8\nl9\nl10\n")
	updated := []byte("l1\nl2\nl3\nl4\nHost web1\n  HostName 10.0.0.2\n  User deploy\nl8\nl9\nl10\n")

	got := renderFileDiff(target.Local(), "/etc/ssh/ssh_config", original, updated)
	want := strings.Join([]string{
		"--- /etc/ssh/ssh_config",
		"+++ /etc/ssh/ssh_config",
		"@@ -3,7 +3,7 @@",
		" l3",
		" l4",
		" Host web1",
		"-  HostName 10.0.0.1",
		"+  HostName 10.0.0.2",
		"   User deploy",
		" l8",
		" l9",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("renderFileDiff:\n got:\n%s\nwant:\n%s", got, want)
	}
}
