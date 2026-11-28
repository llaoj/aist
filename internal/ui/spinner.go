package ui

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/llaoj/aist/internal/termio"
	"golang.org/x/term"
)

type spinnerKey int

const (
	spinnerKeyIgnore spinnerKey = iota
	spinnerKeyCancel
	spinnerKeyInterrupt
)

// StartSpinner starts a terminal spinner with a footer key hint.
// Pressing Esc cancels the in-flight operation; pressing Ctrl+C confirms and
// cancels the operation so stop() returns ErrInterrupted.
func StartSpinner(message, keyHint, confirmExitHint string, cancel context.CancelFunc) func() error {
	return startSpinnerTTY(os.Stdin, os.Stdout, message, keyHint, confirmExitHint, cancel)
}

func startSpinnerTTY(in, out *os.File, message, keyHint, confirmExitHint string, cancel context.CancelFunc) func() error {
	if out == nil || !term.IsTerminal(int(out.Fd())) {
		return func() error { return nil }
	}

	var (
		inFd             int
		oldState         *term.State
		interactiveInput bool
	)
	if in != nil && cancel != nil && term.IsTerminal(int(in.Fd())) {
		inFd = int(in.Fd())
		if state, err := term.MakeRaw(inFd); err == nil && state != nil {
			oldState = state
			interactiveInput = true
		}
	}

	styledHint := ""
	styledConfirmHint := ""
	if interactiveInput {
		if keyHint != "" {
			styledHint = keyHintStyle.Sprint(keyHint)
		}
		if confirmExitHint != "" {
			styledConfirmHint = exitConfirmHintStyle.Sprint(confirmExitHint)
		}
	}

	_ = termio.WriteAll(out, []byte("\033[?25l"))

	done := make(chan struct{})
	hintChanged := make(chan struct{}, 1)
	var (
		wg               sync.WaitGroup
		stopOnce         sync.Once
		interrupted      atomic.Bool
		confirmExitUntil atomic.Int64
	)

	wg.Add(1)
	go func() {
		defer wg.Done()
		runSpinnerDisplay(out, message, styledHint, styledConfirmHint, &confirmExitUntil, hintChanged, done)
	}()

	if interactiveInput {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runSpinnerInput(inFd, done, confirmExitHint != "", &confirmExitUntil, hintChanged, cancel, &interrupted)
		}()
	}

	return func() error {
		stopOnce.Do(func() {
			close(done)
			wg.Wait()
			if oldState != nil {
				termio.DrainInput(inFd)
				_ = term.Restore(inFd, oldState)
			}
		})
		if interrupted.Load() {
			return ErrInterrupted
		}
		return nil
	}
}

func runSpinnerDisplay(
	out *os.File,
	message, styledHint, styledConfirmHint string,
	confirmExitUntil *atomic.Int64,
	hintChanged <-chan struct{},
	done <-chan struct{},
) {
	frameTicker := time.NewTicker(120 * time.Millisecond)
	defer frameTicker.Stop()

	secTicker := time.NewTicker(1 * time.Second)
	defer secTicker.Stop()

	activeHint := func() string {
		if styledConfirmHint != "" && time.Now().UnixNano() < confirmExitUntil.Load() {
			return styledConfirmHint
		}
		return styledHint
	}

	frames := []string{"-", "\\", "|", "/"}
	frameIdx := 0
	elapsed := 0
	prevRows := renderSpinner(out, frames[frameIdx], message, elapsed, activeHint(), 0)

	for {
		select {
		case <-done:
			clearTerminalBlock(out, prevRows, true)
			return
		case <-hintChanged:
			prevRows = renderSpinner(out, frames[frameIdx], message, elapsed, activeHint(), prevRows)
		case <-frameTicker.C:
			frameIdx = (frameIdx + 1) % len(frames)
			prevRows = renderSpinner(out, frames[frameIdx], message, elapsed, activeHint(), prevRows)
		case <-secTicker.C:
			elapsed++
			prevRows = renderSpinner(out, frames[frameIdx], message, elapsed, activeHint(), prevRows)
		}
	}
}

func runSpinnerInput(
	inFd int,
	done <-chan struct{},
	requireConfirm bool,
	confirmExitUntil *atomic.Int64,
	hintChanged chan<- struct{},
	cancel context.CancelFunc,
	interrupted *atomic.Bool,
) {
	reader, err := termio.NewReader(inFd, done)
	if err != nil {
		return
	}
	defer reader.Close()

	for {
		key, err := readSpinnerKey(reader)
		if err != nil {
			return
		}
		switch key {
		case spinnerKeyCancel:
			cancel()
		case spinnerKeyInterrupt:
			now := time.Now()
			if !requireConfirm || now.UnixNano() < confirmExitUntil.Load() {
				interrupted.Store(true)
				cancel()
				return
			}
			confirmExitUntil.Store(now.Add(exitConfirmWindow).UnixNano())
			select {
			case hintChanged <- struct{}{}:
			default:
			}
		}
	}
}

func renderSpinner(out *os.File, frame, message string, elapsed int, styledHint string, prevRows int) int {
	statusLine := fmt.Sprintf("%s %s(%ds)", frame, message, elapsed)
	if styledHint == "" {
		return renderTerminalBlock(out, []string{statusLine}, false, prevRows)
	}
	return renderTerminalBlock(out, []string{statusLine, styledHint}, false, prevRows)
}

func readSpinnerKey(reader *termio.Reader) (spinnerKey, error) {
	var b [1]byte
	n, err := reader.Read(b[:])
	if err != nil || n == 0 {
		return spinnerKeyIgnore, err
	}

	switch b[0] {
	case keyCtrlC:
		return spinnerKeyInterrupt, nil
	case keyEsc:
		seq, err := readEscapeTail(reader)
		if err != nil {
			return spinnerKeyIgnore, err
		}
		if isCtrlCEscapeTail(seq) {
			return spinnerKeyInterrupt, nil
		}
		switch seq {
		case "", "[27u", "[27;1;27~":
			return spinnerKeyCancel, nil
		default:
			return spinnerKeyIgnore, nil
		}
	default:
		return spinnerKeyIgnore, nil
	}
}
