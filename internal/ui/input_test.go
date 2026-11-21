package ui

import (
	"errors"
	"io"
	"strings"
	"testing"
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
