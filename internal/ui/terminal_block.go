package ui

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/fatih/color"
	"github.com/llaoj/aist/internal/termio"
	"golang.org/x/term"
)

const (
	keyCtrlC          = 0x03
	keyBEL            = 0x07
	keyEsc            = 0x1b
	exitConfirmWindow = 2 * time.Second
)

var exitConfirmHintStyle = color.New(color.FgYellow)

// renderTerminalBlock clears prevRows above the cursor and writes lines using
// CRLF so output aligns at column 0 even while raw mode disables OPOST.
func renderTerminalBlock(out *os.File, lines []string, trailingBlank bool, prevRows int) int {
	cols, _, _ := term.GetSize(int(out.Fd()))

	var b strings.Builder
	if prevRows > 0 {
		fmt.Fprintf(&b, "\033[%dA\r\033[J", prevRows)
	}

	rows := 0
	for _, line := range lines {
		b.WriteString(line)
		b.WriteString("\r\n")
		if cols > 0 {
			if w := lipgloss.Width(line); w > cols {
				rows += (w + cols - 1) / cols
				continue
			}
		}
		rows++
	}
	if trailingBlank {
		b.WriteString("\r\n")
	}

	_ = termio.WriteAll(out, []byte(b.String()))
	return rows
}

func clearTerminalBlock(out *os.File, prevRows int, showCursor bool) {
	var b strings.Builder
	if prevRows > 0 {
		fmt.Fprintf(&b, "\033[%dA\r\033[J", prevRows)
	} else {
		b.WriteString("\r\033[K")
	}
	if showCursor {
		b.WriteString("\033[?25h")
	}
	_ = termio.WriteAll(out, []byte(b.String()))
}

// readEscapeTail reads the bytes immediately following Esc (0x1b). An empty
// string indicates a standalone Esc keypress rather than an escape sequence.
func readEscapeTail(reader *termio.Reader) (string, error) {
	var seq [64]byte
	n := 0
	wait := 25 * time.Millisecond

	for n < len(seq) {
		read, err := reader.ReadTimeout(seq[n:n+1], wait)
		if errors.Is(err, termio.ErrTimeout) || read == 0 {
			break
		}
		if err != nil {
			return "", err
		}
		n++
		wait = 5 * time.Millisecond

		if isEscapeSequenceComplete(seq[:n]) {
			break
		}
	}

	return string(seq[:n]), nil
}

func isEscapeSequenceComplete(seq []byte) bool {
	n := len(seq)
	switch seq[0] {
	case '[':
		return n >= 2 && seq[n-1] >= 0x40 && seq[n-1] <= 0x7e
	case 'O':
		return n >= 2
	case ']':
		return seq[n-1] == keyBEL || (n >= 2 && seq[n-2] == keyEsc && seq[n-1] == '\\')
	default:
		return true
	}
}

func isCtrlCEscapeTail(seq string) bool {
	switch seq {
	case string(rune(keyCtrlC)), "[99;5u", "[27;5;99~":
		return true
	default:
		return false
	}
}
