package termio

import (
	"errors"
	"io"
	"os"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const writeStallTimeout = 5 * time.Second

var ErrTimeout = errors.New("terminal read timed out")

// Reader reads from a terminal descriptor without setting O_NONBLOCK, which
// would otherwise flip shared stdout/stderr descriptors into non-blocking mode.
type Reader struct {
	fd        int
	wakeRead  *os.File
	stopWatch chan struct{}
	watchDone chan struct{}
	closeOnce sync.Once
}

func NewReader(fd int, done <-chan struct{}) (*Reader, error) {
	wakeRead, wakeWrite, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	r := &Reader{
		fd:        fd,
		wakeRead:  wakeRead,
		stopWatch: make(chan struct{}),
		watchDone: make(chan struct{}),
	}
	go func() {
		defer close(r.watchDone)
		select {
		case <-done:
			_ = wakeWrite.Close()
		case <-r.stopWatch:
			_ = wakeWrite.Close()
		}
	}()
	return r, nil
}

func (r *Reader) Close() error {
	var err error
	r.closeOnce.Do(func() {
		close(r.stopWatch)
		<-r.watchDone
		err = r.wakeRead.Close()
	})
	return err
}

func (r *Reader) Read(buf []byte) (int, error) {
	return r.readPoll(buf, -1)
}

func (r *Reader) ReadTimeout(buf []byte, timeout time.Duration) (int, error) {
	ms := int(timeout.Milliseconds())
	if timeout > 0 && ms == 0 {
		ms = 1
	}
	if ms < 0 {
		ms = 0
	}
	return r.readPoll(buf, ms)
}

func (r *Reader) readPoll(buf []byte, timeoutMs int) (int, error) {
	var deadline time.Time
	if timeoutMs > 0 {
		deadline = time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
	}

	for {
		waitMs := timeoutMs
		if !deadline.IsZero() {
			rem := time.Until(deadline)
			if rem <= 0 {
				return 0, ErrTimeout
			}
			waitMs = int(rem.Milliseconds())
			if waitMs == 0 {
				waitMs = 1
			}
		}

		fds := []unix.PollFd{
			{Fd: int32(r.fd), Events: unix.POLLIN},
			{Fd: int32(r.wakeRead.Fd()), Events: unix.POLLIN},
		}
		n, err := unix.Poll(fds, waitMs)
		if err != nil {
			if errors.Is(err, syscall.EINTR) {
				continue
			}
			return 0, err
		}
		if n == 0 {
			return 0, ErrTimeout
		}
		if fds[1].Revents != 0 {
			return 0, io.EOF
		}
		if fds[0].Revents&unix.POLLIN != 0 {
			nr, readErr := syscall.Read(r.fd, buf)
			if nr > 0 {
				return nr, nil
			}
			if errors.Is(readErr, syscall.EINTR) || errors.Is(readErr, syscall.EAGAIN) || errors.Is(readErr, syscall.EWOULDBLOCK) {
				continue
			}
			if readErr == nil {
				return 0, io.EOF
			}
			return 0, readErr
		}
		if fds[0].Revents&(unix.POLLHUP|unix.POLLERR|unix.POLLNVAL) != 0 {
			return 0, io.EOF
		}
	}
}

// DrainInput discards currently buffered input bytes without setting O_NONBLOCK.
func DrainInput(fd int) {
	buf := make([]byte, 128)
	for {
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		n, err := unix.Poll(fds, 0)
		if err != nil {
			if errors.Is(err, syscall.EINTR) {
				continue
			}
			return
		}
		if n == 0 || fds[0].Revents&unix.POLLIN == 0 {
			return
		}
		nr, readErr := syscall.Read(fd, buf)
		if errors.Is(readErr, syscall.EINTR) {
			continue
		}
		if readErr != nil || nr <= 0 {
			return
		}
	}
}

type fdProvider interface {
	Fd() uintptr
}

// WriteAll writes all of p to w, retrying partial writes and transient EAGAIN
// in case a parent process left stdio in non-blocking mode.
func WriteAll(w io.Writer, p []byte) error {
	if w == nil || len(p) == 0 {
		return nil
	}
	stallDeadline := time.Time{}
	for len(p) > 0 {
		n, err := w.Write(p)
		if n > 0 {
			p = p[n:]
			stallDeadline = time.Time{}
			if len(p) == 0 {
				return nil
			}
		}
		if err != nil {
			if errors.Is(err, syscall.EINTR) {
				continue
			}
			if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) {
				if stallDeadline.IsZero() {
					stallDeadline = time.Now().Add(writeStallTimeout)
				} else if time.Now().After(stallDeadline) {
					return err
				}
				waitWritable(w, 50*time.Millisecond)
				continue
			}
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

func waitWritable(w io.Writer, maxWait time.Duration) {
	if f, hasFd := w.(fdProvider); hasFd {
		ms := int(maxWait.Milliseconds())
		if ms <= 0 {
			ms = 1
		}
		fds := []unix.PollFd{{Fd: int32(f.Fd()), Events: unix.POLLOUT}}
		_, _ = unix.Poll(fds, ms)
		return
	}
	time.Sleep(time.Millisecond)
}
