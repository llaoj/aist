package fileedit

import (
	"errors"
	"os"

	"github.com/llaoj/aist/internal/target"
)

const (
	defaultMaxBytes = 1 << 20 // 1 MiB
	defaultFileMode = 0o600
	defaultDirMode  = 0o700
)

var (
	// ErrUnsafePath means the path is a directory, symlink, special file or
	// otherwise not safe to edit.
	ErrUnsafePath = errors.New("unsafe file path")
	// ErrPrivateKey means the target looks like a private key.
	ErrPrivateKey = errors.New("refusing to read or edit private key")
	// ErrMissingOldContent means an existing file was addressed without
	// old_content.
	ErrMissingOldContent = errors.New("old_content is required for an existing file")
	// ErrNoMatch means old_content was not found in the file.
	ErrNoMatch = errors.New("old_content was not found in the file")
	// ErrAmbiguousMatch means old_content occurs more than once.
	ErrAmbiguousMatch = errors.New("old_content matches more than one location")
	// ErrIdenticalContent means the edit would not change the file.
	ErrIdenticalContent = errors.New("new_content is identical to existing content")
	// ErrTooLarge means the file or edit exceeds the configured limit.
	ErrTooLarge = errors.New("file is too large to edit")
)

// Change is a prepared, not-yet-written edit.
type Change struct {
	Target   target.Target
	Path     string
	Reason   string
	Diff     string
	Original []byte
	// OriginalHash is the SHA-256 compare-and-swap token for Original, shared
	// by local, sudo and remote writes.
	OriginalHash string
	Updated      []byte
	Perm         os.FileMode
	Exists       bool
	UseSudo      bool
}

// LocationLabel renders the target, path and privilege mode for human display.
func (c *Change) LocationLabel() string {
	if c == nil {
		return ""
	}
	return c.Target.RenderPath(c.Path, c.UseSudo)
}
