package executor

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"time"
)

// SSHOptions configures the system ssh runner.
type SSHOptions struct {
	// Binary is the ssh executable. Empty uses "ssh".
	Binary string
	// Timeout bounds one remote command. Zero uses DefaultSSHTimeout.
	Timeout time.Duration
	// DisablePTY disables the PTY path. It is primarily useful for tests
	// and automation where stdin is not a terminal.
	DisablePTY bool
}

// SSHRunner runs commands through the system ssh client. It relies on the user's
// existing ssh config, ProxyJump, agent and MFA setup and never stores
// credentials.
type SSHRunner struct {
	opts SSHOptions
}

// NewSSH creates an SSH runner.
func NewSSH(opts SSHOptions) *SSHRunner {
	if opts.Binary == "" {
		opts.Binary = "ssh"
	}
	if opts.Timeout <= 0 {
		opts.Timeout = DefaultSSHTimeout
	}
	return &SSHRunner{opts: opts}
}

// Execute runs command on the target through ssh.
func (s *SSHRunner) Execute(ctx context.Context, req CommandRequest) (CommandResult, error) {
	start := time.Now()
	host := req.Target.Host()
	result := CommandResult{Target: req.Target.String(), Command: req.Command}
	if req.Target.IsLocal() {
		return result, errors.New("ssh executor requires a remote target")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}

	timeout := s.opts.Timeout
	if timeout <= 0 {
		timeout = DefaultSSHTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	command := nonInteractiveEnvPrefix() + req.Command
	usePTY := req.Display != nil && !s.opts.DisablePTY && isTerminalInput()
	var output string
	var err error
	var streamed bool
	if usePTY {
		// Allocate a remote TTY (-t) so sudo and interactive commands on the
		// remote host can prompt via /dev/tty. runCtx already carries the SSH
		// timeout, so disable a second hard timer.
		cmd := exec.CommandContext(runCtx, s.opts.Binary, sshArgs(true, host, command)...)
		output, err = runInteractive(runCtx, cmd, req.Display, 0, DefaultExitGrace, maxCapturedOutputBytes)
		streamed = true
		if errors.Is(err, errNotTerminal) {
			streamed = false
			fallbackCmd := exec.CommandContext(runCtx, s.opts.Binary, sshArgs(false, host, command)...)
			output, err = runCapturedWithStdin(fallbackCmd, os.Stdin, maxCapturedOutputBytes)
		}
	} else {
		cmd := exec.CommandContext(runCtx, s.opts.Binary, sshArgs(false, host, command)...)
		output, err = runCapturedWithStdin(cmd, os.Stdin, maxCapturedOutputBytes)
	}

	result.Output = output
	result.OutputStreamed = streamed
	result.Duration = time.Since(start)
	result.ExitCode = exitCodeOrZero(err)

	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return result, ctx.Err()
		}
		return result, &TimeoutError{Timeout: timeout}
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return result, ctx.Err()
	}
	if errors.Is(err, ErrStuckExit) {
		result.ExitCode = exitCodeKilled
		return result, &StuckExitError{Grace: DefaultExitGrace}
	}
	if err != nil {
		if result.ExitCode == 255 {
			return result, sshUnreachableError(host, output)
		}
		if !hasCommandExitCode(err) {
			return result, err
		}
	}
	return result, nil
}

// errNotTerminal lets SSH fall back to a piped command when no TTY is present.
var errNotTerminal = errors.New("stdin is not a terminal")

func sshArgs(allocateTTY bool, host, command string) []string {
	if allocateTTY {
		return []string{"-t", "-o", "LogLevel=ERROR", host, command}
	}
	return []string{"-o", "LogLevel=ERROR", host, command}
}
