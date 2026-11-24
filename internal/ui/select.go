package ui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/llaoj/aist/internal/termio"
	"golang.org/x/term"
)

type selectKey int

const (
	selectKeyIgnore selectKey = iota
	selectKeyPrev
	selectKeyNext
	selectKeyEnter
	selectKeyInterrupt
)

// PromptSelect displays a single-choice selector and returns the selected index.
func PromptSelect(prompt, keyHint string, options []string) (int, error) {
	return promptSelectTTY(os.Stdin, os.Stdout, prompt, keyHint, options)
}

func promptSelectTTY(in, out *os.File, prompt, keyHint string, options []string) (int, error) {
	if len(options) == 0 {
		return 0, nil
	}

	fd := int(in.Fd())
	if !term.IsTerminal(fd) {
		return 0, errors.New("select error: stdin is not a terminal")
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return 0, fmt.Errorf("select error: %w", err)
	}
	defer func() { _ = term.Restore(fd, oldState) }()

	done := make(chan struct{})
	defer close(done)

	reader, err := termio.NewReader(fd, done)
	if err != nil {
		return 0, fmt.Errorf("select error: %w", err)
	}
	defer reader.Close()

	_ = termio.WriteAll(out, []byte("\033[?25l"))
	defer func() { _ = termio.WriteAll(out, []byte("\033[?25h")) }()

	selected := 0
	prevRows := 0
	redraw := func(final bool) {
		prevRows = renderSelect(out, prompt, keyHint, options, selected, final, prevRows)
	}

	redraw(false)
	for {
		key, err := readSelectKey(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				redraw(true)
				return 0, ErrInterrupted
			}
			return 0, fmt.Errorf("select error: %w", err)
		}

		switch key {
		case selectKeyPrev:
			if selected > 0 {
				selected--
				redraw(false)
			}
		case selectKeyNext:
			if selected < len(options)-1 {
				selected++
				redraw(false)
			}
		case selectKeyEnter:
			redraw(true)
			return selected, nil
		case selectKeyInterrupt:
			redraw(true)
			return 0, ErrInterrupted
		}
	}
}

func renderSelect(out *os.File, prompt, keyHint string, options []string, selected int, final bool, prevRows int) int {
	cols, _, _ := term.GetSize(int(out.Fd()))

	var b strings.Builder
	if prevRows > 0 {
		fmt.Fprintf(&b, "\033[%dA\r\033[J", prevRows)
	}

	rows := 0
	writeLine := func(line string) {
		b.WriteString(line)
		b.WriteString("\r\n")
		if cols > 0 {
			if w := lipgloss.Width(line); w > cols {
				rows += (w + cols - 1) / cols
				return
			}
		}
		rows++
	}

	for _, line := range strings.Split(prompt, "\n") {
		writeLine(line)
	}
	for i, option := range options {
		if i == selected {
			writeLine("> [x] " + option)
		} else {
			writeLine("  [ ] " + option)
		}
	}
	if !final && keyHint != "" {
		writeLine(keyHintStyle.Sprint(keyHint))
	}
	if final {
		b.WriteString("\r\n")
	}

	_ = termio.WriteAll(out, []byte(b.String()))
	return rows
}

func readSelectKey(reader *termio.Reader) (selectKey, error) {
	var b [1]byte
	n, err := reader.Read(b[:])
	if err != nil || n == 0 {
		return selectKeyIgnore, err
	}

	switch b[0] {
	case 0x03:
		return selectKeyInterrupt, nil
	case '\r', '\n':
		return selectKeyEnter, nil
	case 0x1b:
		return readEscapeSequence(reader)
	default:
		return selectKeyIgnore, nil
	}
}

func readEscapeSequence(reader *termio.Reader) (selectKey, error) {
	var seq [16]byte
	n := 0
	wait := 25 * time.Millisecond

	for n < len(seq) {
		read, err := reader.ReadTimeout(seq[n:n+1], wait)
		if errors.Is(err, termio.ErrTimeout) || read == 0 {
			break
		}
		if err != nil {
			return selectKeyIgnore, err
		}
		n++
		wait = 5 * time.Millisecond

		// Stop as soon as a complete CSI (ESC [ ... <final>) or SS3 (ESC O <ch>) arrives.
		if n >= 2 && (seq[0] == 'O' || (seq[0] == '[' && seq[n-1] >= 0x40 && seq[n-1] <= 0x7e) || (seq[0] != '[' && seq[0] != 'O')) {
			break
		}
	}

	switch string(seq[:n]) {
	case "[A", "OA", "[D", "OD":
		return selectKeyPrev, nil
	case "[B", "OB", "[C", "OC":
		return selectKeyNext, nil
	case "[13u", "[13;1u", "[27;1;13~":
		return selectKeyEnter, nil
	default:
		return selectKeyIgnore, nil
	}
}
