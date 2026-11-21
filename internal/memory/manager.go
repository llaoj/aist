package memory

import (
	"context"
	"errors"
	"fmt"
)

// ManagerOptions configures Memory orchestration.
type ManagerOptions struct {
	Snapshot SnapshotStore
	Writer   Writer
	Notifier Notifier
	MaxBytes int
}

// Manager coordinates Memory search, save and notifications.
type Manager struct {
	snapshot SnapshotStore
	writer   Writer
	notifier Notifier
	maxBytes int
}

// NewManager creates a Memory Manager.
func NewManager(opts ManagerOptions) *Manager {
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = DefaultMaxBytes
	}
	return &Manager{
		snapshot: opts.Snapshot,
		writer:   opts.Writer,
		notifier: opts.Notifier,
		maxBytes: opts.MaxBytes,
	}
}

// Search returns matching Memory entries from the current snapshot.
func (s *Manager) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.snapshot == nil {
		return nil, errors.New("memory store is not configured")
	}
	return Search(s.snapshot.Snapshot(), query, SearchOptions{
		Limit:    limit,
		MaxBytes: s.maxBytes,
	}), nil
}

// Change applies append or replace semantics to a Memory entry.
func (s *Manager) Change(ctx context.Context, req ChangeRequest) (ChangeResult, error) {
	if err := ctx.Err(); err != nil {
		return ChangeResult{}, err
	}
	if s == nil || s.snapshot == nil {
		return ChangeResult{}, errors.New("memory store is not configured")
	}
	if s.writer == nil {
		return ChangeResult{}, errors.New("memory writer is not configured")
	}

	mode, err := ParseChangeOperation(string(req.Operation))
	if err != nil {
		return ChangeResult{}, err
	}
	name, err := NormalizeName(req.Name)
	if err != nil {
		return ChangeResult{}, err
	}

	existing, exists := s.snapshot.Lookup(name)
	destination := WriteDestination{Name: name}
	if exists {
		destination.Path = existing.Path
	}

	body, err := SanitizeContent(req.Content)
	if err != nil {
		return ChangeResult{}, err
	}
	summary, err := sanitizeField(req.Summary)
	if err != nil {
		return ChangeResult{}, err
	}
	memType, err := sanitizeField(req.Type)
	if err != nil {
		return ChangeResult{}, err
	}
	source, err := sanitizeField(req.Source)
	if err != nil {
		return ChangeResult{}, err
	}

	content := WriteContent{
		Content: body,
		Summary: singleLine(summary),
		Type:    memType,
		Source:  singleLine(source),
	}

	var saved Entry
	if mode == ChangeOperationReplace {
		saved, err = s.writer.Upsert(ctx, destination, content)
	} else {
		saved, err = s.writer.Append(ctx, destination, content)
	}
	if err != nil {
		return ChangeResult{}, err
	}

	action := ChangeActionUpdated
	if !exists {
		action = ChangeActionCreated
	} else if mode == ChangeOperationReplace {
		action = ChangeActionReplaced
	}
	result := ChangeResult{Entry: saved, Operation: mode, Action: action}
	if err := s.reload(ctx); err != nil {
		return result, err
	}
	s.notify(ctx, result)
	return result, nil
}

func (s *Manager) reload(ctx context.Context) error {
	if err := s.snapshot.Reload(ctx); err != nil {
		return fmt.Errorf("memory saved but reload failed: %w", err)
	}
	return nil
}

func (s *Manager) notify(ctx context.Context, result ChangeResult) {
	if s.notifier != nil {
		s.notifier.MemoryChanged(ctx, result)
	}
}

// Lookup returns one Memory by name.
func (s *Manager) Lookup(name string) (Entry, bool) {
	if s == nil || s.snapshot == nil {
		return Entry{}, false
	}
	normalized, err := NormalizeName(name)
	if err != nil {
		return Entry{}, false
	}
	return s.snapshot.Lookup(normalized)
}

// Report returns the latest loading report.
func (s *Manager) Report() Report {
	if s == nil || s.snapshot == nil {
		return Report{}
	}
	return s.snapshot.Report()
}
