package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteCreatesPrivateFileAndDirectories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "file.yaml")
	if err := WriteFile(path, []byte("data"), WriteOptions{FileMode: 0o600, DirMode: 0o700}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %o", info.Mode().Perm())
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("Stat dir: %v", err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode = %o", dirInfo.Mode().Perm())
	}
}

func TestWriteReplacesAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.yaml")
	opts := WriteOptions{FileMode: 0o600, DirMode: 0o700}
	if err := WriteFile(path, []byte("old"), opts); err != nil {
		t.Fatalf("first Write: %v", err)
	}
	if err := WriteFile(path, []byte("new"), opts); err != nil {
		t.Fatalf("second Write: %v", err)
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(current) != "new" {
		t.Fatalf("current = %q", current)
	}
}
