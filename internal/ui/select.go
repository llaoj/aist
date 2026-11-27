package ui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

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
	promptLines := strings.Split(prompt, "\n")
	lines := make([]string, 0, len(promptLines)+len(options)+1)
	lines = append(lines, promptLines...)
	for i, option := range options {
		if i == selected {
			lines = append(lines, "> [x] "+option)
		} else {
			lines = append(lines, "  [ ] "+option)
		}
	}
	if !final && keyHint != "" {
		lines = append(lines, keyHintStyle.Sprint(keyHint))
	}
	return renderTerminalBlock(out, lines, final, prevRows)
}

func readSelectKey(reader *termio.Reader) (selectKey, error) {
	var b [1]byte
	n, err := reader.Read(b[:])
	if err != nil || n == 0 {
		return selectKeyIgnore, err
	}

	switch b[0] {
	case keyCtrlC:
		return selectKeyInterrupt, nil
	case '\r', '\n':
		return selectKeyEnter, nil
	case keyEsc:
		return readSelectEscapeKey(reader)
	default:
		return selectKeyIgnore, nil
	}
}

func readSelectEscapeKey(reader *termio.Reader) (selectKey, error) {
	seq, err := readEscapeTail(reader)
	if err != nil {
		return selectKeyIgnore, err
	}
	if isCtrlCEscapeTail(seq) {
		return selectKeyInterrupt, nil
	}
	switch seq {
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
