package ui

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
)

type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestStartSpinnerRendersHintOnLineBelowAndClearsOnStop(t *testing.T) {
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Fatalf("pty.Open: %v", err)
	}
	defer ptmx.Close()
	defer tty.Close()

	if err := pty.Setsize(ptmx, &pty.Winsize{Rows: 24, Cols: 80}); err != nil {
		t.Fatalf("pty.Setsize: %v", err)
	}

	var out safeBuffer
	copyDone := make(chan struct{})
	go func() {
		defer close(copyDone)
		_, _ = io.Copy(&out, ptmx)
	}()

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	hint := "Esc 取消请求 · Ctrl+C 退出"
	stop := startSpinnerTTY(tty, tty, "思考中", hint, cancel)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(out.String(), "思考中(0s)\r\n") && strings.Contains(out.String(), hint) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	rendered := out.String()
	if !strings.Contains(rendered, "- 思考中(0s)\r\n") {
		t.Fatalf("expected status line followed by CRLF, got %q", rendered)
	}
	if !strings.Contains(rendered, hint) {
		t.Fatalf("expected hint on line below status, got %q", rendered)
	}

	if err := stop(); err != nil {
		t.Fatalf("stop() = %v, want nil", err)
	}

	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(out.String(), "\033[2A\r\033[J\033[?25h") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(out.String(), "\033[2A\r\033[J\033[?25h") {
		t.Fatalf("expected 2-row clear and cursor restore on stop, got %q", out.String())
	}

	_ = tty.Close()
	_ = ptmx.Close()
	<-copyDone
}

func TestStartSpinnerEscCancelsWithoutInterruptError(t *testing.T) {
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Fatalf("pty.Open: %v", err)
	}
	defer ptmx.Close()
	defer tty.Close()

	go func() { _, _ = io.Copy(io.Discard, ptmx) }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stop := startSpinnerTTY(tty, tty, "思考中", "Esc 取消请求 · Ctrl+C 退出", cancel)
	defer func() { _ = stop() }()

	time.Sleep(15 * time.Millisecond)

	// Arrow keys and OSC replies must not trigger a false Esc cancel.
	if _, err := ptmx.Write([]byte("\x1b[A\x1b]11;rgb:0000/0000/0000\x1b\\")); err != nil {
		t.Fatalf("write non-esc sequences: %v", err)
	}
	select {
	case <-ctx.Done():
		t.Fatal("escape sequence unexpectedly canceled spinner context")
	case <-time.After(60 * time.Millisecond):
	}

	if _, err := ptmx.Write([]byte{0x1b}); err != nil {
		t.Fatalf("write esc: %v", err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("standalone Esc did not cancel spinner context")
	}

	if err := stop(); err != nil {
		t.Fatalf("stop() after Esc = %v, want nil", err)
	}
}

func TestStartSpinnerCtrlCCancelsAndReturnsErrInterrupted(t *testing.T) {
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Fatalf("pty.Open: %v", err)
	}
	defer ptmx.Close()
	defer tty.Close()

	go func() { _, _ = io.Copy(io.Discard, ptmx) }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stop := startSpinnerTTY(tty, tty, "思考中", "Esc 取消请求 · Ctrl+C 退出", cancel)

	time.Sleep(15 * time.Millisecond)
	if _, err := ptmx.Write([]byte{0x03}); err != nil {
		t.Fatalf("write ctrl+c: %v", err)
	}

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("Ctrl+C did not cancel spinner context")
	}

	if err := stop(); !errors.Is(err, ErrInterrupted) {
		t.Fatalf("stop() after Ctrl+C = %v, want ErrInterrupted", err)
	}
	if err := stop(); !errors.Is(err, ErrInterrupted) {
		t.Fatalf("idempotent stop() after Ctrl+C = %v, want ErrInterrupted", err)
	}
}

func TestStartSpinnerEscImmediatelyFollowedByCtrlCReturnsErrInterrupted(t *testing.T) {
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Fatalf("pty.Open: %v", err)
	}
	defer ptmx.Close()
	defer tty.Close()

	go func() { _, _ = io.Copy(io.Discard, ptmx) }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stop := startSpinnerTTY(tty, tty, "思考中", "Esc 取消请求 · Ctrl+C 退出", cancel)

	time.Sleep(15 * time.Millisecond)
	if _, err := ptmx.Write([]byte{keyEsc, keyCtrlC}); err != nil {
		t.Fatalf("write esc+ctrl+c: %v", err)
	}

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("Esc+Ctrl+C did not cancel spinner context")
	}

	if err := stop(); !errors.Is(err, ErrInterrupted) {
		t.Fatalf("stop() after Esc+Ctrl+C = %v, want ErrInterrupted", err)
	}
}
