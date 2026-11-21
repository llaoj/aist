package executor

import (
	"bytes"
	"io"
	"strings"
	"sync"

	"github.com/llaoj/aist/internal/termio"
)

func splitMarkedOutput(raw, beginMarker, endMarker string) (payload, outside string) {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	if beginMarker == "" || endMarker == "" {
		return "", normalized
	}
	start := strings.Index(normalized, beginMarker)
	if start < 0 {
		return "", normalized
	}
	rest := normalized[start+len(beginMarker):]
	rest = strings.TrimPrefix(rest, "\n")
	end := strings.Index(rest, endMarker)
	if end < 0 {
		// Marker started but never closed: drop everything after start so partial
		// file payloads never leak into error messages.
		return "", normalized[:start]
	}
	body := rest[:end]
	after := strings.TrimPrefix(rest[end+len(endMarker):], "\n")
	return body, normalized[:start] + after
}

// markerFilterWriter forwards PTY bytes to dst immediately unless they fall
// inside a [beginMarker, endMarker] span, which is suppressed from display.
type markerFilterWriter struct {
	mu          sync.Mutex
	dst         io.Writer
	begin       []byte
	end         []byte
	pending     []byte
	suppressing bool
	trimNewline bool
	broken      bool
}

func newMarkerFilterWriter(dst io.Writer, beginMarker, endMarker string) *markerFilterWriter {
	return &markerFilterWriter{
		dst:   dst,
		begin: []byte(beginMarker),
		end:   []byte(endMarker),
	}
}

func (w *markerFilterWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	total := len(p)
	w.pending = append(w.pending, p...)
	w.drainLocked()
	return total, nil
}

func (w *markerFilterWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.trimNewline && bytes.Equal(w.pending, []byte("\r")) {
		w.pending = nil
		w.trimNewline = false
	}
	if !w.suppressing && len(w.pending) > 0 {
		w.writeLocked(w.pending)
	}
	w.pending = nil
}

func (w *markerFilterWriter) writeLocked(p []byte) {
	if w.broken || len(p) == 0 {
		return
	}
	if err := termio.WriteAll(w.dst, p); err != nil {
		w.broken = true
	}
}

func (w *markerFilterWriter) drainLocked() {
	for len(w.pending) > 0 {
		if !w.suppressing {
			if w.trimNewline {
				if w.pending[0] == '\r' {
					if len(w.pending) < 2 {
						return
					}
					if w.pending[1] == '\n' {
						w.pending = w.pending[2:]
					} else {
						w.pending = w.pending[1:]
					}
					w.trimNewline = false
					continue
				}
				if w.pending[0] == '\n' {
					w.pending = w.pending[1:]
				}
				w.trimNewline = false
				if len(w.pending) == 0 {
					return
				}
			}
			if idx := bytes.Index(w.pending, w.begin); idx >= 0 {
				if idx > 0 {
					w.writeLocked(w.pending[:idx])
				}
				w.pending = w.pending[idx+len(w.begin):]
				w.suppressing = true
				continue
			}
			keep := longestSuffixPrefixOverlap(w.pending, w.begin)
			flushLen := len(w.pending) - keep
			if flushLen > 0 {
				w.writeLocked(w.pending[:flushLen])
				w.pending = w.pending[flushLen:]
			}
			return
		}

		if idx := bytes.Index(w.pending, w.end); idx >= 0 {
			w.pending = w.pending[idx+len(w.end):]
			w.suppressing = false
			w.trimNewline = true
			continue
		}
		if maxTail := len(w.end) - 1; maxTail > 0 && len(w.pending) > maxTail {
			w.pending = w.pending[len(w.pending)-maxTail:]
		} else if len(w.end) <= 1 {
			w.pending = nil
		}
		return
	}
}

func longestSuffixPrefixOverlap(buf, prefix []byte) int {
	max := len(prefix) - 1
	if max <= 0 {
		return 0
	}
	if len(buf) < max {
		max = len(buf)
	}
	for k := max; k > 0; k-- {
		if bytes.Equal(buf[len(buf)-k:], prefix[:k]) {
			return k
		}
	}
	return 0
}
