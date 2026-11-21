// Package storage provides low-level durable file primitives.
//
// It intentionally knows nothing about AIST state paths, backups or sessions.
// Callers decide where state lives and when a backup is required.
package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteOptions configures an atomic file write.
type WriteOptions struct {
	FileMode os.FileMode
	DirMode  os.FileMode
}

// WriteFile creates parent directories and atomically replaces path.
func WriteFile(path string, data []byte, opts WriteOptions) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, opts.DirMode); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	return ReplaceFile(path, data, opts.FileMode)
}

// ReplaceFile writes data through a temporary file, fsyncs it and renames it
// over path. The parent directory must already exist.
func ReplaceFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to set temporary file permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to write temporary file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to sync temporary file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close temporary file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to replace file %s: %w", path, err)
	}
	syncDir(dir)
	return nil
}

func syncDir(dir string) {
	if dirFile, err := os.Open(dir); err == nil {
		_ = dirFile.Sync()
		_ = dirFile.Close()
	}
}
