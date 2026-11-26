package executor

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

func TestRunInteractiveTTYLargeBurstWithSharedStdio(t *testing.T) {
	outerMaster, outerSlave, err := pty.Open()
	if err != nil {
		t.Skipf("pty unavailable: %v", err)
	}
	defer outerMaster.Close()

	_ = pty.Setsize(outerMaster, &pty.Winsize{Rows: 24, Cols: 80})

	stdinFd := int(outerSlave.Fd())
	stdoutFd, err := syscall.Dup(stdinFd)
	if err != nil {
		outerSlave.Close()
		t.Fatalf("Dup: %v", err)
	}
	stdoutFile := os.NewFile(uintptr(stdoutFd), "/dev/stdout")

	var displayed bytes.Buffer
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		buf := make([]byte, 256)
		for {
			// Drain slowly so the 1KB kernel TTY output queue fills repeatedly.
			time.Sleep(time.Millisecond)
			n, readErr := outerMaster.Read(buf)
			if n > 0 {
				displayed.Write(buf[:n])
			}
			if readErr != nil {
				return
			}
		}
	}()

	cmd := exec.Command("sh", "-c", `seq 1 2500; echo "=== PIPELINE ==="; yes sample_line | head -n 400; echo "=== END ==="`)
	cmd.Env = withNonInteractiveEnv(os.Environ())

	output, runErr := runInteractiveTTY(context.Background(), stdinFd, cmd, stdoutFile, 15*time.Second, 300*time.Millisecond, 0)

	flags, flagErr := unix.FcntlInt(uintptr(stdoutFd), unix.F_GETFL, 0)
	_ = stdoutFile.Close()
	_ = outerSlave.Close()
	<-readerDone

	if runErr != nil {
		t.Fatalf("runInteractiveTTY failed: %v", runErr)
	}
	if flagErr != nil {
		t.Fatalf("FcntlInt: %v", flagErr)
	}
	if flags&unix.O_NONBLOCK != 0 {
		t.Fatalf("stdout fd left with O_NONBLOCK: %#x", flags)
	}
	for _, want := range []string{"1\r\n", "2500\r\n", "=== PIPELINE ===", "sample_line", "=== END ==="} {
		if !strings.Contains(output, want) {
			t.Fatalf("captured output missing %q (len=%d)", want, len(output))
		}
		if !strings.Contains(displayed.String(), want) {
			t.Fatalf("displayed output missing %q (len=%d)", want, displayed.Len())
		}
	}
}

type failingAfterWriter struct {
	limit   int
	written int
}

func (w *failingAfterWriter) Write(p []byte) (int, error) {
	if w.written >= w.limit {
		return 0, errors.New("display broken")
	}
	rem := w.limit - w.written
	if len(p) > rem {
		w.written += rem
		return rem, errors.New("display broken")
	}
	w.written += len(p)
	return len(p), nil
}

func TestCopyPTYOutputReportsBrokenDisplay(t *testing.T) {
	outputBuf := newLockedBuffer(0)
	display := &failingAfterWriter{limit: 4}
	broken := copyPTYOutput(strings.NewReader("0123456789"), display, outputBuf)
	if !broken {
		t.Fatal("copyPTYOutput did not report broken display")
	}
	if outputBuf.String() != "0123456789" {
		t.Fatalf("captured output = %q, want all bytes", outputBuf.String())
	}
	if display.written != 4 {
		t.Fatalf("display writes = %d, want 4", display.written)
	}
}

func TestRunInteractiveTTYDrainsWhenDisplayFails(t *testing.T) {
	outerMaster, outerSlave, err := pty.Open()
	if err != nil {
		t.Skipf("pty unavailable: %v", err)
	}
	defer outerMaster.Close()
	defer outerSlave.Close()

	_ = pty.Setsize(outerMaster, &pty.Winsize{Rows: 24, Cols: 80})

	cmd := exec.Command("sh", "-c", `seq 1 4000; echo "ALL_DONE"`)
	cmd.Env = withNonInteractiveEnv(os.Environ())

	display := &failingAfterWriter{limit: 512}
	output, runErr := runInteractiveTTY(context.Background(), int(outerSlave.Fd()), cmd, display, 10*time.Second, 200*time.Millisecond, 0)
	if runErr != nil {
		t.Fatalf("runInteractiveTTY failed when display broke: %v", runErr)
	}
	if !strings.Contains(output, "ALL_DONE") {
		t.Fatalf("captured output was truncated (len=%d)", len(output))
	}
}

