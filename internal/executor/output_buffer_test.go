package executor

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestLockedBufferUnlimitedKeepsEverything(t *testing.T) {
	buf := newLockedBuffer(0)
	_, _ = buf.Write([]byte(strings.Repeat("a", 100)))
	if got := buf.String(); got != strings.Repeat("a", 100) {
		t.Fatalf("String() length = %d", len(got))
	}
}

func TestLockedBufferUnderLimitKeepsEverything(t *testing.T) {
	buf := newLockedBuffer(10)
	_, _ = buf.Write([]byte("0123"))
	_, _ = buf.Write([]byte("4567"))
	if got := buf.String(); got != "01234567" {
		t.Fatalf("String() = %q", got)
	}
}

func TestLockedBufferKeepsHeadAndTail(t *testing.T) {
	buf := newLockedBuffer(10)
	for _, chunk := range []string{"01234", "56789", "abcde", "fghij"} {
		if n, err := buf.Write([]byte(chunk)); n != len(chunk) || err != nil {
			t.Fatalf("Write = %d, %v", n, err)
		}
	}
	want := "01234\n\n... [omitted 10 bytes] ...\n\nfghij"
	if got := buf.String(); got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestLockedBufferBoundsMemory(t *testing.T) {
	const limit = 1024
	buf := newLockedBuffer(limit)
	chunk := []byte(strings.Repeat("x", 300))
	for range 10000 {
		_, _ = buf.Write(chunk)
	}
	if buf.head.Len() > limit/2 || len(buf.tail) > 2*(limit-limit/2) {
		t.Fatalf("buffer grew: head=%d tail=%d", buf.head.Len(), len(buf.tail))
	}
	if !strings.Contains(buf.String(), "omitted") {
		t.Fatal("missing omission marker")
	}
}

func TestLockedBufferKeepsValidUTF8(t *testing.T) {
	buf := newLockedBuffer(10)
	_, _ = buf.Write([]byte(strings.Repeat("中", 10)))
	got := buf.String()
	if !utf8.ValidString(got) {
		t.Fatalf("String() is not valid UTF-8: %q", got)
	}
	if !strings.HasPrefix(got, "中") || !strings.HasSuffix(got, "中") {
		t.Fatalf("String() = %q", got)
	}
}
