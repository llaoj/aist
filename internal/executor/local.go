package executor

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
)

const (
	// DefaultLocalTimeout bounds one local command. It matches the SSH default
	// so local and remote execution behave consistently.
	DefaultLocalTimeout = 30 * time.Minute
	// DefaultExitGrace is how long a command may keep running after its output
	// ended before it is treated as stuck. This catches shells that never
	// finish exiting on macOS PTYs.
	DefaultExitGrace = 5 * time.Second
)

// Exit codes reported for commands aist had to stop itself.
const (
	// exitCodeTimeout follows the timeout(1) convention for timed out commands.
	exitCodeTimeout = 124
	// exitCodeInterrupted follows the shell convention for SIGINT (128 + 2).
	exitCodeInterrupted = 130
	// exitCodeKilled follows the shell convention for a SIGKILL (128 + 9).
	exitCodeKilled = 137
)

// LocalOptions configures local command execution.
type LocalOptions struct {
	// Timeout bounds one command. Zero uses DefaultLocalTimeout; a negative
	// value disables the hard timeout.
	Timeout time.Duration
	// ExitGrace bounds how long to wait after output ends. Zero uses
	// DefaultExitGrace; a negative value disables the exit grace.
	ExitGrace time.Duration
}

// LocalRunner runs commands in a pseudo-terminal attached to the current
// terminal. It enforces a hard timeout and an exit grace so a shell that never
// finishes exiting cannot block the caller forever.
type LocalRunner struct {
	opts LocalOptions
}

// NewLocal creates a local shell runner with default options.
func NewLocal() *LocalRunner { return NewLocalWithOptions(LocalOptions{}) }

// NewLocalWithOptions creates a local shell runner with explicit options.
func NewLocalWithOptions(opts LocalOptions) *LocalRunner {
	if opts.Timeout == 0 {
		opts.Timeout = DefaultLocalTimeout
	}
	if opts.ExitGrace == 0 {
		opts.ExitGrace = DefaultExitGrace
	}
	return &LocalRunner{opts: opts}
}

// Execute runs command with `sh -c` in a PTY and captures stdout/stderr.
func (r *LocalRunner) Execute(ctx context.Context, req CommandRequest) (CommandResult, error) {
	start := time.Now()
	result := CommandResult{Target: req.Target.String(), Command: req.Command}
	if !req.Target.IsLocal() {
		return result, errors.New("local executor cannot run commands on a remote target")
	}

	if err := ctx.Err(); err != nil {
		return result, err
	}

	var output string
	var err error
	streamed := req.Display != nil
	if streamed {
		output, err = r.runPTY(ctx, req.Command, req.Display)
		if errors.Is(err, errNotTerminal) {
			// Non-interactive callers (tests, CI, piped sessions) have no terminal.
			streamed = false
			output, err = r.runCaptured(ctx, req.Command)
		}
	} else {
		output, err = r.runCaptured(ctx, req.Command)
	}

	result.Output = output
	result.OutputStreamed = streamed
	result.Duration = time.Since(start)
	result.ExitCode = exitCodeOrZero(err)
	if err != nil {
		return result, r.resultError(&result, err)
	}
	return result, nil
}

// resultError maps low-level failures to user-facing errors and sets an
// explicit exit code when aist stopped the command itself.
func (r *LocalRunner) resultError(result *CommandResult, err error) error {
	switch {
	case errors.Is(err, ErrCommandTimeout):
		result.ExitCode = exitCodeTimeout
		return &TimeoutError{Timeout: r.opts.Timeout}
	case errors.Is(err, ErrCommandInterrupted):
		result.ExitCode = exitCodeInterrupted
		return ErrCommandInterrupted
	case errors.Is(err, ErrStuckExit):
		result.ExitCode = exitCodeKilled
		return &StuckExitError{Grace: r.opts.ExitGrace}
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case hasCommandExitCode(err):
		return nil
	default:
		return err
	}
}

func (r *LocalRunner) runPTY(ctx context.Context, command string, display io.Writer) (string, error) {
	cmd := exec.Command("sh", "-c", command)
	cmd.Env = withNonInteractiveEnv(os.Environ())
	return runInteractive(ctx, cmd, display, r.opts.Timeout, r.opts.ExitGrace, maxCapturedOutputBytes)
}

// runCaptured runs command without a PTY. It is only used when stdin is not a
// terminal, so no interactive prompt can be served anyway.
func (r *LocalRunner) runCaptured(ctx context.Context, command string) (string, error) {
	cmd := exec.Command("sh", "-c", command)
	cmd.Env = withNonInteractiveEnv(os.Environ())
	cmd.Stdin = nil
	out := newLockedBuffer(maxCapturedOutputBytes)
	cmd.Stdout = out
	cmd.Stderr = out
	cmd.WaitDelay = outputDrainWait
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	err := waitCapture(ctx, cmd, startWait(cmd), r.opts.Timeout)
	return out.String(), ignoreWaitDelay(err)
}
