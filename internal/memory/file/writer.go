package file

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/llaoj/aist/internal/memory"
	"github.com/llaoj/aist/internal/memory/markdown"
	"github.com/llaoj/aist/internal/storage"
)

// WriterOptions configures the Memory file writer.
type WriterOptions struct {
	// Dir is the default directory for new Memory files.
	Dir      string
	MaxBytes int
	FileMode os.FileMode
	DirMode  os.FileMode
	// Now is injected in tests to make timestamps deterministic.
	Now func() time.Time
}

// Writer writes one Memory per Markdown file.
type Writer struct {
	opts WriterOptions
}

// NewWriter creates a Memory file writer.
func NewWriter(opts WriterOptions) *Writer {
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = memory.DefaultMaxBytes
	}
	if opts.FileMode == 0 {
		opts.FileMode = 0o600
	}
	if opts.DirMode == 0 {
		opts.DirMode = 0o700
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Writer{opts: opts}
}

// Append adds content to an existing Memory or creates a new one.
func (w *Writer) Append(ctx context.Context, destination memory.WriteDestination, content memory.WriteContent) (memory.Entry, error) {
	body, err := w.prepareBody(ctx, content.Content)
	if err != nil {
		return memory.Entry{}, err
	}
	name, path, err := w.resolve(destination)
	if err != nil {
		return memory.Entry{}, err
	}

	mem, exists, err := w.read(path)
	if err != nil {
		return memory.Entry{}, err
	}
	now := w.opts.Now()
	if exists {
		mem.Content = strings.TrimSpace(mem.Content + "\n" + body)
		mem.Updated = now
		mem.Modified = now
		applyMetadata(&mem, content, false)
	} else {
		mem = memory.Entry{
			Path:     path,
			Name:     name,
			Updated:  now,
			Modified: now,
			Content:  body,
		}
		applyMetadata(&mem, content, true)
	}

	return w.persist(mem)
}

// Replace overwrites an existing Memory with new content, creating it when
// missing.
func (w *Writer) Upsert(ctx context.Context, destination memory.WriteDestination, content memory.WriteContent) (memory.Entry, error) {
	body, err := w.prepareBody(ctx, content.Content)
	if err != nil {
		return memory.Entry{}, err
	}
	name, path, err := w.resolve(destination)
	if err != nil {
		return memory.Entry{}, err
	}

	mem, _, err := w.read(path)
	if err != nil {
		return memory.Entry{}, err
	}
	now := w.opts.Now()
	mem.Path = path
	mem.Name = name
	mem.Content = body
	mem.Updated = now
	mem.Modified = now
	applyMetadata(&mem, content, true)
	return w.persist(mem)
}

func (w *Writer) prepareBody(ctx context.Context, body string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return "", errors.New("memory content is empty")
	}
	return body, nil
}

func applyMetadata(mem *memory.Entry, content memory.WriteContent, deriveSummary bool) {
	if summary := strings.TrimSpace(content.Summary); summary != "" {
		mem.Summary = summary
	} else if deriveSummary || strings.TrimSpace(mem.Summary) == "" {
		mem.Summary = memory.DeriveSummary(*mem, memory.DefaultSummaryLimit)
	}
	if memType := strings.TrimSpace(content.Type); memType != "" {
		mem.Type = memType
	}
	if source := strings.TrimSpace(content.Source); source != "" {
		mem.Source = source
	}
}

func (w *Writer) read(path string) (memory.Entry, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return memory.Entry{}, false, nil
		}
		return memory.Entry{}, false, fmt.Errorf("failed to read memory %s: %w", path, err)
	}
	mem, _, err := markdown.Decode(path, data)
	if err != nil {
		return memory.Entry{}, false, fmt.Errorf("failed to parse memory %s: %w", path, err)
	}
	return mem, true, nil
}

func (w *Writer) persist(mem memory.Entry) (memory.Entry, error) {
	data, err := markdown.Encode(mem)
	if err != nil {
		return memory.Entry{}, err
	}
	if len(data) > w.opts.MaxBytes {
		return memory.Entry{}, fmt.Errorf("memory %s is %d bytes, exceeds max_bytes %d",
			mem.Name, len(data), w.opts.MaxBytes)
	}

	if err := storage.WriteFile(mem.Path, data, storage.WriteOptions{
		FileMode: w.opts.FileMode,
		DirMode:  w.opts.DirMode,
	}); err != nil {
		return memory.Entry{}, err
	}
	return mem, nil
}

func (w *Writer) resolve(destination memory.WriteDestination) (string, string, error) {
	name, err := memory.NormalizeName(destination.Name)
	if err != nil {
		return "", "", err
	}

	path := strings.TrimSpace(destination.Path)
	if path == "" {
		if strings.TrimSpace(w.opts.Dir) == "" {
			return "", "", errors.New("memory writer directory is empty")
		}
		path = filepath.Join(w.opts.Dir, name+".md")
	} else {
		path = filepath.Clean(path)
		if !filepath.IsAbs(path) {
			return "", "", fmt.Errorf("memory path must be absolute: %s", path)
		}
		if filepath.Ext(path) != ".md" {
			return "", "", fmt.Errorf("memory path must end with .md: %s", path)
		}
		if !withinDir(w.opts.Dir, path) {
			return "", "", fmt.Errorf("memory path must stay inside %s: %s", w.opts.Dir, path)
		}
	}
	return name, path, nil
}

func withinDir(dir, path string) bool {
	if strings.TrimSpace(dir) == "" {
		return false
	}
	root := filepath.Clean(dir)
	rel, err := filepath.Rel(root, filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
