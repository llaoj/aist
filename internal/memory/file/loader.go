// Package file provides filesystem adapters for the Memory domain.
package file

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/llaoj/aist/internal/memory"
	"github.com/llaoj/aist/internal/memory/markdown"
)

// LoadOptions controls Memory discovery.
type LoadOptions struct {
	// Dir is the managed Memory directory (~/.aist/memory).
	Dir string
	// MaxBytes is the total content budget for loaded files.
	MaxBytes int
}

// Loader loads Memory files from the managed global directory.
type Loader struct {
	opts LoadOptions
}

// NewLoader creates a Memory file loader.
func NewLoader(opts LoadOptions) *Loader {
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = memory.DefaultMaxBytes
	}
	return &Loader{opts: opts}
}

// Load discovers, parses and budget-limits Memory files under Dir.
func (l *Loader) Load(ctx context.Context) (memory.LoadResult, error) {
	if err := ctx.Err(); err != nil {
		return memory.LoadResult{}, err
	}

	result := memory.LoadResult{
		Report: memory.Report{MaxBytes: l.opts.MaxBytes},
	}
	dir := strings.TrimSpace(l.opts.Dir)
	if dir == "" {
		return result, nil
	}
	dirStatus := memory.FileStatus{Path: filepath.Clean(dir), Source: memory.FileSourceGlobal}
	info, err := os.Lstat(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			dirStatus.Reason = err.Error()
			result.Report.Files = append(result.Report.Files, dirStatus)
			result.Report.Warnings = append(result.Report.Warnings, fmt.Sprintf("%s: %v", dir, err))
		}
		return result, nil
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		dirStatus.Reason = "memory directory must not be a symlink"
		result.Report.Files = append(result.Report.Files, dirStatus)
		result.Report.Warnings = append(result.Report.Warnings, fmt.Sprintf("%s: %s", dir, dirStatus.Reason))
		return result, nil
	case !info.IsDir():
		dirStatus.Reason = "memory path is not a directory"
		result.Report.Files = append(result.Report.Files, dirStatus)
		result.Report.Warnings = append(result.Report.Warnings, fmt.Sprintf("%s: %s", dir, dirStatus.Reason))
		return result, nil
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		result.Report.Warnings = append(result.Report.Warnings,
			fmt.Sprintf("%s: permissions %04o are too open; expected 0700", dir, perm))
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		result.Report.Warnings = append(result.Report.Warnings, fmt.Sprintf("%s: %v", dir, err))
		return result, nil
	}

	used := 0
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if err := l.loadEntry(ctx, dir, entry, &result, &used); err != nil {
			return result, err
		}
	}

	return result, nil
}

func (l *Loader) loadEntry(ctx context.Context, dir string, entry os.DirEntry, result *memory.LoadResult, used *int) error {
	if strings.HasPrefix(entry.Name(), ".") || !strings.HasSuffix(entry.Name(), ".md") {
		return nil
	}

	path := filepath.Clean(filepath.Join(dir, entry.Name()))
	status := memory.FileStatus{Path: path, Source: memory.FileSourceGlobal}

	info, err := os.Lstat(path)
	if err != nil {
		status.Reason = err.Error()
		if !os.IsNotExist(err) {
			result.Report.Warnings = append(result.Report.Warnings, fmt.Sprintf("%s: %v", path, err))
		}
		result.Report.Files = append(result.Report.Files, status)
		return nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		status.Reason = "symlink not allowed"
		result.Report.Files = append(result.Report.Files, status)
		result.Report.Warnings = append(result.Report.Warnings, fmt.Sprintf("%s: %s", path, status.Reason))
		return nil
	}
	if !info.Mode().IsRegular() {
		status.Reason = "not a regular file"
		result.Report.Files = append(result.Report.Files, status)
		result.Report.Warnings = append(result.Report.Warnings, fmt.Sprintf("%s: %s", path, status.Reason))
		return nil
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		result.Report.Warnings = append(result.Report.Warnings,
			fmt.Sprintf("%s: permissions %04o are too open; expected 0600", path, perm))
	}

	status.Bytes = int(info.Size())
	if int(info.Size()) > l.opts.MaxBytes {
		status.Reason = fmt.Sprintf("exceeds max_bytes (%d)", l.opts.MaxBytes)
		result.Report.Files = append(result.Report.Files, status)
		return nil
	}
	if *used+int(info.Size()) > l.opts.MaxBytes {
		status.Reason = fmt.Sprintf("memory budget exceeded (%d/%d bytes)", *used, l.opts.MaxBytes)
		result.Report.Files = append(result.Report.Files, status)
		return nil
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		status.Reason = err.Error()
		result.Report.Files = append(result.Report.Files, status)
		result.Report.Warnings = append(result.Report.Warnings, fmt.Sprintf("%s: %v", path, err))
		return nil
	}

	parsed, warnings, err := markdown.Decode(path, data)
	if err != nil {
		status.Reason = err.Error()
		result.Report.Files = append(result.Report.Files, status)
		result.Report.Warnings = append(result.Report.Warnings, fmt.Sprintf("%s: %v", path, err))
		return nil
	}
	for _, warning := range warnings {
		result.Report.Warnings = append(result.Report.Warnings,
			fmt.Sprintf("%s:%d: %s", path, warning.Line, warning.Message))
	}

	status.Loaded = true
	result.Report.Files = append(result.Report.Files, status)
	result.Report.Loaded++
	result.Report.Bytes += int(info.Size())
	result.Memories = append(result.Memories, parsed)
	*used += int(info.Size())
	return nil
}
