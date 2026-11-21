package executor

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/llaoj/aist/internal/scriptexec"
	"github.com/llaoj/aist/internal/target"
)

func TestMarkerFilterWriterStreamsPromptAndHidesMarkedPayload(t *testing.T) {
	const (
		begin = "\x02AIST_BEGIN\x02"
		end   = "\x02AIST_END\x02"
	)

	var visible bytes.Buffer
	w := newMarkerFilterWriter(&visible, begin, end)

	prompt := "[sudo] password for ops: "
	if _, err := w.Write([]byte(prompt)); err != nil {
		t.Fatalf("Write prompt: %v", err)
	}
	// Prompt must be visible immediately without waiting for Flush or newline.
	if visible.String() != prompt {
		t.Fatalf("visible before newline = %q, want %q", visible.String(), prompt)
	}

	stream := "\r\n" + begin + "\nSECRET_FILE_BASE64_PAYLOAD\n" + end + "\r\npost-note\n"
	// Feed byte-by-byte to exercise split marker boundaries.
	for i := 0; i < len(stream); i++ {
		_, _ = w.Write([]byte{stream[i]})
	}
	w.Flush()

	got := visible.String()
	if strings.Contains(got, "SECRET_FILE_BASE64_PAYLOAD") || strings.Contains(got, "AIST_BEGIN") || strings.Contains(got, "AIST_END") {
		t.Fatalf("visible leaked hidden payload: %q", got)
	}
	want := prompt + "\r\npost-note\n"
	if got != want {
		t.Fatalf("visible = %q, want %q", got, want)
	}
}

type failingDisplayWriter struct {
	calls int
}

func (w *failingDisplayWriter) Write([]byte) (int, error) {
	w.calls++
	return 0, errors.New("display broken")
}

func TestMarkerFilterWriterStopsWritingAfterDisplayFailure(t *testing.T) {
	dst := &failingDisplayWriter{}
	w := newMarkerFilterWriter(dst, "\x02AIST_BEGIN\x02", "\x02AIST_END\x02")
	for i := 0; i < 100; i++ {
		_, _ = w.Write([]byte("line\n"))
	}
	w.Flush()
	if dst.calls != 1 {
		t.Fatalf("display writes = %d, want 1", dst.calls)
	}
}

func TestSplitMarkedOutputExtractsPayloadAndSanitizesUnclosed(t *testing.T) {
	const (
		begin = "\x02AIST_BEGIN\x02"
		end   = "\x02AIST_END\x02"
	)

	raw := "[sudo] password:\r\n" + begin + "\r\nLINE1\r\nLINE2\r\n" + end + "\r\n"
	payload, outside := splitMarkedOutput(raw, begin, end)
	if payload != "LINE1\nLINE2\n" {
		t.Fatalf("payload = %q", payload)
	}
	if outside != "[sudo] password:\n" {
		t.Fatalf("outside = %q", outside)
	}

	unclosed := "before\n" + begin + "\nSENSITIVE_PARTIAL"
	payload, outside = splitMarkedOutput(unclosed, begin, end)
	if payload != "" || strings.Contains(outside, "SENSITIVE_PARTIAL") {
		t.Fatalf("unclosed leaked payload=%q outside=%q", payload, outside)
	}
}

func TestScriptRunnerRunPipeLocalPreservesBinaryStdinAndArgs(t *testing.T) {
	runner := NewScriptRunner(ScriptRunnerOptions{})
	input := []byte("line1\nline2 with 'quotes' $var\n")
	stdout, stderr, err := runner.RunPipe(context.Background(), scriptexec.PipeRequest{
		Target: target.Local(),
		Script: `printf '%s\n' "$1"; cat`,
		Args:   []string{"/etc/my config $1.conf"},
		Stdin:  input,
	})
	if err != nil {
		t.Fatalf("RunPipe err=%v stderr=%q", err, stderr)
	}
	want := "/etc/my config $1.conf\n" + string(input)
	if string(stdout) != want {
		t.Fatalf("stdout = %q, want %q", string(stdout), want)
	}
}
