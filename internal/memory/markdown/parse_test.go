package markdown

import (
	"strings"
	"testing"
	"time"

	"github.com/llaoj/aist/internal/memory"
)

func TestParseYAMLFrontmatter(t *testing.T) {
	data := []byte(`---
type: db
modified: 2026-10-27T09:30:00Z
source: verified
---

# prod-mysql

## Memory

- MySQL primary 10.2.0.5
`)
	mem, warnings, err := Decode("/memory/mysql.md", data)
	if err != nil || len(warnings) != 0 {
		t.Fatalf("Parse: %v warnings=%+v", err, warnings)
	}
	if mem.Name != "prod-mysql" || mem.Type != "db" || mem.Source != "verified" {
		t.Fatalf("unexpected metadata: %+v", mem)
	}
	if mem.Modified.Format(time.RFC3339) != "2026-10-27T09:30:00Z" {
		t.Fatalf("modified = %s", mem.Modified)
	}
	if !strings.Contains(mem.Content, "MySQL primary") {
		t.Fatalf("content = %q", mem.Content)
	}
}

func TestParseMissingTitleFallsBackToFilename(t *testing.T) {
	mem, warnings, err := Decode("/memory/legacy.md", []byte("## Memory\n\n- fact\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(warnings) == 0 || !strings.Contains(warnings[0].Message, "missing # title") {
		t.Fatalf("warnings = %+v", warnings)
	}
	if mem.Name != "legacy" {
		t.Fatalf("name = %q", mem.Name)
	}
}

func TestParseKeepsUnknownSections(t *testing.T) {
	data := []byte("# prod\n\n## Targets\n\n- web1\n\n## Notes\n\n- note\n")
	mem, _, err := Decode("/memory/prod.md", data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !strings.Contains(mem.Content, "## Targets") || !strings.Contains(mem.Content, "## Notes") {
		t.Fatalf("unknown sections lost: %q", mem.Content)
	}
}

func TestParseInvalidDateWarns(t *testing.T) {
	data := []byte("---\nupdated: not-a-date\n---\n\n# prod\n\n## Memory\n\n- fact\n")
	_, warnings, err := Decode("/memory/prod.md", data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(warnings) == 0 || !strings.Contains(warnings[0].Message, "invalid updated") {
		t.Fatalf("warnings = %+v", warnings)
	}
}

func TestRenderRoundTrip(t *testing.T) {
	updated := time.Date(2026, 10, 28, 10, 0, 0, 0, time.FixedZone("CST", 8*3600))
	original := memory.Entry{
		Path:     "/memory/prod-k8s.md",
		Name:     "prod-k8s",
		Updated:  updated,
		Modified: updated,
		Source:   "user statement",
		Summary:  "Kubernetes cluster",
		Type:     "k8s",
		Content:  "- master: 192.0.2.1\n- worker: 192.0.2.2",
	}
	data, err := Encode(original)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	text := string(data)
	if !strings.HasPrefix(text, "---\n") {
		t.Fatalf("missing YAML frontmatter: %q", text)
	}
	for _, want := range []string{"updated: ", "source: ", "summary: ", "type: k8s"} {
		if !strings.Contains(text, want) {
			t.Fatalf("frontmatter missing %q: %q", want, text)
		}
	}
	parsed, warnings, err := Decode(original.Path, data)
	if err != nil || len(warnings) != 0 {
		t.Fatalf("Parse: %v warnings=%+v", err, warnings)
	}
	if parsed.Name != original.Name || parsed.Content != original.Content {
		t.Fatalf("round trip mismatch: %+v", parsed)
	}
	if parsed.Summary != original.Summary || parsed.Type != original.Type || parsed.Source != original.Source {
		t.Fatalf("metadata mismatch: %+v", parsed)
	}
	if !parsed.Updated.Equal(original.Updated) {
		t.Fatalf("updated = %s, want %s", parsed.Updated, original.Updated)
	}
}

func TestRenderRequiresName(t *testing.T) {
	if _, err := Encode(memory.Entry{}); err == nil {
		t.Fatal("expected error for empty name")
	}
}
