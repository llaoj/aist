package ui

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/creack/pty"
)

func TestReadLine(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "newline", in: "value\n", want: "value"},
		{name: "crlf", in: "value\r\n", want: "value"},
		{name: "no trailing newline", in: "value", want: "value"},
		{name: "empty", in: "", want: ""},
		{name: "preserves inner spaces", in: "  value  \n", want: "  value  "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readLine(strings.NewReader(tt.in))
			if err != nil {
				t.Fatalf("readLine() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("readLine() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReadLineError(t *testing.T) {
	wantErr := errors.New("boom")
	_, err := readLine(errReader{err: wantErr})
	if !errors.Is(err, wantErr) {
		t.Fatalf("readLine() error = %v, want %v", err, wantErr)
	}
}

type errReader struct {
	err error
}

func (r errReader) Read([]byte) (int, error) {
	return 0, r.err
}

var _ io.Reader = errReader{}

func TestInputModelUsesNativeCursorAndEmitsNoIdleRedraws(t *testing.T) {
	model := newInputModel("用户:", "Enter 发送 · Ctrl+C 退出")
	if model.textArea.VirtualCursor() {
		t.Fatal("expected native terminal cursor (VirtualCursor == false)")
	}
	view := model.View()
	if view.Cursor == nil {
		t.Fatal("expected hardware tea.Cursor on View")
	}

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

	p := tea.NewProgram(model, tea.WithInput(tty), tea.WithOutput(tty))
	runDone := make(chan error, 1)
	go func() {
		_, err := p.Run()
		runDone <- err
	}()

	// Wait for initial render to settle.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(out.String(), "用户:") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(150 * time.Millisecond)

	settledLen := len(out.String())
	// Wait longer than BubbleTea's 530ms blink interval and verify zero idle writes.
	time.Sleep(700 * time.Millisecond)
	if afterLen := len(out.String()); afterLen != settledLen {
		t.Fatalf("idle prompt emitted %d unexpected bytes (would reset scrollback in PuTTY): %q", afterLen-settledLen, out.String()[settledLen:])
	}

	if _, err := ptmx.Write([]byte{keyCtrlC}); err != nil {
		t.Fatalf("write ctrl+c: %v", err)
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("p.Run: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("program did not exit on Ctrl+C")
	}

	_ = tty.Close()
	_ = ptmx.Close()
	<-copyDone
}
