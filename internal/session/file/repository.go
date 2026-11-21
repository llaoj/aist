// Package file persists sessions as session.json metadata plus an append-only
// messages.jsonl event log below ~/.aist/sessions/<id>/.
package file

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/llaoj/aist/internal/session"
	"github.com/llaoj/aist/internal/state"
	"github.com/llaoj/aist/internal/storage"
)

const (
	metaFileName     = "session.json"
	messagesFileName = "messages.jsonl"
	dirMode          = os.FileMode(0o700)
	fileMode         = os.FileMode(0o600)
	maxLineSize      = 16 << 20
)

// RepositoryOptions configures the JSONL repository.
type RepositoryOptions struct {
	// Root is the state root (~/.aist).
	Root string
	// State overrides Root with a prepared state layout.
	State *state.Root
}

// Repository is the filesystem session repository.
type Repository struct {
	paths state.Paths
}

// NewRepository creates a JSONL repository.
func NewRepository(opts RepositoryOptions) *Repository {
	var paths state.Paths
	if opts.State != nil {
		paths = opts.State.Paths
	} else {
		paths = state.NewPaths(opts.Root)
	}
	return &Repository{paths: paths}
}

type metadata struct {
	SchemaVersion    int                 `json:"schema_version"`
	ID               string              `json:"id"`
	Title            string              `json:"title,omitempty"`
	Cwd              string              `json:"cwd,omitempty"`
	CreatedAt        time.Time           `json:"created_at"`
	UpdatedAt        time.Time           `json:"updated_at"`
	MessageCount     int                 `json:"message_count"`
	LastCompactionID string              `json:"last_compaction_id,omitempty"`
	TokenUsage       session.Usage       `json:"token_usage"`
	TokenAnchor      session.TokenAnchor `json:"token_anchor"`
}

type eventHeader struct {
	Type string    `json:"type"`
	ID   string    `json:"id"`
	Seq  int       `json:"seq"`
	Time time.Time `json:"time"`
}

type messageEvent struct {
	Type string `json:"type"`
	session.Message
}

type compactionEvent struct {
	Type                 string    `json:"type"`
	ID                   string    `json:"id"`
	Seq                  int       `json:"seq"`
	Time                 time.Time `json:"time"`
	PreviousCompactionID string    `json:"previous_compaction_id,omitempty"`
	Covers               struct {
		FromSeq int `json:"from_seq"`
		ToSeq   int `json:"to_seq"`
	} `json:"covers"`
	Summary      string         `json:"summary"`
	Model        string         `json:"model,omitempty"`
	Usage        *session.Usage `json:"usage,omitempty"`
	TokensBefore int            `json:"tokens_before"`
	TokensAfter  int            `json:"tokens_after"`
}

// Create writes metadata and creates the event log, including any initial
// messages supplied by the caller.
func (r *Repository) Create(ctx context.Context, sess *session.Session) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir, err := r.ensureSessionDir(sess.ID)
	if err != nil {
		return err
	}
	// Create the append log explicitly so a fresh session is visible even
	// before its first message.
	file, err := os.OpenFile(filepath.Join(dir, messagesFileName), os.O_CREATE|os.O_WRONLY, fileMode)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", messagesFileName, err)
	}
	_ = file.Chmod(fileMode)
	if err := file.Close(); err != nil {
		return err
	}
	for _, message := range sess.Messages {
		if err := r.appendEvent(sess.ID, messageEvent{Type: "message", Message: message}); err != nil {
			return err
		}
	}
	if err := r.writeMetadata(sess); err != nil {
		return err
	}
	return nil
}

// Load reads metadata and replays the event log.
func (r *Repository) Load(ctx context.Context, id string) (*session.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir, err := r.sessionDir(id)
	if err != nil {
		return nil, err
	}
	metaPath := filepath.Join(dir, metaFileName)
	eventsPath := filepath.Join(dir, messagesFileName)
	metaExists := fileExists(metaPath)
	eventsExists := fileExists(eventsPath)
	if !metaExists && !eventsExists {
		return nil, fmt.Errorf("session %s not found", id)
	}

	events, eventsErr := loadEvents(eventsPath)
	meta, metaErr := readMetadata(metaPath)
	if metaErr != nil {
		if eventsErr != nil {
			return nil, fmt.Errorf("session metadata is damaged and events cannot be replayed: %w", metaErr)
		}
		return rehydrateFromEvents(id, events), nil
	}
	if eventsErr != nil {
		return nil, eventsErr
	}

	sess := sessionFromMetadata(meta)
	sess.Messages = events.messages
	sess.Compactions = nil
	sess.LatestCompactionID = ""
	for _, compaction := range events.compactions {
		sess.RecordCompaction(compaction)
	}
	if sess.TokenUsage.IsZero() {
		sess.TokenUsage = rebuildUsage(sess)
	}
	return sess, nil
}

