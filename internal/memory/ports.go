package memory

import "context"

// Loader discovers and parses Memory files.
type Loader interface {
	Load(ctx context.Context) (LoadResult, error)
}

// WriteDestination identifies the file to update. Path is optional; when empty the
// writer uses its default directory and the normalized name.
type WriteDestination struct {
	Name string
	Path string
}

// WriteContent is the sanitized payload for an append or replace operation.
type WriteContent struct {
	Content string
	Summary string
	Type    string
	Source  string
}

// Writer persists Memory files. Notifications belong to the manager, not to
// storage adapters.
type Writer interface {
	Append(ctx context.Context, destination WriteDestination, content WriteContent) (Entry, error)
	Upsert(ctx context.Context, destination WriteDestination, content WriteContent) (Entry, error)
}

// SnapshotStore holds an in-memory Memory snapshot.
type SnapshotStore interface {
	Snapshot() []Entry
	Report() Report
	Lookup(name string) (Entry, bool)
	Reload(ctx context.Context) error
}

// Notifier reports successful Memory changes. Implementations render in the
// UI layer; the domain manager never prints.
type Notifier interface {
	MemoryChanged(ctx context.Context, result ChangeResult)
}
