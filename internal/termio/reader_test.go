package termio

import (
	"bytes"
	"errors"
	"io"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

func TestReaderKeepsSharedTTYBlockingAndCancelsCleanly(t *testing.T) {
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Skipf("pty unavailable: %v", err)
	}
	defer ptmx.Close()
	defer tty.Close()

	stdinFd := int(tty.Fd())
	oldState, err := term.MakeRaw(stdinFd)
	if err != nil {
		t.Fatalf("MakeRaw: %v", err)
	}
	defer term.Restore(stdinFd, oldState)
	stdoutFd, err := syscall.Dup(stdinFd)
	if err != nil {
		t.Fatalf("Dup: %v", err)
	}
	defer syscall.Close(stdoutFd)

	done := make(chan struct{})
	reader, err := NewReader(stdinFd, done)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	defer reader.Close()

	flags, err := unix.FcntlInt(uintptr(stdoutFd), unix.F_GETFL, 0)
	if err != nil {
		t.Fatalf("FcntlInt: %v", err)
	}
	if flags&unix.O_NONBLOCK != 0 {
		t.Fatalf("shared stdout fd unexpectedly has O_NONBLOCK set: %#x", flags)
	}

	if _, err := ptmx.Write([]byte("hi")); err != nil {
		t.Fatalf("ptmx.Write: %v", err)
	}
	buf := make([]byte, 8)
	n, err := reader.Read(buf)
	if err != nil || string(buf[:n]) != "hi" {
		t.Fatalf("Read = (%q, %v), want (\"hi\", nil)", string(buf[:n]), err)
	}

	readDone := make(chan error, 1)
	go func() {
		_, rErr := reader.Read(buf)
		readDone <- rErr
	}()

	time.Sleep(20 * time.Millisecond)
	close(done)

	select {
	case rErr := <-readDone:
		if !errors.Is(rErr, io.EOF) {
			t.Fatalf("canceled Read err = %v, want io.EOF", rErr)
		}
	case <-time.After(time.Second):
		t.Fatal("Read did not unblock after done closed")
	}
}

func TestReadTimeoutAndDrainInput(t *testing.T) {
	rPipe, wPipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	defer rPipe.Close()
	defer wPipe.Close()

	done := make(chan struct{})
	defer close(done)

	reader, err := NewReader(int(rPipe.Fd()), done)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	defer reader.Close()

	buf := make([]byte, 8)
	if _, err := reader.ReadTimeout(buf, 15*time.Millisecond); !errors.Is(err, ErrTimeout) {
		t.Fatalf("ReadTimeout err = %v, want ErrTimeout", err)
	}

	if _, err := wPipe.Write([]byte("leftover")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	DrainInput(int(rPipe.Fd()))

	if _, err := reader.ReadTimeout(buf, 10*time.Millisecond); !errors.Is(err, ErrTimeout) {
		t.Fatalf("after DrainInput, ReadTimeout err = %v, want ErrTimeout", err)
	}
}

type flakyNonblockWriter struct {
	buf       bytes.Buffer
	chunkSize int
	eagains   int
}

func (w *flakyNonblockWriter) Write(p []byte) (int, error) {
	if len(p) > w.chunkSize {
		n, _ := w.buf.Write(p[:w.chunkSize])
		w.eagains++
		return n, syscall.EAGAIN
	}
	return w.buf.Write(p)
}

func TestWriteAllRetriesPartialEAGAIN(t *testing.T) {
	payload := bytes.Repeat([]byte("0123456789abcdef"), 256)
	dst := &flakyNonblockWriter{chunkSize: 300}
	if err := WriteAll(dst, payload); err != nil {
		t.Fatalf("WriteAll: %v", err)
	}
	if !bytes.Equal(dst.buf.Bytes(), payload) {
		t.Fatalf("written %d bytes, want %d", dst.buf.Len(), len(payload))
	}
	if dst.eagains == 0 {
		t.Fatal("expected EAGAIN retries to be exercised")
	}
}
