package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/llaoj/aist/internal/scriptexec"
)

// ScriptRunnerOptions configures ScriptRunner.
type ScriptRunnerOptions struct {
	LocalTimeout time.Duration
	SSH          SSHOptions
	// Display receives live PTY output such as sudo prompts. Empty uses os.Stdout.
	Display io.Writer
}

// ScriptRunner executes structured POSIX sh helper scripts locally or over SSH.
type ScriptRunner struct {
	localTimeout time.Duration
	ssh          SSHOptions
	display      io.Writer
}

// NewScriptRunner creates a helper script runner for local and SSH targets.
func NewScriptRunner(opts ScriptRunnerOptions) *ScriptRunner {
	localTimeout := opts.LocalTimeout
	if localTimeout <= 0 {
		localTimeout = DefaultLocalTimeout
	}
	sshOpts := opts.SSH
	if sshOpts.Binary == "" {
		sshOpts.Binary = "ssh"
	}
	if sshOpts.Timeout <= 0 {
		sshOpts.Timeout = DefaultSSHTimeout
	}
	display := opts.Display
	if display == nil {
		display = os.Stdout
	}
	return &ScriptRunner{
		localTimeout: localTimeout,
		ssh:          sshOpts,
		display:      display,
	}
}

// RunPipe runs a script without a PTY so binary bytes and LF line endings are
// preserved on stdin and stdout.
func (r *ScriptRunner) RunPipe(ctx context.Context, req scriptexec.PipeRequest) ([]byte, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}

	timeout := r.localTimeout
	if !req.Target.IsLocal() {
		timeout = r.ssh.Timeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var cmd *exec.Cmd
	if req.Target.IsLocal() {
		argv := append([]string{"-c", req.Script, "_"}, req.Args...)
		cmd = exec.CommandContext(runCtx, "sh", argv...)
	} else {
		host := req.Target.Host()
		remoteCmd := buildRemoteShCommand(false, false, req.Script, req.Args)
		cmd = exec.CommandContext(runCtx, r.ssh.Binary, sshArgs(false, host, remoteCmd)...)
	}

	if len(req.Stdin) > 0 {
		cmd.Stdin = bytes.NewReader(req.Stdin)
	}
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	cmd.WaitDelay = outputDrainWait

	err := ignoreWaitDelay(cmd.Run())
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, stderrBuf.String(), ctx.Err()
		}
		return nil, stderrBuf.String(), &TimeoutError{Timeout: timeout}
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return nil, stderrBuf.String(), ctx.Err()
	}
	if err != nil {
		code := exitCodeOrZero(err)
		if !req.Target.IsLocal() && code == 255 {
			return nil, stderrBuf.String(), sshUnreachableError(req.Target.Host(), stderrBuf.String())
		}
		return stdoutBuf.Bytes(), stderrBuf.String(), scriptExitError(code, err)
	}
	return stdoutBuf.Bytes(), stderrBuf.String(), nil
}

// RunInteractive runs a script in an interactive PTY so sudo and SSH prompts
// interact directly with the user's terminal while hiding marked data payloads.
func (r *ScriptRunner) RunInteractive(ctx context.Context, req scriptexec.InteractiveRequest) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	timeout := r.localTimeout
	if !req.Target.IsLocal() {
		timeout = r.ssh.Timeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	usePTY := isTerminalInput() && (req.Target.IsLocal() || !r.ssh.DisablePTY)

	buildCmd := func(interactiveTTY bool) *exec.Cmd {
		if req.Target.IsLocal() {
			shArgv := append([]string{"-c", req.Script, "_"}, req.Args...)
			if !req.UseSudo {
				return exec.CommandContext(runCtx, "sh", shArgv...)
			}
			sudoArgv := []string{"sh"}
			if !interactiveTTY {
				sudoArgv = []string{"-n", "sh"}
			}
			sudoArgv = append(sudoArgv, shArgv...)
			return exec.CommandContext(runCtx, "sudo", sudoArgv...)
		}
		host := req.Target.Host()
		remoteCmd := buildRemoteShCommand(req.UseSudo, req.UseSudo && !interactiveTTY, req.Script, req.Args)
		return exec.CommandContext(runCtx, r.ssh.Binary, sshArgs(interactiveTTY, host, remoteCmd)...)
	}

	var rawOutput string
	var err error
	if usePTY {
		var displayOut io.Writer = r.display
		var filter *markerFilterWriter
		if req.BeginMarker != "" && req.EndMarker != "" {
			filter = newMarkerFilterWriter(displayOut, req.BeginMarker, req.EndMarker)
			displayOut = filter
		}
		rawOutput, err = runInteractive(runCtx, buildCmd(true), displayOut, 0, DefaultExitGrace, 0)
		if filter != nil {
			filter.Flush()
		}
		if errors.Is(err, errNotTerminal) {
			rawOutput, err = runCapturedWithStdin(buildCmd(false), nil, 0)
		}
	} else {
		rawOutput, err = runCapturedWithStdin(buildCmd(false), nil, 0)
	}

	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", ctx.Err()
		}
		return "", &TimeoutError{Timeout: timeout}
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return "", ctx.Err()
	}

	payload, sanitized := splitMarkedOutput(rawOutput, req.BeginMarker, req.EndMarker)
	if err != nil {
		code := exitCodeOrZero(err)
		if !req.Target.IsLocal() && code == 255 {
			return "", sshUnreachableError(req.Target.Host(), sanitized)
		}
		detail := strings.TrimSpace(sanitized)
		if detail != "" {
			return payload, fmt.Errorf("%w: %s", scriptExitError(code, err), detail)
		}
		return payload, scriptExitError(code, err)
	}
	if req.BeginMarker != "" {
		return payload, nil
	}
	return sanitized, nil
}

func buildRemoteShCommand(useSudo, nonInteractiveSudo bool, script string, args []string) string {
	var parts []string
	if useSudo {
		parts = append(parts, "sudo")
		if nonInteractiveSudo {
			parts = append(parts, "-n")
		}
	}
	parts = append(parts, "sh", "-c", shellQuote(script), "_")
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func scriptExitError(code int, err error) error {
	if code <= 0 {
		return err
	}
	return fmt.Errorf("helper script exited with code %d", code)
}