func TestRunInteractiveTTYForwardsInput(t *testing.T) {
	outerMaster, outerSlave, err := pty.Open()
	if err != nil {
		t.Skipf("pty unavailable: %v", err)
	}
	defer outerMaster.Close()

	_ = pty.Setsize(outerMaster, &pty.Winsize{Rows: 24, Cols: 80})

	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		buf := make([]byte, 256)
		prompted := false
		for {
			n, readErr := outerMaster.Read(buf)
			if n > 0 && !prompted && strings.Contains(string(buf[:n]), "PROMPT:") {
				prompted = true
				_, _ = outerMaster.Write([]byte("hello_world\n"))
			}
			if readErr != nil {
				return
			}
		}
	}()

	cmd := exec.Command("sh", "-c", `printf "PROMPT:"; IFS= read -r line; printf "\nGOT:%s\n" "$line"`)
	output, runErr := runInteractiveTTY(context.Background(), int(outerSlave.Fd()), cmd, outerSlave, 5*time.Second, 300*time.Millisecond, 0)
	_ = outerSlave.Close()
	<-readerDone

	if runErr != nil {
		t.Fatalf("runInteractiveTTY: %v", runErr)
	}
	if !strings.Contains(output, "GOT:hello_world") {
		t.Fatalf("output = %q, want GOT:hello_world", output)
	}
}

func TestRunInteractiveTTYSingleCtrlCForwardsToChild(t *testing.T) {
	outerMaster, outerSlave, err := pty.Open()
	if err != nil {
		t.Skipf("pty unavailable: %v", err)
	}
	defer outerMaster.Close()

	_ = pty.Setsize(outerMaster, &pty.Winsize{Rows: 24, Cols: 80})

	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		buf := make([]byte, 256)
		sent := false
		for {
			n, readErr := outerMaster.Read(buf)
			if n > 0 && !sent && strings.Contains(string(buf[:n]), "READY") {
				sent = true
				_, _ = outerMaster.Write([]byte{ctrlCByte})
			}
			if readErr != nil {
				return
			}
		}
	}()

	cmd := exec.Command("sh", "-c", `trap 'echo CAUGHT_INT; exit 0' INT; sleep 10 & pid=$!; echo READY; wait "$pid"`)
	output, runErr := runInteractiveTTY(context.Background(), int(outerSlave.Fd()), cmd, outerSlave, 5*time.Second, 300*time.Millisecond, 0)
	_ = outerSlave.Close()
	<-readerDone

	if runErr != nil {
		t.Fatalf("runInteractiveTTY: %v (output=%q)", runErr, output)
	}
	if !strings.Contains(output, "CAUGHT_INT") {
		t.Fatalf("output = %q, want CAUGHT_INT", output)
	}
}

func TestRunInteractiveTTYDoubleCtrlCForceInterruptsStuckCommand(t *testing.T) {
	outerMaster, outerSlave, err := pty.Open()
	if err != nil {
		t.Skipf("pty unavailable: %v", err)
	}
	defer outerMaster.Close()

	_ = pty.Setsize(outerMaster, &pty.Winsize{Rows: 24, Cols: 80})

	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		buf := make([]byte, 256)
		sent := false
		for {
			n, readErr := outerMaster.Read(buf)
			if n > 0 && !sent && strings.Contains(string(buf[:n]), "READY") {
				sent = true
				_, _ = outerMaster.Write([]byte{ctrlCByte})
				time.Sleep(100 * time.Millisecond)
				_, _ = outerMaster.Write([]byte{ctrlCByte})
			}
			if readErr != nil {
				return
			}
		}
	}()

	start := time.Now()
	cmd := exec.Command("sh", "-c", `trap '' INT TERM; echo READY; sleep 30`)
	_, runErr := runInteractiveTTY(context.Background(), int(outerSlave.Fd()), cmd, outerSlave, 15*time.Second, 300*time.Millisecond, 0)
	_ = outerSlave.Close()
	<-readerDone

	if !errors.Is(runErr, ErrCommandInterrupted) {
		t.Fatalf("runErr = %v, want ErrCommandInterrupted", runErr)
	}
	if elapsed := time.Since(start); elapsed > 6*time.Second {
		t.Fatalf("force interrupt took too long: %s", elapsed)
	}
}
