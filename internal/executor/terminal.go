package executor

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/llaoj/aist/internal/termio"
	"golang.org/x/term"
)

const (
	ctrlCByte            = 0x03
	forceInterruptWindow = time.Second
)

func isTerminalInput() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// runInteractive starts cmd in a PTY attached to the current terminal. It
// forwards input, terminal size and Ctrl+C so SSH prompts (password, MFA, sudo)
// and interactive commands behave like a normal shell.
// captureLimit bounds the returned output (see lockedBuffer); zero keeps all.
func runInteractive(ctx context.Context, cmd *exec.Cmd, display io.Writer, timeout, exitGrace time.Duration, captureLimit int) (string, error) {
	return runInteractiveTTY(ctx, int(os.Stdin.Fd()), cmd, display, timeout, exitGrace, captureLimit)
}

func runInteractiveTTY(ctx context.Context, fd int, cmd *exec.Cmd, display io.Writer, timeout, exitGrace time.Duration, captureLimit int) (string, error) {
	if !term.IsTerminal(fd) {
		return "", errNotTerminal
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return "", errors.New("failed to make terminal raw: " + err.Error())
	}
	if oldState == nil {
		return "", errors.New("failed to get original terminal state")
	}
	defer func() { _ = term.Restore(fd, oldState) }()

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return "", err
	}
	defer ptmx.Close()

	if width, height, sizeErr := term.GetSize(fd); sizeErr == nil && width > 0 && height > 0 {
		_ = pty.Setsize(ptmx, &pty.Winsize{Rows: uint16(height), Cols: uint16(width)})
	}

	done := make(chan struct{})

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGWINCH)
	defer signal.Stop(sigChan)
	signalDone := make(chan struct{})
	go func() {
		defer close(signalDone)
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case sig := <-sigChan:
				switch sig {
				case syscall.SIGINT:
					signalProcess(cmd, syscall.SIGINT)
				case syscall.SIGWINCH:
					if w, h, sizeErr := term.GetSize(fd); sizeErr == nil && w > 0 && h > 0 {
						_ = pty.Setsize(ptmx, &pty.Winsize{Rows: uint16(h), Cols: uint16(w)})
					}
				}
			}
		}
	}()

	outputBuf := newLockedBuffer(captureLimit)
	var displayBroken bool
	var wg sync.WaitGroup
	outputDone := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(outputDone)
		displayBroken = copyPTYOutput(ptmx, display, outputBuf)
	}()

	forceInterrupt := make(chan struct{})
	var forceInterruptOnce sync.Once
	triggerForceInterrupt := func() {
		forceInterruptOnce.Do(func() { close(forceInterrupt) })
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		copyTerminalInput(fd, ptmx, done, triggerForceInterrupt)
	}()

	err = waitInteractive(ctx, cmd, ptmx, startWait(cmd), timeout, exitGrace, outputDone, forceInterrupt)

	// Let the output copier drain, then release the PTY so any blocked read
	// returns. Waits are bounded so a stuck terminal write cannot hang us.
	outputDrained := true
	select {
	case <-outputDone:
	case <-time.After(outputDrainWait):
		outputDrained = false
	}
	close(done)
	_ = ptmx.Close()
	waitGroup(&wg, outputDrainWait)
	<-signalDone
	termio.DrainInput(fd)

	output := outputBuf.String()
	if outputDrained && display != nil && !displayBroken && strings.TrimSpace(output) != "" && !strings.HasSuffix(output, "\n") {
		_ = termio.WriteAll(display, []byte("\n"))
	}
	return output, err
}

// copyPTYOutput drains ptmx until EOF/EIO regardless of display write errors so
// child processes never block on a full PTY buffer and outputDone never closes
// prematurely. It reports whether display broke so callers can avoid retrying it.
func copyPTYOutput(ptmx io.Reader, display io.Writer, outputBuf *lockedBuffer) (displayBroken bool) {
	buf := make([]byte, 32*1024)
	displayBroken = display == nil
	for {
		n, readErr := ptmx.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			_, _ = outputBuf.Write(chunk)
			if !displayBroken {
				if writeErr := termio.WriteAll(display, chunk); writeErr != nil {
					displayBroken = true
				}
			}
		}
		if readErr != nil {
			if errors.Is(readErr, syscall.EINTR) {
				continue
			}
			return
		}
	}
}

// waitInteractive waits for the command while enforcing the hard timeout, the
// context deadline, double Ctrl+C force-interrupt, and the post-output exit grace.
func waitInteractive(ctx context.Context, cmd *exec.Cmd, ptmx *os.File, waitCh <-chan error, timeout, exitGrace time.Duration, outputDone, forceInterrupt <-chan struct{}) error {
	var ctxDone <-chan struct{}
	if ctx != nil {
		ctxDone = ctx.Done()
	}

	var timeoutCh <-chan time.Time
	if timeout > 0 {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		timeoutCh = timer.C
	}

	var graceTimer *time.Timer
	var graceCh <-chan time.Time
	defer func() {
		if graceTimer != nil {
			graceTimer.Stop()
		}
	}()

	pendingOutput := outputDone
	for {
		select {
		case err := <-waitCh:
			return err
		case <-ctxDone:
			terminatePTYProcess(cmd, ptmx, waitCh)
			return ctx.Err()
		case <-forceInterrupt:
			terminatePTYProcess(cmd, ptmx, waitCh)
			return ErrCommandInterrupted
		case <-timeoutCh:
			terminatePTYProcess(cmd, ptmx, waitCh)
			return ErrCommandTimeout
		case <-pendingOutput:
			pendingOutput = nil
			if exitGrace > 0 {
				graceTimer = time.NewTimer(exitGrace)
				graceCh = graceTimer.C
			}
		case <-graceCh:
			terminatePTYProcess(cmd, ptmx, waitCh)
			return ErrStuckExit
		}
	}
}

func waitGroup(wg *sync.WaitGroup, timeout time.Duration) {
	waited := make(chan struct{})
	go func() {
		wg.Wait()
		close(waited)
	}()
	select {
	case <-waited:
	case <-time.After(timeout):
	}
}

func copyTerminalInput(fd int, ptmx *os.File, done <-chan struct{}, onForceInterrupt func()) {
	reader, err := termio.NewReader(fd, done)
	if err != nil {
		return
	}
	defer reader.Close()

	var lastCtrlC time.Time
	buf := make([]byte, 128)
	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			if onForceInterrupt != nil {
				for _, b := range chunk {
					if b != ctrlCByte {
						continue
					}
					now := time.Now()
					if !lastCtrlC.IsZero() && now.Sub(lastCtrlC) <= forceInterruptWindow {
						onForceInterrupt()
						return
					}
					lastCtrlC = now
				}
			}
			if writeErr := termio.WriteAll(ptmx, chunk); writeErr != nil {
				return
			}
		}
		if readErr != nil {
			return
		}
	}
}
