package executor

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestNewLocalWithOptionsDefaults(t *testing.T) {
	runner := NewLocal()
	if runner.opts.Timeout != DefaultLocalTimeout {
		t.Fatalf("timeout = %s, want %s", runner.opts.Timeout, DefaultLocalTimeout)
	}
	if runner.opts.ExitGrace != DefaultExitGrace {
		t.Fatalf("exit grace = %s, want %s", runner.opts.ExitGrace, DefaultExitGrace)
	}

	disabled := NewLocalWithOptions(LocalOptions{Timeout: -1, ExitGrace: -1})
	if disabled.opts.Timeout != -1 || disabled.opts.ExitGrace != -1 {
		t.Fatalf("disabled options = %+v", disabled.opts)
	}
}

func TestLocalRunnerInjectsNonInteractiveEnv(t *testing.T) {
	runner := NewLocal()
	result, err := runner.Execute(context.Background(), CommandRequest{
		Target:  mustTarget(t, "local"),
		Command: `printf '%s %s %s' "$PAGER" "$GIT_PAGER" "$EDITOR"`,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := strings.TrimSpace(result.Output); got != "cat cat false" {
		t.Fatalf("env = %q, want \"cat cat false\"", got)
	}
}

func TestLocalRunnerNonZeroExitCodeIsNotExecutionError(t *testing.T) {
	runner := NewLocal()
	result, err := runner.Execute(context.Background(), CommandRequest{
		Target:  mustTarget(t, "local"),
		Command: `printf 'miss\n'; exit 1`,
	})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if result.Output != "miss\n" {
		t.Fatalf("output = %q, want %q", result.Output, "miss\n")
	}
}

func TestLocalHardTimeout(t *testing.T) {
	runner := NewLocalWithOptions(LocalOptions{Timeout: 100 * time.Millisecond})
	start := time.Now()
	_, err := runner.runCaptured(context.Background(), "sleep 10")
	if err == nil || !errors.Is(err, ErrCommandTimeout) {
		t.Fatalf("err = %v, want ErrCommandTimeout", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("timeout took too long: %s", elapsed)
	}
}

func TestLocalContextCancel(t *testing.T) {
	runner := NewLocal()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := runner.runCaptured(ctx, "sleep 10")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}
}

func TestLocalErrorMappingExitCodes(t *testing.T) {
	runner := NewLocal()

	result := CommandResult{Target: "local"}
	err := runner.resultError(&result, ErrCommandTimeout)
	if result.ExitCode != exitCodeTimeout {
		t.Fatalf("timeout exit code = %d, want %d", result.ExitCode, exitCodeTimeout)
	}
	var timeoutErr *TimeoutError
	if !errors.As(err, &timeoutErr) || !errors.Is(err, ErrCommandTimeout) || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("timeout error = %v", err)
	}

	result = CommandResult{Target: "local"}
	err = runner.resultError(&result, ErrStuckExit)
	if result.ExitCode != exitCodeKilled {
		t.Fatalf("stuck exit code = %d, want %d", result.ExitCode, exitCodeKilled)
	}
	var stuckErr *StuckExitError
	if !errors.As(err, &stuckErr) || !errors.Is(err, ErrStuckExit) || !strings.Contains(err.Error(), "did not exit") {
		t.Fatalf("stuck error = %v", err)
	}
}

func TestWaitInteractiveExitGrace(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 10")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	outputDone := make(chan struct{})
	close(outputDone)
	err := waitInteractive(context.Background(), cmd, nil, startWait(cmd), 0, 50*time.Millisecond, outputDone)
	if !errors.Is(err, ErrStuckExit) {
		t.Fatalf("err = %v, want ErrStuckExit", err)
	}
	if cmd.ProcessState == nil {
		t.Fatal("command was not reaped after exit grace")
	}
}

func TestLocalRunCapturedDoesNotHangOnBackgroundChildPipe(t *testing.T) {
	runner := NewLocalWithOptions(LocalOptions{Timeout: 5 * time.Second})
	start := time.Now()
	out, err := runner.runCaptured(context.Background(), "(sleep 2 >/dev/null 2>&1 &); (sleep 2 &); echo done")
	if err != nil {
		t.Fatalf("runCaptured: %v", err)
	}
	if strings.TrimSpace(out) != "done" {
		t.Fatalf("out = %q, want \"done\"", out)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("runCaptured waited for background child: %s", elapsed)
	}
}
