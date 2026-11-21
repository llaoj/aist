package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/llaoj/aist/internal/termio"
	"golang.org/x/term"
)

// StartSpinner starts a terminal spinner with ESC interrupt support.
// If cancel is non-nil, pressing ESC will call cancel to abort the in-flight operation.
// Returns a stop function that blocks until spinner fully stops.
func StartSpinner(message, escHint string, cancel context.CancelFunc) func() {
	stat, err := os.Stdout.Stat()
	if err != nil || (stat.Mode()&os.ModeCharDevice) == 0 {
		return func() {}
	}

	fd := int(os.Stdin.Fd())
	supportESCInterrupt := cancel != nil && term.IsTerminal(fd)

	hint := ""
	if supportESCInterrupt {
		hint = escHint
	}

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		// Hide cursor while spinner is active
		fmt.Print("\033[?25l")

		frameTicker := time.NewTicker(120 * time.Millisecond)
		defer frameTicker.Stop()

		secTicker := time.NewTicker(1 * time.Second)
		defer secTicker.Stop()

		frames := []string{"-", "\\", "|", "/"}
		frameIdx := 0
		elapsed := 0

		for {
			select {
			case <-done:
				// Show cursor again before exiting
				fmt.Printf("\r\033[K\033[?25h")
				return
			case <-frameTicker.C:
				frameIdx = (frameIdx + 1) % len(frames)
				fmt.Printf("\r\033[K%s %s(%ds) %s\n\033[A", frames[frameIdx], message, elapsed, hint)
			case <-secTicker.C:
				elapsed++
				fmt.Printf("\r\033[K%s %s(%ds) %s\n\033[A", frames[frameIdx], message, elapsed, hint)
			}
		}
	}()

	// Start ESC listener within spinner lifecycle if cancel provided
	if supportESCInterrupt {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Switch stdin to raw mode so keypresses are delivered immediately
			// without waiting for Enter (line-buffered cooked mode would swallow ESC).
			oldState, err := term.MakeRaw(fd)
			if err != nil || oldState == nil {
				return
			}
			defer term.Restore(fd, oldState)
			defer termio.DrainInput(fd)

			reader, err := termio.NewReader(fd, done)
			if err != nil {
				return
			}
			defer reader.Close()

			buf := make([]byte, 1)
			seqBuf := make([]byte, 1)
			for {
				n, err := reader.Read(buf)
				if err != nil {
					return
				}

				if n > 0 && buf[0] == 0x1b {
					// If another byte arrives immediately, it is an escape
					// sequence (e.g. arrow keys \x1b[A), not a standalone ESC.
					_, seqErr := reader.ReadTimeout(seqBuf, 15*time.Millisecond)
					if errors.Is(seqErr, termio.ErrTimeout) {
						cancel()
						return
					}
					if seqErr != nil {
						return
					}
				}
			}
		}()
	}

	return func() {
		close(done)
		wg.Wait()
	}
}
