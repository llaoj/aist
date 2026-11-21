package file

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/llaoj/aist/internal/memory"
	"github.com/llaoj/aist/internal/memory/markdown"
)

func testWriter(t *testing.T, maxBytes int) *Writer {
	t.Helper()
	root := t.TempDir()
	return NewWriter(WriterOptions{
		Dir:      filepath.Join(root, "memory"),
		MaxBytes: maxBytes,
		Now:      func() time.Time { return time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC) },
	})
}

func appendMemory(w *Writer, name, body string) (memory.Entry, error) {
	return w.Append(context.Background(), memory.WriteDestination{Name: name}, memory.WriteContent{
		Content: body,
		Source:  "user",
	})
}

func TestWriterAppendCreatesPrivateFile(t *testing.T) {
	writer := testWriter(t, 4096)
	mem, err := appendMemory(writer, "prod-k8s", "- master: 10.0.0.1")
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if mem.Name != "prod-k8s" || !strings.Contains(mem.Content, "10.0.0.1") {
		t.Fatalf("mem = %+v", mem)
	}
	info, err := os.Stat(mem.Path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
}

func TestWriterAppendAddsToExistingMemory(t *testing.T) {
	writer := testWriter(t, 4096)
	first, err := appendMemory(writer, "prod", "- first")
	if err != nil {
		t.Fatalf("first Append: %v", err)
	}
	second, err := writer.Append(context.Background(), memory.WriteDestination{Name: "prod"}, memory.WriteContent{
		Content: "- second",
		Source:  "verified",
	})
	if err != nil {
		t.Fatalf("second Append: %v", err)
	}
	if !strings.Contains(second.Content, "first") || !strings.Contains(second.Content, "second") {
		t.Fatalf("content = %q", second.Content)
	}
	if second.Source != "verified" {
		t.Fatalf("source = %q", second.Source)
	}

	data, _ := os.ReadFile(first.Path)
	parsed, _, err := markdown.Decode(first.Path, data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !strings.Contains(parsed.Content, "first") || !strings.Contains(parsed.Content, "second") {
		t.Fatalf("persisted content = %q", parsed.Content)
	}
}

func TestWriterReplace(t *testing.T) {
	writer := testWriter(t, 4096)
	if _, err := appendMemory(writer, "prod", "- old"); err != nil {
		t.Fatalf("Append: %v", err)
	}
	replaced, err := writer.Upsert(context.Background(), memory.WriteDestination{Name: "prod"}, memory.WriteContent{
		Content: "- new",
		Source:  "user",
	})
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if strings.Contains(replaced.Content, "old") || !strings.Contains(replaced.Content, "new") {
		t.Fatalf("content = %q", replaced.Content)
	}
}

func TestWriterUsesExistingTargetPath(t *testing.T) {
	dir := t.TempDir()
	writer := NewWriter(WriterOptions{Dir: dir, MaxBytes: 4096})
	path := filepath.Join(dir, "project.md")
	if err := os.WriteFile(path, []byte("# project\n\n## Memory\n\n- old\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	mem, err := writer.Append(context.Background(), memory.WriteDestination{Name: "project", Path: path}, memory.WriteContent{
		Content: "- added",
	})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if mem.Path != path {
		t.Fatalf("path = %q, want %q", mem.Path, path)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "old") || !strings.Contains(string(data), "added") {
		t.Fatalf("content = %q", data)
	}
}

func TestWriterRejectsInvalidTargetAndBudget(t *testing.T) {
	writer := testWriter(t, 80)
	if _, err := appendMemory(writer, "../escape", "- bad"); err == nil {
		t.Fatal("expected invalid name error")
	}
	if _, err := writer.Append(context.Background(), memory.WriteDestination{Name: "prod", Path: "relative.md"}, memory.WriteContent{Content: "- bad"}); err == nil {
		t.Fatal("expected absolute path error")
	}
	if _, err := appendMemory(writer, "prod", strings.Repeat("x", 200)); err == nil {
		t.Fatal("expected budget error")
	}
}
