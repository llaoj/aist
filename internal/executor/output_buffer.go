package executor

import (
	"bytes"
	"fmt"
	"sync"
	"unicode/utf8"
)

// maxCapturedOutputBytes bounds shell command output kept in memory. Tool
// results are cut to a few thousand characters anyway, so keeping the head and
// tail is enough while a runaway command cannot exhaust client memory.
const maxCapturedOutputBytes = 1 << 20

// lockedBuffer is an io.Writer safe to read while a copier goroutine may still
// be writing, which happens when we abandon a stuck command. A positive limit
// keeps only the first and last limit/2 bytes; zero keeps everything.
type lockedBuffer struct {
	mu      sync.Mutex
	limit   int
	head    bytes.Buffer
	tail    []byte
	omitted int
}

func newLockedBuffer(limit int) *lockedBuffer {
	return &lockedBuffer{limit: limit}
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.limit <= 0 {
		return b.head.Write(p)
	}
	headCap := b.limit / 2
	take := min(len(p), headCap-b.head.Len())
	b.head.Write(p[:take])
	b.tail = append(b.tail, p[take:]...)
	// Compact only after doubling so trimming stays amortized O(n).
	if tailCap := b.limit - headCap; len(b.tail) > 2*tailCap {
		drop := len(b.tail) - tailCap
		b.omitted += drop
		b.tail = append(b.tail[:0:0], b.tail[drop:]...)
	}
	return len(p), nil
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.limit <= 0 {
		return b.head.String()
	}
	tail := b.tail
	omitted := b.omitted
	if tailCap := b.limit - b.limit/2; len(tail) > tailCap {
		omitted += len(tail) - tailCap
		tail = tail[len(tail)-tailCap:]
	}
	if omitted == 0 {
		return b.head.String() + string(tail)
	}
	head := b.head.Bytes()
	headEnd := len(head)
	for i := len(head) - 1; i >= 0 && i >= len(head)-utf8.UTFMax; i-- {
		if utf8.RuneStart(head[i]) {
			if !utf8.FullRune(head[i:]) {
				headEnd = i
			}
			break
		}
	}
	tailStart := 0
	for tailStart < len(tail) && !utf8.RuneStart(tail[tailStart]) {
		tailStart++
	}
	omitted += len(head) - headEnd + tailStart
	return fmt.Sprintf("%s\n\n... [omitted %d bytes] ...\n\n%s", head[:headEnd], omitted, tail[tailStart:])
}