func rehydrateFromEvents(id string, events replayedEvents) *session.Session {
	sess := &session.Session{ID: id, SchemaVersion: 1}
	for _, message := range events.messages {
		if sess.CreatedAt.IsZero() || message.Time.Before(sess.CreatedAt) {
			sess.CreatedAt = message.Time
		}
		if message.Time.After(sess.UpdatedAt) {
			sess.UpdatedAt = message.Time
		}
	}
	for _, compaction := range events.compactions {
		if sess.CreatedAt.IsZero() || compaction.Time.Before(sess.CreatedAt) {
			sess.CreatedAt = compaction.Time
		}
		if compaction.Time.After(sess.UpdatedAt) {
			sess.UpdatedAt = compaction.Time
		}
	}
	sess.Messages = events.messages
	for _, compaction := range events.compactions {
		sess.RecordCompaction(compaction)
	}
	sess.TokenUsage = rebuildUsage(sess)
	return sess
}

// AppendMessage appends one message event.
func (r *Repository) AppendMessage(ctx context.Context, sessionID string, message session.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.appendEvent(sessionID, messageEvent{Type: "message", Message: message})
}

// AppendCompaction appends one compaction event.
func (r *Repository) AppendCompaction(ctx context.Context, sessionID string, compaction session.Compaction) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	event := compactionEvent{
		Type:                 "compaction",
		ID:                   compaction.ID,
		Seq:                  compaction.Seq,
		Time:                 compaction.Time,
		PreviousCompactionID: compaction.PreviousCompactionID,
		Summary:              compaction.Summary,
		Model:                compaction.Model,
		Usage:                compaction.Usage,
		TokensBefore:         compaction.TokensBefore,
		TokensAfter:          compaction.TokensAfter,
	}
	event.Covers.FromSeq = compaction.CoversFromSeq
	event.Covers.ToSeq = compaction.CoversToSeq
	return r.appendEvent(sessionID, event)
}

// SaveMetadata atomically replaces session.json.
func (r *Repository) SaveMetadata(ctx context.Context, sess *session.Session) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir, err := r.sessionDir(sess.ID)
	if err != nil {
		return err
	}
	return writeMetadataFile(filepath.Join(dir, metaFileName), sess)
}

// List scans session metadata and returns summaries sorted by updated_at.
func (r *Repository) List(ctx context.Context) ([]session.SessionInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(r.paths.SessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}
	summaries := make([]session.SessionInfo, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		meta, err := readMetadata(filepath.Join(r.paths.SessionsDir, entry.Name(), metaFileName))
		if err != nil {
			continue
		}
		summaries = append(summaries, session.SessionInfo{
			ID:           meta.ID,
			Title:        meta.Title,
			UpdatedAt:    meta.UpdatedAt,
			MessageCount: meta.MessageCount,
		})
	}
	sort.SliceStable(summaries, func(i, j int) bool {
		return summaries[i].UpdatedAt.After(summaries[j].UpdatedAt)
	})
	return summaries, nil
}

// Delete removes the whole session directory.
func (r *Repository) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir, err := r.sessionDir(id)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("failed to delete session %s: %w", id, err)
	}
	return nil
}

func (r *Repository) appendEvent(sessionID string, event any) error {
	dir, err := r.sessionDir(sessionID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return fmt.Errorf("failed to create session directory: %w", err)
	}
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to encode session event: %w", err)
	}
	data = append(data, '\n')

	file, err := os.OpenFile(filepath.Join(dir, messagesFileName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, fileMode)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", messagesFileName, err)
	}
	defer file.Close()
	_ = file.Chmod(fileMode)
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("failed to append session event: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("failed to sync session event: %w", err)
	}
	return nil
}

func (r *Repository) writeMetadata(sess *session.Session) error {
	dir, err := r.ensureSessionDir(sess.ID)
	if err != nil {
		return err
	}
	return writeMetadataFile(filepath.Join(dir, metaFileName), sess)
}

func (r *Repository) ensureSessionDir(id string) (string, error) {
	dir, err := r.sessionDir(id)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return "", fmt.Errorf("failed to create session directory: %w", err)
	}
	if err := os.Chmod(dir, dirMode); err != nil {
		return "", fmt.Errorf("failed to set session directory permissions: %w", err)
	}
	return dir, nil
}

