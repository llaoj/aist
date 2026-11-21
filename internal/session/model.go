// Package session owns the durable conversation domain: messages, compaction
// events, token accounting and the model/display views.
//
// The domain package does not read files, environment variables or UI. All
// persistence and model calls go through ports.
package session

import "time"

// Role identifies a message author.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Usage is token usage reported by a provider.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// IsZero reports whether no token counts are set.
func (u Usage) IsZero() bool {
	return u.PromptTokens == 0 && u.CompletionTokens == 0 && u.TotalTokens == 0
}

// WithTotal returns usage with TotalTokens filled from prompt + completion.
func (u Usage) WithTotal() Usage {
	if u.TotalTokens == 0 {
		u.TotalTokens = u.PromptTokens + u.CompletionTokens
	}
	return u
}

// Add accumulates another usage into u.
func (u *Usage) Add(other Usage) {
	other = other.WithTotal()
	u.PromptTokens += other.PromptTokens
	u.CompletionTokens += other.CompletionTokens
	u.TotalTokens += other.TotalTokens
}

// ToolOutcome is the language-neutral outcome of a tool execution.
type ToolOutcome string

const (
	ToolOutcomeSuccess  ToolOutcome = "success"
	ToolOutcomeFailed   ToolOutcome = "failed"
	ToolOutcomeSkipped  ToolOutcome = "skipped"
	ToolOutcomeRejected ToolOutcome = "rejected"
)

// ToolCall mirrors the OpenAI-compatible tool call shape without importing the
// llm package.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall carries a tool name and its JSON arguments.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Message is one raw conversation entry. Messages are never deleted by
// compaction; CompressedBy only records which compaction covers them.
type Message struct {
	ID                    string      `json:"id"`
	Seq                   int         `json:"seq"`
	Time                  time.Time   `json:"time"`
	Role                  Role        `json:"role"`
	Content               string      `json:"content,omitempty"`
	ToolCalls             []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID            string      `json:"tool_call_id,omitempty"`
	Name                  string      `json:"name,omitempty"`
	Model                 string      `json:"model,omitempty"`
	Usage                 *Usage      `json:"usage,omitempty"`
	ToolOutcome           ToolOutcome `json:"tool_outcome,omitempty"`
	CoveredByCompactionID string      `json:"-"`
	// Synthetic marks a generated model-view message that is never persisted.
	Synthetic bool `json:"-"`
}

// Compaction is a durable summary of a sequence range of raw messages.
type Compaction struct {
	ID                   string    `json:"id"`
	Seq                  int       `json:"seq"`
	Time                 time.Time `json:"time"`
	PreviousCompactionID string    `json:"previous_compaction_id,omitempty"`
	CoversFromSeq        int       `json:"covers_from_seq"`
	CoversToSeq          int       `json:"covers_to_seq"`
	Summary              string    `json:"summary"`
	Model                string    `json:"model,omitempty"`
	Usage                *Usage    `json:"usage,omitempty"`
	TokensBefore         int       `json:"tokens_before"`
	TokensAfter          int       `json:"tokens_after"`
}

// TokenAnchor records the last real provider response used for incremental
// trigger estimation.
type TokenAnchor struct {
	TotalTokens int  `json:"total_tokens"`
	AfterSeq    int  `json:"after_seq"`
	Valid       bool `json:"valid"`
}

// Session is a durable conversation.
type Session struct {
	ID                 string       `json:"id"`
	Title              string       `json:"title,omitempty"`
	Cwd                string       `json:"cwd,omitempty"`
	CreatedAt          time.Time    `json:"created_at"`
	UpdatedAt          time.Time    `json:"updated_at"`
	Messages           []Message    `json:"-"`
	Compactions        []Compaction `json:"-"`
	TokenUsage         Usage        `json:"-"`
	TokenAnchor        TokenAnchor  `json:"-"`
	LatestCompactionID string       `json:"-"`
	SchemaVersion      int          `json:"-"`
}

// LatestCompaction returns the compaction referenced by LastCompactionID or,
// when the ID is missing, the one with the highest sequence.
func (s *Session) LatestCompaction() *Compaction {
	if s == nil || len(s.Compactions) == 0 {
		return nil
	}
	if s.LatestCompactionID != "" {
		for i := range s.Compactions {
			if s.Compactions[i].ID == s.LatestCompactionID {
				return &s.Compactions[i]
			}
		}
	}
	latest := &s.Compactions[0]
	for i := 1; i < len(s.Compactions); i++ {
		if s.Compactions[i].Seq > latest.Seq {
			latest = &s.Compactions[i]
		}
	}
	return latest
}

// NextEventSeq returns the next event sequence number.
func (s *Session) NextEventSeq() int {
	next := 1
	if len(s.Messages) > 0 && s.Messages[len(s.Messages)-1].Seq >= next {
		next = s.Messages[len(s.Messages)-1].Seq + 1
	}
	for _, compaction := range s.Compactions {
		if compaction.Seq >= next {
			next = compaction.Seq + 1
		}
	}
	return next
}

// RecordCompaction records a compaction and marks covered messages.
func (s *Session) RecordCompaction(compaction Compaction) {
	s.Compactions = append(s.Compactions, compaction)
	s.LatestCompactionID = compaction.ID
	for i := range s.Messages {
		seq := s.Messages[i].Seq
		if seq >= compaction.CoversFromSeq && seq <= compaction.CoversToSeq {
			s.Messages[i].CoveredByCompactionID = compaction.ID
		}
	}
}
