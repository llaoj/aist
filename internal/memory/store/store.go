// Package store keeps an in-memory Memory snapshot.
package store

import (
	"context"
	"sync"

	"github.com/llaoj/aist/internal/memory"
)

// Snapshot is a concurrency-safe snapshot of loaded memories.
type Snapshot struct {
	mu       sync.RWMutex
	loader   memory.Loader
	memories []memory.Entry
	report   memory.Report
}

// NewSnapshot creates a Memory store backed by a loader.
func NewSnapshot(loader memory.Loader) *Snapshot {
	return &Snapshot{loader: loader}
}

// Reload replaces the snapshot with a fresh loading pass.
func (s *Snapshot) Reload(ctx context.Context) error {
	if s.loader == nil {
		return nil
	}
	result, err := s.loader.Load(ctx)

	s.mu.Lock()
	s.memories = append([]memory.Entry(nil), result.Memories...)
	s.report = result.Report
	s.mu.Unlock()
	return err
}

// Snapshot returns a copy of the current memories.
func (s *Snapshot) Snapshot() []memory.Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]memory.Entry(nil), s.memories...)
}

// Report returns the report from the last loading pass.
func (s *Snapshot) Report() memory.Report {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.report
}

// Lookup finds a Memory by name. The last loaded match wins.
func (s *Snapshot) Lookup(name string) (memory.Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := len(s.memories) - 1; i >= 0; i-- {
		if s.memories[i].Name == name {
			return s.memories[i], true
		}
	}
	return memory.Entry{}, false
}