func (r *Repository) sessionDir(id string) (string, error) {
	if !validID(id) {
		return "", fmt.Errorf("invalid session id %q", id)
	}
	return r.paths.SessionDir(id), nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func validID(id string) bool {
	return id != "" && id != "." && id != ".." && filepath.Base(id) == id && !strings.ContainsAny(id, `/\`)
}

func writeMetadataFile(path string, sess *session.Session) error {
	meta := metadataFromSession(sess)
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode session metadata: %w", err)
	}
	data = append(data, '\n')
	if err := storage.WriteFile(path, data, storage.WriteOptions{FileMode: fileMode, DirMode: dirMode}); err != nil {
		return err
	}
	return nil
}

func metadataFromSession(sess *session.Session) metadata {
	return metadata{
		SchemaVersion:    sess.SchemaVersion,
		ID:               sess.ID,
		Title:            sess.Title,
		Cwd:              sess.Cwd,
		CreatedAt:        sess.CreatedAt,
		UpdatedAt:        sess.UpdatedAt,
		MessageCount:     len(sess.Messages),
		LastCompactionID: sess.LatestCompactionID,
		TokenUsage:       sess.TokenUsage,
		TokenAnchor:      sess.TokenAnchor,
	}
}

func sessionFromMetadata(meta metadata) *session.Session {
	return &session.Session{
		ID:                 meta.ID,
		Title:              meta.Title,
		Cwd:                meta.Cwd,
		CreatedAt:          meta.CreatedAt,
		UpdatedAt:          meta.UpdatedAt,
		TokenUsage:         meta.TokenUsage,
		TokenAnchor:        meta.TokenAnchor,
		LatestCompactionID: meta.LastCompactionID,
		SchemaVersion:      meta.SchemaVersion,
	}
}

func readMetadata(path string) (metadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return metadata{}, fmt.Errorf("failed to read session metadata %s: %w", path, err)
	}
	var meta metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return metadata{}, fmt.Errorf("failed to parse session metadata %s: %w", path, err)
	}
	if meta.SchemaVersion == 0 {
		meta.SchemaVersion = 1
	}
	if meta.ID == "" {
		return metadata{}, fmt.Errorf("session metadata %s has no id", path)
	}
	return meta, nil
}

type replayedEvents struct {
	messages    []session.Message
	compactions []session.Compaction
}

func loadEvents(path string) (replayedEvents, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return replayedEvents{}, nil
		}
		return replayedEvents{}, fmt.Errorf("failed to read session events %s: %w", path, err)
	}
	if len(data) == 0 {
		return replayedEvents{}, nil
	}

	lines := strings.Split(string(data), "\n")
	if strings.HasSuffix(string(data), "\n") {
		lines = lines[:len(lines)-1]
	}

	seenMessages := make(map[string]bool)
	seenCompactions := make(map[string]bool)
	result := replayedEvents{}
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(line) > maxLineSize {
			return replayedEvents{}, fmt.Errorf("session event line exceeds %d bytes", maxLineSize)
		}
		var header eventHeader
		if err := json.Unmarshal([]byte(line), &header); err != nil {
			if i == len(lines)-1 {
				continue
			}
			return replayedEvents{}, fmt.Errorf("failed to parse session event %s:%d: %w", path, i+1, err)
		}
		switch header.Type {
		case "message":
			var event messageEvent
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				return replayedEvents{}, fmt.Errorf("failed to parse message event %s:%d: %w", path, i+1, err)
			}
			if event.ID == "" || seenMessages[event.ID] {
				continue
			}
			seenMessages[event.ID] = true
			result.messages = append(result.messages, event.Message)
		case "compaction":
			var event compactionEvent
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				return replayedEvents{}, fmt.Errorf("failed to parse compaction event %s:%d: %w", path, i+1, err)
			}
			if event.ID == "" || seenCompactions[event.ID] {
				continue
			}
			seenCompactions[event.ID] = true
			result.compactions = append(result.compactions, session.Compaction{
				ID:                   event.ID,
				Seq:                  event.Seq,
				Time:                 event.Time,
				PreviousCompactionID: event.PreviousCompactionID,
				CoversFromSeq:        event.Covers.FromSeq,
				CoversToSeq:          event.Covers.ToSeq,
				Summary:              event.Summary,
				Model:                event.Model,
				Usage:                event.Usage,
				TokensBefore:         event.TokensBefore,
				TokensAfter:          event.TokensAfter,
			})
		}
	}

	sort.SliceStable(result.messages, func(i, j int) bool {
		return result.messages[i].Seq < result.messages[j].Seq
	})
	sort.SliceStable(result.compactions, func(i, j int) bool {
		return result.compactions[i].Seq < result.compactions[j].Seq
	})
	return result, nil
}

func rebuildUsage(sess *session.Session) session.Usage {
	var usage session.Usage
	for _, message := range sess.Messages {
		if message.Role == session.RoleAssistant && message.Usage != nil {
			usage.Add(*message.Usage)
		}
	}
	for _, compaction := range sess.Compactions {
		if compaction.Usage != nil {
			usage.Add(*compaction.Usage)
		}
	}
	return usage
}
