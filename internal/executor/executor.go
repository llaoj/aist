// Package executor runs shell commands locally or on SSH targets.
//
// It is intentionally independent from the LLM, tool parsing and UI layers so
// that command execution can be tested or replaced without changing the
// conversation orchestration.
package executor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/llaoj/aist/internal/target"
)

// DefaultSSHTimeout bounds one remote command. Long-running maintenance commands
// can still pass an explicit timeout through SSHOptions.
const DefaultSSHTimeout = 30 * time.Minute

// CommandRequest describes one command execution.
type CommandRequest struct {
	// Target is the validated execution host.
	Target target.Target
	// Command is the raw shell command to execute.
	Command string
	// Display receives live PTY output. When nil, output is captured only.
	Display io.Writer
}

// CommandResult captures the outcome of a command execution.
type CommandResult struct {
	Target         string
	Command        string
	Output         string
	OutputStreamed bool
	ExitCode       int
	Duration       time.Duration
}

// CommandRunner executes one shell request and returns its combined output.
type CommandRunner interface {
	Execute(ctx context.Context, req CommandRequest) (CommandResult, error)
}

// ErrSSHUnreachable means SSH could not connect to the target.
var ErrSSHUnreachable = errors.New("ssh target unreachable")

// ErrCommandTimeout means a command exceeded its execution timeout.
var ErrCommandTimeout = errors.New("command timed out")

// ErrStuckExit means a command finished output but did not exit within grace time.
var ErrStuckExit = errors.New("command did not exit after its output ended")

// TimeoutError reports that a command exceeded its configured timeout.
type TimeoutError struct {
	Timeout time.Duration
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("command timed out after %s", e.Timeout)
}

func (e *TimeoutError) Unwrap() error {
	return ErrCommandTimeout
}

// StuckExitError reports that a command hung after finishing output.
type StuckExitError struct {
	Grace time.Duration
}

func (e *StuckExitError) Error() string {
	return fmt.Sprintf("command did not exit within %s after its output ended", e.Grace)
}

func (e *StuckExitError) Unwrap() error {
	return ErrStuckExit
}

// SSHUnreachableError reports that SSH could not connect to or stay connected with Host.
type SSHUnreachableError struct {
	Host   string
	Detail string
}

func (e *SSHUnreachableError) Error() string {
	msg := fmt.Sprintf("%s: %s\nSSH access is not available. Configure SSH access yourself and verify with `ssh %s`.",
		ErrSSHUnreachable, e.Host, e.Host)
	if e.Detail != "" {
		msg += "\n" + e.Detail
	}
	return msg
}

func (e *SSHUnreachableError) Unwrap() error {
	return ErrSSHUnreachable
}

// CommandRouter sends local requests to the local runner and everything else to SSH.
type CommandRouter struct {
	local  CommandRunner
	remote CommandRunner
}

// NewCommandRouter creates a runner that dispatches by target.
func NewCommandRouter(local, remote CommandRunner) *CommandRouter {
	return &CommandRouter{local: local, remote: remote}
}

// Execute dispatches the request to the local or remote runner.
func (r *CommandRouter) Execute(ctx context.Context, req CommandRequest) (CommandResult, error) {
	if req.Target.IsLocal() {
		if r.local == nil {
			return CommandResult{Target: req.Target.String(), Command: req.Command}, errors.New("local executor is unavailable")
		}
		return r.local.Execute(ctx, req)
	}
	if r.remote == nil {
		return CommandResult{Target: req.Target.String(), Command: req.Command}, errors.New("ssh executor is unavailable")
	}
	return r.remote.Execute(ctx, req)
}

func exitCodeOrZero(err error) int {
	var exitErr interface{ ExitCode() int }
	if errors.As(err, &exitErr) {
		if code := exitErr.ExitCode(); code >= 0 {
			return code
		}
	}
	return 0
}

func hasCommandExitCode(err error) bool {
	if err == nil {
		return true
	}
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && exitErr.ExitCode() >= 0
}

func sshUnreachableError(host, detail string) error {
	return &SSHUnreachableError{Host: host, Detail: strings.TrimSpace(detail)}
}

func runCapturedWithStdin(cmd *exec.Cmd, stdin io.Reader, captureLimit int) (string, error) {
	buf := newLockedBuffer(captureLimit)
	cmd.Stdin = stdin
	cmd.Stdout = buf
	cmd.Stderr = buf
	cmd.WaitDelay = outputDrainWait
	err := cmd.Run()
	return buf.String(), ignoreWaitDelay(err)
}
