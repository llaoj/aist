package session

import (
	"context"
	"time"
)

// SessionInfo is the metadata needed by a session list.
type SessionInfo struct {
	ID           string
	Title        string
	UpdatedAt    time.Time
	MessageCount int
}

// Repository persists session metadata and the append-only event log.
type Repository interface {
	Load(ctx context.Context, id string) (*Session, error)
	Create(ctx context.Context, session *Session) error
	AppendMessage(ctx context.Context, sessionID string, message Message) error
	AppendCompaction(ctx context.Context, sessionID string, compaction Compaction) error
	SaveMetadata(ctx context.Context, session *Session) error
	List(ctx context.Context) ([]SessionInfo, error)
	Delete(ctx context.Context, id string) error
}

// CompactionRequest asks for a durable compaction summary.
type CompactionRequest struct {
	PriorSummary string
	Messages     []Message
}

// CompactionResult is the compacted text, response model and its usage.
type CompactionResult struct {
	Summary string
	Model   string
	Usage   Usage
}

// Compactor produces compaction summaries. Implementations usually adapt an
// LLM manager; session does not import llm.
type Compactor interface {
	Compact(ctx context.Context, req CompactionRequest) (CompactionResult, error)
}

// Clock supplies the current time.
type Clock interface {
	Now() time.Time
}

// IDGenerator creates unique local identifiers.
type IDGenerator interface {
	NewSessionID(now time.Time) string
	NewMessageID(now time.Time) string
	NewCompactionID(now time.Time) string
}
