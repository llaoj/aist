package executor

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

const (
	// killGrace is how long a terminated command gets before SIGKILL.
	killGrace = 2 * time.Second
	// maxWaitAfterKill bounds how long we wait for Wait after SIGKILL. The
	// process may be stuck in the kernel (for example a macOS PTY session
	// leader in the "trying to exit" state), in which case we must give up so
	// the caller can still restore the terminal.
	maxWaitAfterKill = 2 * time.Second
	// outputDrainWait is how long we wait for the output copier to finish after
	// the command exits so captured output is not truncated.
	outputDrainWait = 500 * time.Millisecond
)

// startWait reaps cmd in the background so the caller can enforce deadlines.
func startWait(cmd *exec.Cmd) <-chan error {
	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()
	return waitCh
}

// signalProcess sends sig to the command's process group. Commands are started
// with a new session, so the group id equals the process id.
func signalProcess(cmd *exec.Cmd, sig syscall.Signal) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pid := cmd.Process.Pid
	if pid <= 0 {
		return
	}
	if err := syscall.Kill(-pid, sig); err != nil {
		_ = cmd.Process.Signal(sig)
	}
}

// terminateProcess escalates SIGTERM to SIGKILL and waits a bounded time.
func terminateProcess(cmd *exec.Cmd, waitCh <-chan error) {
	terminatePTYProcess(cmd, nil, waitCh)
}

// terminatePTYProcess escalates SIGTERM to SIGKILL and closes ptmx before
// SIGKILL so a macOS PTY session leader stuck in kernel ttywait can exit.
func terminatePTYProcess(cmd *exec.Cmd, ptmx *os.File, waitCh <-chan error) {
	signalProcess(cmd, syscall.SIGTERM)
	select {
	case <-waitCh:
		return
	case <-time.After(killGrace):
	}
	if ptmx != nil {
		_ = ptmx.Close()
	}
	signalProcess(cmd, syscall.SIGKILL)
	select {
	case <-waitCh:
	case <-time.After(maxWaitAfterKill):
	}
}

func ignoreWaitDelay(err error) error {
	if errors.Is(err, exec.ErrWaitDelay) {
		return nil
	}
	return err
}

// waitCapture waits for a piped (non-PTY) command and enforces ctx and timeout.
func waitCapture(ctx context.Context, cmd *exec.Cmd, waitCh <-chan error, timeout time.Duration) error {
	var timeoutCh <-chan time.Time
	if timeout > 0 {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		timeoutCh = timer.C
	}
	var ctxDone <-chan struct{}
	if ctx != nil {
		ctxDone = ctx.Done()
	}

	select {
	case err := <-waitCh:
		return err
	case <-ctxDone:
		terminateProcess(cmd, waitCh)
		return ctx.Err()
	case <-timeoutCh:
		terminateProcess(cmd, waitCh)
		return ErrCommandTimeout
	}
}
