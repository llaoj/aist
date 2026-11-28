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

const (
	writeStallTimeout = 5 * time.Second
	maxSelectFD       = 1024
)

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

		fdReady, wakeReady, fdClosed, err := waitReadable(r.fd, int(r.wakeRead.Fd()), waitMs)
		if err != nil {
			if errors.Is(err, syscall.EINTR) {
				continue
			}
			return 0, err
		}
		if wakeReady {
			return 0, io.EOF
		}
		if fdReady {
			// select(2) reports a hung-up descriptor as readable, so a zero-byte
			// read with no error below is how hangup becomes io.EOF.
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
		if fdClosed {
			return 0, io.EOF
		}
	}
}

// waitReadable uses select(2) when descriptors fit in FD_SETSIZE because
// Darwin's poll(2) returns POLLNVAL immediately on /dev/tty.
func waitReadable(fd, wakeFd, timeoutMs int) (fdReady, wakeReady, fdClosed bool, err error) {
	if fd >= 0 && fd < maxSelectFD && wakeFd >= 0 && wakeFd < maxSelectFD {
		var rset unix.FdSet
		rset.Set(fd)
		rset.Set(wakeFd)
		nfds := max(fd, wakeFd) + 1
		var tv *unix.Timeval
		if timeoutMs >= 0 {
			t := unix.NsecToTimeval(int64(timeoutMs) * int64(time.Millisecond))
			tv = &t
		}
		n, selErr := unix.Select(nfds, &rset, nil, nil, tv)
		if selErr != nil {
			return false, false, false, selErr
		}
		if n == 0 {
			return false, false, false, ErrTimeout
		}
		return rset.IsSet(fd), rset.IsSet(wakeFd), false, nil
	}

	fds := []unix.PollFd{
		{Fd: int32(fd), Events: unix.POLLIN},
		{Fd: int32(wakeFd), Events: unix.POLLIN},
	}
	n, pollErr := unix.Poll(fds, timeoutMs)
	if pollErr != nil {
		return false, false, false, pollErr
	}
	if n == 0 {
		return false, false, false, ErrTimeout
	}
	if fds[1].Revents != 0 {
		return false, true, false, nil
	}
	if fds[0].Revents&unix.POLLIN != 0 {
		return true, false, false, nil
	}
	if fds[0].Revents&(unix.POLLHUP|unix.POLLERR|unix.POLLNVAL) != 0 {
		return false, false, true, nil
	}
	return false, false, false, nil
}

// DrainInput discards currently buffered input bytes without setting O_NONBLOCK.
func DrainInput(fd int) {
	buf := make([]byte, 128)
	for {
		ready, err := isReadableNow(fd)
		if err != nil {
			if errors.Is(err, syscall.EINTR) {
				continue
			}
			return
		}
		if !ready {
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

func isReadableNow(fd int) (bool, error) {
	if fd >= 0 && fd < maxSelectFD {
		var rset unix.FdSet
		rset.Set(fd)
		tv := unix.NsecToTimeval(0)
		n, err := unix.Select(fd+1, &rset, nil, nil, &tv)
		if err != nil {
			return false, err
		}
		return n > 0 && rset.IsSet(fd), nil
	}

	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	n, err := unix.Poll(fds, 0)
	if err != nil {
		return false, err
	}
	return n > 0 && fds[0].Revents&unix.POLLIN != 0, nil
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
