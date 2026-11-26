package executor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/llaoj/aist/internal/target"
)

func mustTarget(t *testing.T, raw string) target.Target {
	t.Helper()
	tgt, err := target.New(raw)
	if err != nil {
		t.Fatalf("target.New(%q): %v", raw, err)
	}
	return tgt
}

func newFakeSSH(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ssh")
	script := `#!/bin/sh
opts=""
while [ $# -gt 2 ]; do
  case "$1" in
    -o) opts="$opts -o $2"; shift 2 ;;
    -*) opts="$opts $1"; shift ;;
    *) break ;;
  esac
done
target="$1"
command="$2"
printf 'opts=%s\ntarget=%s\ncommand=%s\n' "$opts" "$target" "$command"
case "$target" in
  down)
    echo "ssh: connect to host $target port 22: Connection refused" >&2
    exit 255
    ;;
  fail)
    echo "boom" >&2
    exit 3
    ;;
  slow)
    exec sleep 5
    ;;
  *)
    echo "ok"
    ;;
esac
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestSSHExecuteCapturesOutputAndExitCode(t *testing.T) {
	runner := NewSSH(SSHOptions{Binary: newFakeSSH(t), DisablePTY: true, Timeout: 5 * time.Second})

	result, err := runner.Execute(context.Background(), CommandRequest{Target: mustTarget(t, "web1"), Command: "df -h"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Target != "web1" || result.ExitCode != 0 {
		t.Fatalf("result = %+v", result)
	}
	for _, want := range []string{"-o ConnectTimeout=10", "-o LogLevel=ERROR", "target=web1", "df -h", "PAGER=cat", "ok"} {
		if !strings.Contains(result.Output, want) {
			t.Fatalf("output = %q, want %q", result.Output, want)
		}
	}
	if result.Duration <= 0 {
		t.Fatal("duration not recorded")
	}
}

func TestSSHExecuteReportsUnreachable(t *testing.T) {
	runner := NewSSH(SSHOptions{Binary: newFakeSSH(t), DisablePTY: true, Timeout: 5 * time.Second})

	result, err := runner.Execute(context.Background(), CommandRequest{Target: mustTarget(t, "down"), Command: "true"})
	if !errors.Is(err, ErrSSHUnreachable) {
		t.Fatalf("err = %v, want ErrUnreachable", err)
	}
	if result.ExitCode != 255 {
		t.Fatalf("exit code = %d", result.ExitCode)
	}
	if !strings.Contains(err.Error(), "ssh down") || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("err = %v", err)
	}
}

func TestSSHExecuteReturnsNonZeroExitCodeWithoutError(t *testing.T) {
	runner := NewSSH(SSHOptions{Binary: newFakeSSH(t), DisablePTY: true, Timeout: 5 * time.Second})

	result, err := runner.Execute(context.Background(), CommandRequest{Target: mustTarget(t, "fail"), Command: "false"})
	if err != nil {
		t.Fatalf("Execute returned unexpected error for remote command exit code: %v", err)
	}
	if result.ExitCode != 3 {
		t.Fatalf("exit code = %d, want 3", result.ExitCode)
	}
	if !strings.Contains(result.Output, "boom") {
		t.Fatalf("result = %+v", result)
	}
}

func TestSSHExecuteTimeout(t *testing.T) {
	runner := NewSSH(SSHOptions{Binary: newFakeSSH(t), DisablePTY: true, Timeout: 100 * time.Millisecond})

	_, err := runner.Execute(context.Background(), CommandRequest{Target: mustTarget(t, "slow"), Command: "sleep"})
	var timeoutErr *TimeoutError
	if !errors.As(err, &timeoutErr) || !errors.Is(err, ErrCommandTimeout) || strings.Contains(err.Error(), "on slow") {
		t.Fatalf("err = %v, want TimeoutError", err)
	}
}

func TestSSHRejectsMissingOrLocalTarget(t *testing.T) {
	runner := NewSSH(SSHOptions{Binary: newFakeSSH(t), DisablePTY: true, Timeout: 5 * time.Second})
	for _, raw := range []string{"", "local"} {
		if _, err := runner.Execute(context.Background(), CommandRequest{Target: mustTarget(t, raw), Command: "true"}); err == nil {
			t.Fatalf("target %q accepted", raw)
		}
	}
}

type fakeRunner struct {
	req    CommandRequest
	result CommandResult
	err    error
}

func (f *fakeRunner) Execute(_ context.Context, req CommandRequest) (CommandResult, error) {
	f.req = req
	return f.result, f.err
}

func TestRouterDispatchesByTarget(t *testing.T) {
	local := &fakeRunner{result: CommandResult{Target: "local"}}
	remote := &fakeRunner{result: CommandResult{Target: "web1"}}
	router := NewCommandRouter(local, remote)

	for _, raw := range []string{"", "local", "localhost"} {
		if _, err := router.Execute(context.Background(), CommandRequest{Target: mustTarget(t, raw), Command: "pwd"}); err != nil {
			t.Fatalf("Execute(%q): %v", raw, err)
		}
		if !local.req.Target.IsLocal() {
			t.Fatalf("local target = %q", local.req.Target.String())
		}
	}

	if _, err := router.Execute(context.Background(), CommandRequest{Target: mustTarget(t, "web1"), Command: "df -h"}); err != nil {
		t.Fatalf("Execute(web1): %v", err)
	}
	if remote.req.Target.Host() != "web1" {
		t.Fatalf("remote target = %q", remote.req.Target.Host())
	}
}

func TestSSHArgsAllocateRemoteTTYWhenInteractive(t *testing.T) {
	withTTY := sshArgs(true, "web1", "sudo systemctl restart nginx")
	wantWithTTY := []string{"-t", "-o", "ConnectTimeout=10", "-o", "LogLevel=ERROR", "web1", "sudo systemctl restart nginx"}
	if strings.Join(withTTY, "\x00") != strings.Join(wantWithTTY, "\x00") {
		t.Fatalf("sshArgs(true) = %#v, want %#v", withTTY, wantWithTTY)
	}

	withoutTTY := sshArgs(false, "web1", "df -h")
	wantWithoutTTY := []string{"-o", "ConnectTimeout=10", "-o", "LogLevel=ERROR", "web1", "df -h"}
	if strings.Join(withoutTTY, "\x00") != strings.Join(wantWithoutTTY, "\x00") {
		t.Fatalf("sshArgs(false) = %#v, want %#v", withoutTTY, wantWithoutTTY)
	}
}
