// Package fileedit prepares, previews and applies user-confirmed file changes.
//
// The package is intentionally independent from LLM and UI packages. Callers
// prepare a Change, show its diff, obtain confirmation, then apply it.
package fileedit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/llaoj/aist/internal/scriptexec"
	"github.com/llaoj/aist/internal/state"
	"github.com/llaoj/aist/internal/storage"
	"github.com/llaoj/aist/internal/target"
	"github.com/llaoj/aist/internal/util"
)

var privateKeyMarker = regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----`)

// EditorOptions configures an Editor.
type EditorOptions struct {
	// HomeDir overrides the home directory used for ~ expansion in tests.
	HomeDir string
	// MaxBytes limits the file size and the new content size.
	MaxBytes int64
	// State holds the backup service. When nil, a state root is derived from
	// HomeDir (or the user home directory).
	State *state.Root
	// Runner executes privileged (sudo) and remote (SSH) file helper scripts.
	Runner scriptexec.Runner
}

// EditRequest describes a targeted file edit. OldContent is an exact snippet that
// must occur once. For a new file, OldContent is empty and NewContent is the
// complete file body.
type EditRequest struct {
	Target     string
	Path       string
	OldContent string
	NewContent string
	Reason     string
	UseSudo    bool
}

// Editor prepares and applies file changes.
type Editor struct {
	opts  EditorOptions
	state *state.Root
}

// NewEditor creates a file editor.
func NewEditor(opts EditorOptions) *Editor {
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = defaultMaxBytes
	}
	editor := &Editor{opts: opts, state: opts.State}
	if editor.state == nil {
		home := opts.HomeDir
		if home == "" {
			home, _ = os.UserHomeDir()
		}
		if home != "" {
			editor.state = state.NewFromPaths(state.DefaultForHome(home))
		}
	}
	return editor
}

// Prepare reads and validates the target without writing anything.
func (e *Editor) Prepare(ctx context.Context, req EditRequest) (*Change, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tgt, err := target.New(req.Target)
	if err != nil {
		return nil, fmt.Errorf("invalid target %q: %w", req.Target, err)
	}
	if strings.TrimSpace(req.NewContent) == "" {
		return nil, errors.New("new_content is empty")
	}
	if privateKeyMarker.MatchString(req.NewContent) {
		return nil, ErrPrivateKey
	}
	if int64(len(req.NewContent)) > e.opts.MaxBytes {
		return nil, fmt.Errorf("%w: new_content exceeds %d bytes", ErrTooLarge, e.opts.MaxBytes)
	}

	if !tgt.IsLocal() {
		remotePath, pathErr := normalizeRemotePath(req.Path)
		if pathErr != nil {
			return nil, pathErr
		}
		inspected, needSudo, inspectErr := e.inspectViaRunner(ctx, tgt, remotePath, req.UseSudo)
		if inspectErr != nil {
			return nil, inspectErr
		}
		return e.buildChange(tgt, inspected, req, needSudo)
	}

	localPath, err := normalizeLocalPath(req.Path, e.opts.HomeDir)
	if err != nil {
		return nil, err
	}

	info, err := os.Lstat(localPath)
	if err != nil {
		if os.IsPermission(err) {
			return e.prepareLocalPermission(ctx, tgt, localPath, req, err)
		}
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to inspect %s: %w", localPath, err)
		}
		return e.prepareLocalNewFile(ctx, tgt, localPath, req)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%w: %s is a symlink", ErrUnsafePath, localPath)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s is not a regular file", ErrUnsafePath, localPath)
	}
	if info.Size() > e.opts.MaxBytes {
		return nil, fmt.Errorf("%w: %d bytes exceeds %d", ErrTooLarge, info.Size(), e.opts.MaxBytes)
	}

	original, err := os.ReadFile(localPath)
	if err != nil {
		if os.IsPermission(err) {
			return e.prepareLocalPermission(ctx, tgt, localPath, req, err)
		}
		return nil, fmt.Errorf("failed to read %s: %w", localPath, err)
	}

	needSudo := req.UseSudo || !canReplaceLocalFile(localPath, info)
	return e.buildChange(tgt, inspectedFile{
		Path:    localPath,
		Exists:  true,
		Perm:    info.Mode().Perm(),
		Content: original,
	}, req, needSudo)
}

func (e *Editor) prepareLocalPermission(ctx context.Context, tgt target.Target, localPath string, req EditRequest, cause error) (*Change, error) {
	if !req.UseSudo {
		return nil, fmt.Errorf("permission denied accessing %s; retry file_edit with sudo=true: %w", localPath, cause)
	}
	inspected, needSudo, err := e.inspectViaRunner(ctx, tgt, localPath, true)
	if err != nil {
		return nil, err
	}
	return e.buildChange(tgt, inspected, req, needSudo)
}

func (e *Editor) prepareLocalNewFile(ctx context.Context, tgt target.Target, localPath string, req EditRequest) (*Change, error) {
	parent := filepath.Dir(localPath)
	parentInfo, err := os.Stat(parent)
	if err != nil {
		if os.IsPermission(err) {
			return e.prepareLocalPermission(ctx, tgt, localPath, req, err)
		}
		return nil, fmt.Errorf("parent directory %s is not accessible: %w", parent, err)
	}
	if !parentInfo.IsDir() {
		return nil, fmt.Errorf("%w: parent %s is not a directory", ErrUnsafePath, parent)
	}
	needSudo := req.UseSudo || !canWriteLocalDir(parent)
	return e.buildChange(tgt, inspectedFile{
		Path:   localPath,
		Exists: false,
		Perm:   defaultFileMode,
	}, req, needSudo)
}

func (e *Editor) buildChange(tgt target.Target, inspected inspectedFile, req EditRequest, useSudo bool) (*Change, error) {
	diffHeaderPath := inspected.Path
	if !tgt.IsLocal() {
		diffHeaderPath = fmt.Sprintf("%s:%s", tgt.String(), inspected.Path)
	}

	if !inspected.Exists {
		if strings.TrimSpace(req.OldContent) != "" {
			return nil, fmt.Errorf("old_content must be empty when creating a new file")
		}
		updated := []byte(req.NewContent)
		return &Change{
			Target:  tgt,
			Path:    inspected.Path,
			Reason:  strings.TrimSpace(req.Reason),
			Diff:    renderDiff(diffHeaderPath, nil, updated, "", req.NewContent, 0),
			Updated: updated,
			Perm:    defaultFileMode,
			Exists:  false,
			UseSudo: useSudo,
		}, nil
	}

	if strings.TrimSpace(req.OldContent) == "" {
		return nil, ErrMissingOldContent
	}
	original := inspected.Content
	if int64(len(original)) > e.opts.MaxBytes {
		return nil, fmt.Errorf("%w: %d bytes exceeds %d", ErrTooLarge, len(original), e.opts.MaxBytes)
	}
	if privateKeyMarker.Match(original) {
		return nil, ErrPrivateKey
	}

	count := strings.Count(string(original), req.OldContent)
	switch {
	case count == 0:
		return nil, ErrNoMatch
	case count > 1:
		return nil, fmt.Errorf("%w (%d matches); include more surrounding context", ErrAmbiguousMatch, count)
	}

	index := strings.Index(string(original), req.OldContent)
	updated := make([]byte, 0, len(original)-len(req.OldContent)+len(req.NewContent))
	updated = append(updated, original[:index]...)
	updated = append(updated, req.NewContent...)
	updated = append(updated, original[index+len(req.OldContent):]...)
	if string(updated) == string(original) {
		return nil, ErrIdenticalContent
	}

	perm := inspected.Perm
	if perm == 0 {
		perm = defaultFileMode
	}
	return &Change{
		Target:       tgt,
		Path:         inspected.Path,
		Reason:       strings.TrimSpace(req.Reason),
		Diff:         renderDiff(diffHeaderPath, original, updated, req.OldContent, req.NewContent, index),
		Original:     original,
		OriginalHash: util.SHA256Hex(original),
		Updated:      updated,
		Perm:         perm,
		Exists:       true,
		UseSudo:      useSudo,
	}, nil
}

// Apply writes the prepared change with a backup and atomic replacement.
func (e *Editor) Apply(ctx context.Context, change *Change) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if change == nil {
		return errors.New("file change is nil")
	}

	key := stateKey(change.Target, change.Path)
	localDirect := change.Target.IsLocal() && !change.UseSudo
	if localDirect {
		if err := e.ensureUnchanged(change); err != nil {
			return err
		}
	}

	if change.Exists && e.state != nil {
		if err := e.state.Backups.Backup("fileedit", key, change.Original, change.Perm); err != nil {
			return err
		}
	}

	if localDirect {
		return storage.WriteFile(change.Path, change.Updated, storage.WriteOptions{
			FileMode: change.Perm,
			DirMode:  defaultDirMode,
		})
	}
	return e.applyViaRunner(ctx, change)
}

func (e *Editor) ensureUnchanged(change *Change) error {
	if change.Exists {
		current, err := os.ReadFile(change.Path)
		if err != nil {
			return fmt.Errorf("failed to re-read %s: %w", change.Path, err)
		}
		if util.SHA256Hex(current) != change.OriginalHash {
			return errors.New("file changed after the diff was prepared; no changes were written")
		}
		return nil
	}
	if _, err := os.Lstat(change.Path); err == nil {
		return errors.New("file already exists; no changes were written")
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to inspect %s: %w", change.Path, err)
	}
	return nil
}
