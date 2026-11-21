package file

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/llaoj/aist/internal/memory"
)

func writeMemory(t *testing.T, path, name, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := "# " + name + "\n\n## Memory\n\n" + body + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestLoaderLoadsManagedDirectory(t *testing.T) {
	stateDir := t.TempDir()
	writeMemory(t, filepath.Join(stateDir, "memory", "global.md"), "global", "- global fact")
	writeMemory(t, filepath.Join(stateDir, "extra", "team.md"), "team", "- team fact")

	loader := NewLoader(LoadOptions{Dir: filepath.Join(stateDir, "memory"), MaxBytes: 4096})
	result, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(result.Memories) != 1 || result.Memories[0].Name != "global" {
		t.Fatalf("memories = %+v", result.Memories)
	}
	if result.Report.Loaded != 1 {
		t.Fatalf("loaded = %d", result.Report.Loaded)
	}
}

func TestLoaderSkipsHiddenAndNonMarkdown(t *testing.T) {
	stateDir := t.TempDir()
	writeMemory(t, filepath.Join(stateDir, "memory", "visible.md"), "visible", "- fact")
	writeMemory(t, filepath.Join(stateDir, "memory", ".hidden.md"), "hidden", "- secret")
	if err := os.WriteFile(filepath.Join(stateDir, "memory", "notes.txt"), []byte("ignored"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	result, err := NewLoader(LoadOptions{Dir: filepath.Join(stateDir, "memory"), MaxBytes: 4096}).Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(result.Memories) != 1 || result.Memories[0].Name != "visible" {
		t.Fatalf("memories = %+v", result.Memories)
	}
}

func TestLoaderRejectsSymlinkFile(t *testing.T) {
	stateDir := t.TempDir()
	target := filepath.Join(stateDir, "outside.md")
	writeMemory(t, target, "outside", "- fact")
	link := filepath.Join(stateDir, "memory", "linked.md")
	if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	result, err := NewLoader(LoadOptions{Dir: filepath.Join(stateDir, "memory"), MaxBytes: 4096}).Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(result.Memories) != 0 {
		t.Fatalf("symlink memory was loaded: %+v", result.Memories)
	}
	if len(result.Report.Warnings) == 0 || !strings.Contains(result.Report.Warnings[0], "symlink not allowed") {
		t.Fatalf("warnings = %+v", result.Report.Warnings)
	}
}

func TestLoaderRejectsSymlinkRoot(t *testing.T) {
	stateDir := t.TempDir()
	target := filepath.Join(stateDir, "other")
	writeMemory(t, filepath.Join(target, "a.md"), "a", "- fact")
	if err := os.Symlink(target, filepath.Join(stateDir, "memory")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	result, err := NewLoader(LoadOptions{Dir: filepath.Join(stateDir, "memory"), MaxBytes: 4096}).Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(result.Memories) != 0 {
		t.Fatalf("symlinked root was loaded: %+v", result.Memories)
	}
	if len(result.Report.Warnings) == 0 || !strings.Contains(result.Report.Warnings[0], "must not be a symlink") {
		t.Fatalf("warnings = %+v", result.Report.Warnings)
	}
}

func TestLoaderWarnsOnLoosePermissions(t *testing.T) {
	stateDir := t.TempDir()
	path := filepath.Join(stateDir, "memory", "prod.md")
	writeMemory(t, path, "prod", "- fact")
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("Chmod: %v", err)
	}

	result, err := NewLoader(LoadOptions{Dir: filepath.Join(stateDir, "memory"), MaxBytes: 4096}).Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(result.Memories) != 1 {
		t.Fatalf("memories = %+v", result.Memories)
	}
	found := false
	for _, warning := range result.Report.Warnings {
		if strings.Contains(warning, "expected 0600") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing permission warning: %+v", result.Report.Warnings)
	}
}

func TestLoaderEnforcesBudget(t *testing.T) {
	stateDir := t.TempDir()
	first := filepath.Join(stateDir, "memory", "a.md")
	second := filepath.Join(stateDir, "memory", "b.md")
	writeMemory(t, first, "a", "- first fact")
	writeMemory(t, second, "b", "- second fact")

	firstInfo, _ := os.Stat(first)
	loader := NewLoader(LoadOptions{Dir: filepath.Join(stateDir, "memory"), MaxBytes: int(firstInfo.Size()) + 5})
	result, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(result.Memories) != 1 || result.Memories[0].Name != "a" {
		t.Fatalf("memories = %+v", result.Memories)
	}
	var skipped memory.FileStatus
	for _, file := range result.Report.Files {
		if file.Path == second {
			skipped = file
		}
	}
	if skipped.Loaded || !strings.Contains(skipped.Reason, "budget") {
		t.Fatalf("second file status = %+v", skipped)
	}
}

func TestLoaderReportsParseWarnings(t *testing.T) {
	stateDir := t.TempDir()
	path := filepath.Join(stateDir, "memory", "warn.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte("## Memory\n\n- no title\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	result, err := NewLoader(LoadOptions{Dir: filepath.Join(stateDir, "memory"), MaxBytes: 4096}).Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(result.Memories) != 1 || result.Memories[0].Name != "warn" {
		t.Fatalf("memories = %+v", result.Memories)
	}
	if len(result.Report.Warnings) == 0 || !strings.Contains(result.Report.Warnings[0], "missing # title") {
		t.Fatalf("warnings = %+v", result.Report.Warnings)
	}
}
