// Package scriptexec defines the contract for running helper scripts locally,
// remotely or through sudo. It is shared by the command executor and fileedit.
package scriptexec

import (
	"context"

	"github.com/llaoj/aist/internal/target"
)

// PipeRequest describes a non-PTY script invocation with raw byte I/O.
type PipeRequest struct {
	Target target.Target
	Script string
	Args   []string
	Stdin  []byte
}

// InteractiveRequest describes a PTY-backed script invocation. Output between
// BeginMarker and EndMarker is captured without echoing to the terminal.
type InteractiveRequest struct {
	Target      target.Target
	UseSudo     bool
	Script      string
	Args        []string
	BeginMarker string
	EndMarker   string
}

// Runner executes helper scripts for local, remote SSH and sudo file operations.
type Runner interface {
	RunPipe(ctx context.Context, req PipeRequest) (stdout []byte, stderr string, err error)
	RunInteractive(ctx context.Context, req InteractiveRequest) (output string, err error)
}
