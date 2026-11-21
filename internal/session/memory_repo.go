package session

import (
	"context"
	"errors"
	"sort"
	"sync"
)

// ErrSessionNotFound is returned when a session ID does not exist.
var ErrSessionNotFound = errors.New("session not found")

// InMemoryRepository is an in-memory Repository used by tests and by callers
// that construct an interactive session without the application container.
type InMemoryRepository struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// NewInMemoryRepository creates an empty repository.
func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{sessions: make(map[string]*Session)}
}

// Load returns a detached copy of a session.
func (r *InMemoryRepository) Load(_ context.Context, id string) (*Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	session, ok := r.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return cloneSession(session), nil
}

// Create stores a new session.
func (r *InMemoryRepository) Create(_ context.Context, session *Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.sessions[session.ID]; exists {
		return errors.New("session already exists")
	}
	r.sessions[session.ID] = cloneSession(session)
	return nil
}

// AppendMessage appends one message.
func (r *InMemoryRepository) AppendMessage(_ context.Context, id string, message Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	session, ok := r.sessions[id]
	if !ok {
		return ErrSessionNotFound
	}
	session.Messages = append(session.Messages, cloneMessage(message))
	return nil
}

// AppendCompaction appends one compaction and marks covered messages.
func (r *InMemoryRepository) AppendCompaction(_ context.Context, id string, compaction Compaction) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	session, ok := r.sessions[id]
	if !ok {
		return ErrSessionNotFound
	}
	session.RecordCompaction(compaction)
	return nil
}

// SaveMetadata stores metadata.
func (r *InMemoryRepository) SaveMetadata(_ context.Context, session *Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.sessions[session.ID]
	if !ok {
		return ErrSessionNotFound
	}
	current.Title = session.Title
	current.UpdatedAt = session.UpdatedAt
	current.TokenUsage = session.TokenUsage
	current.TokenAnchor = session.TokenAnchor
	current.LatestCompactionID = session.LatestCompactionID
	return nil
}

// List returns summaries sorted by updated_at descending.
func (r *InMemoryRepository) List(_ context.Context) ([]SessionInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]SessionInfo, 0, len(r.sessions))
	for _, session := range r.sessions {
		out = append(out, SessionInfo{
			ID:           session.ID,
			Title:        session.Title,
			UpdatedAt:    session.UpdatedAt,
			MessageCount: len(session.Messages),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

// Delete removes a session.
func (r *InMemoryRepository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, id)
	return nil
}

func cloneSession(session *Session) *Session {
	cloned := *session
	cloned.Messages = appendMessages(nil, session.Messages)
	cloned.Compactions = append([]Compaction(nil), session.Compactions...)
	if session.TokenUsage != (Usage{}) {
		cloned.TokenUsage = session.TokenUsage
	}
	return &cloned
}

func cloneMessage(message Message) Message {
	return appendMessages(nil, []Message{message})[0]
}
