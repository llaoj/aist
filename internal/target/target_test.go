package target

import (
	"strings"
	"testing"
)

func TestParseLocalTargets(t *testing.T) {
	var zero Target
	if !zero.IsLocal() || zero.String() != LocalName || zero.Host() != "" {
		t.Fatalf("zero Target = %+v, want local", zero)
	}
	for _, raw := range []string{"", "  ", "local", "LOCAL", "localhost"} {
		parsed, err := New(raw)
		if err != nil {
			t.Fatalf("Parse(%q): %v", raw, err)
		}
		if !parsed.IsLocal() || parsed.String() != LocalName || parsed.Host() != "" {
			t.Fatalf("Parse(%q) = %+v, want local", raw, parsed)
		}
	}
}

func TestParseRemoteTargets(t *testing.T) {
	cases := map[string]string{
		"web1":             "web1",
		" user@10.0.0.1 ":  "user@10.0.0.1",
		"k8s-master-01":    "k8s-master-01",
		"::1":              "::1",
		"[2001:db8::1]":    "[2001:db8::1]",
		"deploy@[::1]":     "deploy@[::1]",
		"host_with_under":  "host_with_under",
		"host.with-dashes": "host.with-dashes",
	}
	for raw, want := range cases {
		parsed, err := New(raw)
		if err != nil {
			t.Fatalf("Parse(%q): %v", raw, err)
		}
		if parsed.IsLocal() || parsed.Host() != want || parsed.String() != want {
			t.Fatalf("Parse(%q) = %+v, want %q", raw, parsed, want)
		}
	}
}

func TestParseRejectsUnsafeTargets(t *testing.T) {
	for _, raw := range []string{
		"-oProxyCommand=evil",
		"web1 rm -rf /",
		"web1;id",
		"web1|id",
		"web1`id`",
		"web1$(id)",
		"@host",
		"user@",
		"web1\nother",
		"web1\x00other",
		"web1/../../etc",
		".",
		"..",
	} {
		if _, err := New(raw); err == nil {
			t.Fatalf("Parse(%q) accepted unsafe target", raw)
		}
	}
	if _, err := New(strings.Repeat("a", maxLength+1)); err == nil {
		t.Fatal("long target accepted")
	}
}

func TestRenderPath(t *testing.T) {
	if got := Local().RenderPath("/etc/hosts", false); got != "/etc/hosts" {
		t.Fatalf("local path = %q", got)
	}
	if got := Local().RenderPath("/etc/hosts", true); got != "/etc/hosts (sudo)" {
		t.Fatalf("local sudo path = %q", got)
	}
	remote, err := New("web1")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := remote.RenderPath("/etc/nginx/nginx.conf", true); got != "[web1] /etc/nginx/nginx.conf (sudo)" {
		t.Fatalf("remote sudo path = %q", got)
	}
}
