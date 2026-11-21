// Package state owns the single writable AIST state root (~/.aist).
//
// The package centralizes directory layout, permissions and file backups. It
// does not know about configuration, sessions or Memory contents.
package state

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// DefaultRootName is the per-user state directory name.
const DefaultRootName = ".aist"

const (
	dirMode  os.FileMode = 0o700
	fileMode os.FileMode = 0o600
)

// Paths describes the fixed layout below the state root.
type Paths struct {
	Root            string
	FileEditDir     string
	SessionsDir     string
	MemoryDir       string
	TokenFile       string
	PreferencesFile string
}

// NewPaths builds the path layout for a state root.
func NewPaths(root string) Paths {
	root = filepath.Clean(root)
	return Paths{
		Root:            root,
		FileEditDir:     filepath.Join(root, "fileedit"),
		SessionsDir:     filepath.Join(root, "sessions"),
		MemoryDir:       filepath.Join(root, "memory"),
		TokenFile:       filepath.Join(root, "token.json"),
		PreferencesFile: filepath.Join(root, "preferences.json"),
	}
}

// DefaultForHome resolves <home>/.aist.
func DefaultForHome(home string) Paths {
	return NewPaths(filepath.Join(home, DefaultRootName))
}

// ResolvePaths builds the path layout for root, defaulting to <home>/.aist
// when root is empty.
func ResolvePaths(root string) (Paths, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Paths{}, fmt.Errorf("failed to determine home directory: %w", err)
		}
		return DefaultForHome(home), nil
	}
	return NewPaths(root), nil
}

// Ensure creates the state directories and tightens their permissions.
func (p Paths) Ensure() error {
	dirs := []string{
		p.Root,
		p.FileEditDir,
		p.BackupsDir("fileedit"),
		p.SessionsDir,
		p.MemoryDir,
	}
	for _, dir := range dirs {
		if err := ensureDir(dir); err != nil {
			return err
		}
	}
	return nil
}

// SessionDir returns the directory for a session ID.
func (p Paths) SessionDir(id string) string {
	return filepath.Join(p.SessionsDir, safeSegment(id))
}

// BackupsDir returns the backup root for a domain.
func (p Paths) BackupsDir(category string) string {
	return filepath.Join(p.Root, safeSegment(category), "backups")
}

// BackupDir returns the mirrored backup directory for an original path.
func (p Paths) BackupDir(category, originalPath string) string {
	return filepath.Join(p.BackupsDir(category), mirrorPath(originalPath))
}

func ensureDir(dir string) error {
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return fmt.Errorf("failed to create state directory %s: %w", dir, err)
	}
	if err := os.Chmod(dir, dirMode); err != nil {
		return fmt.Errorf("failed to set permissions on %s: %w", dir, err)
	}
	return nil
}

func safeSegment(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, string(filepath.Separator), "_")
	value = strings.ReplaceAll(value, "..", "_")
	if value == "" || value == "." {
		return "default"
	}
	return value
}

// mirrorPath converts an absolute path into a relative path safe to use below
// a domain's backups root. The root prefix is stripped so users can browse a
// familiar directory tree.
func mirrorPath(original string) string {
	if original == "" {
		return "default"
	}
	cleaned := filepath.Clean(original)
	volume := filepath.VolumeName(cleaned)
	cleaned = strings.TrimPrefix(cleaned, volume)
	cleaned = strings.TrimLeft(cleaned, string(filepath.Separator))

	if runtime.GOOS == "windows" {
		replacer := strings.NewReplacer(":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
		cleaned = replacer.Replace(cleaned)
		cleaned = strings.ReplaceAll(cleaned, "\\", "/")
	}
	if cleaned == "" {
		return "default"
	}
	return cleaned
}
