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
// Pressing Esc cancels the in-flight operation; pressing Ctrl+C cancels the
// operation and causes stop() to return ErrInterrupted.
func StartSpinner(message, keyHint string, cancel context.CancelFunc) func() error {
	return startSpinnerTTY(os.Stdin, os.Stdout, message, keyHint, cancel)
}

func startSpinnerTTY(in, out *os.File, message, keyHint string, cancel context.CancelFunc) func() error {
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
	if interactiveInput && keyHint != "" {
		styledHint = keyHintStyle.Sprint(keyHint)
	}

	_ = termio.WriteAll(out, []byte("\033[?25l"))

	done := make(chan struct{})
	var (
		wg          sync.WaitGroup
		stopOnce    sync.Once
		interrupted atomic.Bool
	)

	wg.Add(1)
	go func() {
		defer wg.Done()
		runSpinnerDisplay(out, message, styledHint, done)
	}()

	if interactiveInput {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runSpinnerInput(inFd, done, cancel, &interrupted)
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

func runSpinnerDisplay(out *os.File, message, styledHint string, done <-chan struct{}) {
	frameTicker := time.NewTicker(120 * time.Millisecond)
	defer frameTicker.Stop()

	secTicker := time.NewTicker(1 * time.Second)
	defer secTicker.Stop()

	frames := []string{"-", "\\", "|", "/"}
	frameIdx := 0
	elapsed := 0
	prevRows := renderSpinner(out, frames[frameIdx], message, elapsed, styledHint, 0)

	for {
		select {
		case <-done:
			clearTerminalBlock(out, prevRows, true)
			return
		case <-frameTicker.C:
			frameIdx = (frameIdx + 1) % len(frames)
			prevRows = renderSpinner(out, frames[frameIdx], message, elapsed, styledHint, prevRows)
		case <-secTicker.C:
			elapsed++
			prevRows = renderSpinner(out, frames[frameIdx], message, elapsed, styledHint, prevRows)
		}
	}
}

func runSpinnerInput(inFd int, done <-chan struct{}, cancel context.CancelFunc, interrupted *atomic.Bool) {
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
			interrupted.Store(true)
			cancel()
			return
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
