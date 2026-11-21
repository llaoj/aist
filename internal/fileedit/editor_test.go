package fileedit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
}

func TestPrepareApplyAndBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ssh_config")
	original := "Host web1\n  HostName 10.0.0.1\n  User deploy\n"
	writeTestFile(t, path, original, 0o640)

	editor := NewEditor(EditorOptions{HomeDir: dir})
	change, err := editor.Prepare(context.Background(), EditRequest{
		Path:       path,
		OldContent: "  HostName 10.0.0.1\n",
		NewContent: "  HostName 10.0.0.2\n",
		Reason:     "Move web1",
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if !change.Exists || change.Perm != 0o640 {
		t.Fatalf("change = %+v", change)
	}
	if !strings.Contains(change.Diff, "-  HostName 10.0.0.1") || !strings.Contains(change.Diff, "+  HostName 10.0.0.2") {
		t.Fatalf("diff = %q", change.Diff)
	}

	if err := editor.Apply(context.Background(), change); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	updated, _ := os.ReadFile(path)
	if !strings.Contains(string(updated), "10.0.0.2") {
		t.Fatalf("updated content = %q", updated)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
	backups := 0
	backupContent := ""
	_ = filepath.WalkDir(filepath.Join(dir, ".aist", "fileedit", "backups"), func(backupPath string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(entry.Name(), ".bak") {
			return nil
		}
		backups++
		data, _ := os.ReadFile(backupPath)
		backupContent += string(data)
		return nil
	})
	if backups != 1 {
		t.Fatalf("backups = %d", backups)
	}
	if backupContent != original {
		t.Fatalf("backup = %q, want %q", backupContent, original)
	}
}

func TestPrepareCreatesPrivateNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.conf")
	editor := NewEditor(EditorOptions{HomeDir: dir})

	change, err := editor.Prepare(context.Background(), EditRequest{
		Path:       path,
		NewContent: "setting=1\n",
		Reason:     "Create config",
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if change.Exists {
		t.Fatal("new file marked as existing")
	}
	if !strings.Contains(change.Diff, "+setting=1") {
		t.Fatalf("diff = %q", change.Diff)
	}
	if err := editor.Apply(context.Background(), change); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
}

func TestPrepareRejectsUnsafeEdits(t *testing.T) {
	dir := t.TempDir()
	editor := NewEditor(EditorOptions{HomeDir: dir, MaxBytes: 16})

	file := filepath.Join(dir, "file.conf")
	writeTestFile(t, file, "a\nb\na\n", 0o600)

	cases := []struct {
		name    string
		request EditRequest
		want    error
	}{
		{"relative", EditRequest{Path: "file.conf", NewContent: "x"}, ErrUnsafePath},
		{"missing old", EditRequest{Path: file, NewContent: "x"}, ErrMissingOldContent},
		{"no match", EditRequest{Path: file, OldContent: "missing", NewContent: "x"}, ErrNoMatch},
		{"ambiguous", EditRequest{Path: file, OldContent: "a", NewContent: "x"}, ErrAmbiguousMatch},
		{"no change", EditRequest{Path: file, OldContent: "b\n", NewContent: "b\n"}, ErrIdenticalContent},
		{"too large", EditRequest{Path: file, OldContent: "a", NewContent: strings.Repeat("x", 100)}, ErrTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := editor.Prepare(context.Background(), tc.request); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestPrepareRejectsPrivateKeysAndSpecialFiles(t *testing.T) {
	dir := t.TempDir()
	const secret = "-----BEGIN OPENSSH PRIVATE KEY-----\nsecret-material\n"
	keyPath := filepath.Join(dir, "id_rsa")
	writeTestFile(t, keyPath, secret, 0o600)

	editor := NewEditor(EditorOptions{HomeDir: dir})
	if _, err := editor.Prepare(context.Background(), EditRequest{
		Path:       keyPath,
		OldContent: "secret-material",
		NewContent: "other",
	}); !errors.Is(err, ErrPrivateKey) {
		t.Fatalf("err = %v, want private key", err)
	} else if strings.Contains(err.Error(), "secret-material") {
		t.Fatalf("error leaked private key content: %v", err)
	}

	normalPath := filepath.Join(dir, "notes.txt")
	writeTestFile(t, normalPath, secret, 0o600)
	if _, err := editor.Prepare(context.Background(), EditRequest{
		Path:       normalPath,
		OldContent: "secret-material",
		NewContent: "other",
	}); !errors.Is(err, ErrPrivateKey) {
		t.Fatalf("content err = %v, want private key", err)
	}

	symlink := filepath.Join(dir, "link.conf")
	if err := os.Symlink(normalPath, symlink); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	if _, err := editor.Prepare(context.Background(), EditRequest{
		Path:       symlink,
		OldContent: "x",
		NewContent: "y",
	}); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("symlink err = %v, want unsafe", err)
	}

	if _, err := editor.Prepare(context.Background(), EditRequest{
		Path:       dir,
		OldContent: "x",
		NewContent: "y",
	}); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("directory err = %v, want unsafe", err)
	}
}

func TestPrepareExpandsHome(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".ssh", "config")
	writeTestFile(t, path, "Host old\n", 0o600)
	editor := NewEditor(EditorOptions{HomeDir: dir})

	change, err := editor.Prepare(context.Background(), EditRequest{
		Path:       "~/.ssh/config",
		OldContent: "Host old\n",
		NewContent: "Host new\n",
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if change.Path != path {
		t.Fatalf("path = %q, want %q", change.Path, path)
	}
}

func TestApplyRejectsConcurrentChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.conf")
	writeTestFile(t, path, "a\n", 0o600)
	editor := NewEditor(EditorOptions{HomeDir: dir})

	change, err := editor.Prepare(context.Background(), EditRequest{
		Path:       path,
		OldContent: "a\n",
		NewContent: "b\n",
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	writeTestFile(t, path, "external\n", 0o600)

	if err := editor.Apply(context.Background(), change); err == nil {
		t.Fatal("expected concurrent change error")
	}
	current, _ := os.ReadFile(path)
	if string(current) != "external\n" {
		t.Fatalf("file was overwritten: %q", current)
	}
}

func TestApplyRejectsNewFileThatAppeared(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.conf")
	editor := NewEditor(EditorOptions{HomeDir: dir})

	change, err := editor.Prepare(context.Background(), EditRequest{
		Path:       path,
		NewContent: "new\n",
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	writeTestFile(t, path, "appeared\n", 0o600)

	if err := editor.Apply(context.Background(), change); err == nil {
		t.Fatal("expected existing file error")
	}
	current, _ := os.ReadFile(path)
	if string(current) != "appeared\n" {
		t.Fatalf("file was overwritten: %q", current)
	}
}
