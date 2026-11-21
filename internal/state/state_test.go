package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenCreatesPrivateLayout(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".aist")
	store, err := EnsureRoot(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	dirs := []string{
		store.Paths.Root,
		store.Paths.FileEditDir,
		store.Paths.BackupsDir("fileedit"),
		store.Paths.SessionsDir,
		store.Paths.MemoryDir,
	}
	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("Stat %s: %v", dir, err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Fatalf("%s mode = %o", dir, info.Mode().Perm())
		}
	}
	if store.Paths.TokenFile != filepath.Join(root, "token.json") {
		t.Fatalf("TokenFile = %q", store.Paths.TokenFile)
	}
	if store.Paths.PreferencesFile != filepath.Join(root, "preferences.json") {
		t.Fatalf("PreferencesFile = %q", store.Paths.PreferencesFile)
	}
}

func TestEnsureRootDefaultsToHomeAist(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	store, err := EnsureRoot("")
	if err != nil {
		t.Fatalf("EnsureRoot: %v", err)
	}
	wantRoot := filepath.Join(home, DefaultRootName)
	if store.Paths.Root != wantRoot {
		t.Fatalf("Root = %q, want %q", store.Paths.Root, wantRoot)
	}
	if store.Paths.TokenFile != filepath.Join(wantRoot, "token.json") {
		t.Fatalf("TokenFile = %q", store.Paths.TokenFile)
	}
	if store.Paths.PreferencesFile != filepath.Join(wantRoot, "preferences.json") {
		t.Fatalf("PreferencesFile = %q", store.Paths.PreferencesFile)
	}
	info, err := os.Stat(wantRoot)
	if err != nil {
		t.Fatalf("Stat %s: %v", wantRoot, err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("%s mode = %o", wantRoot, info.Mode().Perm())
	}
}

func TestBackupKeepsVersionsAndSkipsDuplicates(t *testing.T) {
	store := NewFromPaths(NewPaths(t.TempDir()))
	original := "/etc/nginx/nginx.conf"
	if err := store.Backups.Backup("fileedit", original, []byte("one"), 0o644); err != nil {
		t.Fatalf("first backup: %v", err)
	}
	if err := store.Backups.Backup("fileedit", original, []byte("two"), 0o644); err != nil {
		t.Fatalf("second backup: %v", err)
	}
	if err := store.Backups.Backup("fileedit", original, []byte("two"), 0o644); err != nil {
		t.Fatalf("duplicate backup: %v", err)
	}

	dir := store.Paths.BackupDir("fileedit", original)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	backupCount := 0
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".bak" {
			backupCount++
		}
	}
	if backupCount != 2 {
		t.Fatalf("backups = %d, want 2", backupCount)
	}
	meta, err := readBackupMeta(filepath.Join(dir, "meta.json"))
	if err != nil {
		t.Fatalf("read meta: %v", err)
	}
	if meta.OriginalPath != original || meta.Mode != "0644" || meta.VersionCount != 2 {
		t.Fatalf("meta = %+v", meta)
	}
}

func TestBackupPrunesToMaxVersions(t *testing.T) {
	store := NewFromPaths(NewPaths(t.TempDir()))
	store.Backups.MaxVersions = 2
	original := "/tmp/file.conf"
	base := time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		store.Backups.Now = func() time.Time { return base.Add(time.Duration(i) * time.Minute) }
		if err := store.Backups.Backup("fileedit", original, []byte{byte('a' + i)}, 0o600); err != nil {
			t.Fatalf("backup %d: %v", i, err)
		}
	}
	dir := store.Paths.BackupDir("fileedit", original)
	entries, _ := os.ReadDir(dir)
	backupCount := 0
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".bak" {
			backupCount++
		}
	}
	if backupCount != 2 {
		t.Fatalf("backups = %d, want 2", backupCount)
	}
}
