package ui

import (
	"errors"
	"io"
	"testing"
	"time"

	"github.com/creack/pty"
)

func TestPromptSelectSingleEnterAcrossManyRounds(t *testing.T) {
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Fatalf("pty.Open: %v", err)
	}
	defer ptmx.Close()
	defer tty.Close()

	if err := pty.Setsize(ptmx, &pty.Winsize{Rows: 28, Cols: 98}); err != nil {
		t.Fatalf("pty.Setsize: %v", err)
	}

	// Drain PTY master output so writes never block on a full buffer.
	go func() { _, _ = io.Copy(io.Discard, ptmx) }()

	options := []string{"执行", "跳过此命令", "拒绝全部，由我补充说明"}
	cases := []struct {
		name  string
		input string
		want  int
	}{
		{name: "single CR selects first", input: "\r", want: 0},
		{name: "single LF selects first", input: "\n", want: 0},
		{name: "kitty enter selects first", input: "\x1b[13u", want: 0},
		{name: "down then single CR selects second", input: "\x1b[B\r", want: 1},
		{name: "down twice then single CR selects third", input: "\x1b[B\x1b[B\r", want: 2},
		{name: "down then up selects first", input: "\x1b[B\x1b[A\r", want: 0},
		{name: "ss3 down then CR selects second", input: "\x1bOB\r", want: 1},
		{name: "focus event ignored before CR", input: "\x1b[I\r", want: 0},
	}

	// Repeat several rounds to verify zero leaked stdin readers steal the next Enter.
	for round := 0; round < 5; round++ {
		for _, tc := range cases {
			done := make(chan struct{})
			var got int
			var gotErr error
			go func() {
				defer close(done)
				got, gotErr = promptSelectTTY(tty, tty, "是否执行此命令?", "↑↓ 选择 · Enter 确认", "再按一次 Ctrl+C 退出", options)
			}()

			time.Sleep(10 * time.Millisecond)
			if _, err := ptmx.Write([]byte(tc.input)); err != nil {
				t.Fatalf("round %d %s write: %v", round, tc.name, err)
			}

			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatalf("round %d %s: promptSelectTTY did not complete on single Enter", round, tc.name)
			}
			if gotErr != nil {
				t.Fatalf("round %d %s err = %v", round, tc.name, gotErr)
			}
			if got != tc.want {
				t.Fatalf("round %d %s selected = %d, want %d", round, tc.name, got, tc.want)
			}
		}
	}
}

func TestPromptSelectCtrlCReturnsErrInterrupted(t *testing.T) {
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Fatalf("pty.Open: %v", err)
	}
	defer ptmx.Close()
	defer tty.Close()

	go func() { _, _ = io.Copy(io.Discard, ptmx) }()

	done := make(chan error, 1)
	go func() {
		_, err := promptSelectTTY(tty, tty, "是否执行此命令?", "↑↓ 选择 · Enter 确认", "再按一次 Ctrl+C 退出", []string{"执行", "跳过"})
		done <- err
	}()

	time.Sleep(10 * time.Millisecond)
	if _, err := ptmx.Write([]byte{keyCtrlC}); err != nil {
		t.Fatalf("write first ctrl+c: %v", err)
	}
	select {
	case err := <-done:
		t.Fatalf("promptSelectTTY exited on single Ctrl+C (err=%v)", err)
	case <-time.After(60 * time.Millisecond):
	}

	if _, err := ptmx.Write([]byte{keyCtrlC}); err != nil {
		t.Fatalf("write second ctrl+c: %v", err)
	}

	select {
	case err := <-done:
		if !errors.Is(err, ErrInterrupted) {
			t.Fatalf("err = %v, want ErrInterrupted", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("promptSelectTTY did not exit on second Ctrl+C")
	}
}
