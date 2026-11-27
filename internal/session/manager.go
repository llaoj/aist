package session

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"unicode"
)

// Settings are the session behavior settings derived from configuration.
type Settings struct {
	RetainSessions         int
	RetainTailTokens       int
	CompactionTriggerRatio float64
	CompactionEnabled      bool
	FallbackContextWindow  int
	ContextWindowLimit     int
	MaxOutputTokens        int
	Cwd                    string
	TitleMaxRunes          int
}

func (o Settings) withDefaults() Settings {
	if o.RetainSessions <= 0 {
		o.RetainSessions = 10
	}
	if o.RetainTailTokens <= 0 {
		o.RetainTailTokens = 20000
	}
	if o.CompactionTriggerRatio <= 0 || o.CompactionTriggerRatio > 1 {
		o.CompactionTriggerRatio = 0.85
	}
	if o.FallbackContextWindow <= 0 {
		o.FallbackContextWindow = 128000
	}
	if o.TitleMaxRunes <= 0 {
		o.TitleMaxRunes = 80
	}
	return o
}

// ManagerOptions wires the session manager ports and settings.
type ManagerOptions struct {
	Repository        Repository
	TokenEstimator    TokenEstimator
	Compactor         Compactor
	Clock             Clock
	IDGenerator       IDGenerator
	Settings          Settings
	OnCompactionError func(error)
	OnPruneError      func(error)
}

// Manager orchestrates appending, persisting, compaction and title
// derivation. It owns one active session.
type Manager struct {
	mu        sync.Mutex
	repo      Repository
	estimate  TokenEstimator
	compactor Compactor
	clock     Clock
	ids       IDGenerator
	opts      Settings
	session   *Session
	persisted bool

	onCompactionError func(error)
	onPruneError      func(error)
}

// NewManager creates a Manager. A memory repository is used when Repository is nil,
// which keeps interactive sessions usable in tests without touching disk.
func NewManager(opts ManagerOptions) *Manager {
	if opts.Repository == nil {
		opts.Repository = NewInMemoryRepository()
	}
	if opts.TokenEstimator == nil {
		opts.TokenEstimator = HeuristicEstimator{}
	}
	if opts.Clock == nil {
		opts.Clock = SystemClock{}
	}
	if opts.IDGenerator == nil {
		opts.IDGenerator = TimestampIDGenerator{}
	}
	return &Manager{
		repo:              opts.Repository,
		estimate:          opts.TokenEstimator,
		compactor:         opts.Compactor,
		clock:             opts.Clock,
		ids:               opts.IDGenerator,
		opts:              opts.Settings.withDefaults(),
		onCompactionError: opts.OnCompactionError,
		onPruneError:      opts.OnPruneError,
	}
}

// Create initializes a new session in memory; storage is deferred until the
// first message so unused sessions leave no empty records on disk.
func (s *Manager) Create(ctx context.Context) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.clock.Now()
	created := &Session{
		ID:            s.ids.NewSessionID(now),
		Cwd:           s.opts.Cwd,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: 1,
	}
	s.session = created
	s.persisted = false
	return s.session, nil
}

// Load restores a persisted session and refreshes its updated_at timestamp.
func (s *Manager) Load(ctx context.Context, id string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	loaded, err := s.repo.Load(ctx, id)
	if err != nil {
		return nil, err
	}
	s.session = loaded
	s.persisted = true
	s.session.UpdatedAt = s.clock.Now()
	if err := s.repo.SaveMetadata(ctx, s.session); err != nil {
		return nil, err
	}
	return s.session, nil
}

// Current returns the active session. The pointer is read-only to callers.
func (s *Manager) Current() *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.session
}

// ModelMessages returns the active model view.
func (s *Manager) ModelMessages() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.session.ModelMessages()
}

// DisplayMessages returns the active display view.
func (s *Manager) DisplayMessages() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.session.DisplayMessages()
}

// AppendUser records a user message.
func (s *Manager) AppendUser(ctx context.Context, content string) (Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.appendMessageLocked(ctx, RoleUser, content, "", nil, "", nil, "")
}

// AppendAssistant records an assistant message with optional tool calls and
// provider usage. Usage is not added to TokenUsage here; AfterResponse does it
// so that metadata updates stay in one place.
func (s *Manager) AppendAssistant(ctx context.Context, content string, calls []ToolCall, usage *Usage, model string) (Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.appendMessageLocked(ctx, RoleAssistant, content, "", calls, model, usage, "")
}

// AppendToolResult records one tool result and its structured outcome.
func (s *Manager) AppendToolResult(ctx context.Context, callID, content string, outcome ToolOutcome) (Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.appendMessageLocked(ctx, RoleTool, content, callID, nil, "", nil, outcome)
}

func (s *Manager) appendMessageLocked(ctx context.Context, role Role, content, toolCallID string, calls []ToolCall, model string, usage *Usage, outcome ToolOutcome) (Message, error) {
	if s.session == nil {
		return Message{}, errors.New("no active session")
	}
	now := s.clock.Now()
	message := Message{
		ID:          s.ids.NewMessageID(now),
		Seq:         s.session.NextEventSeq(),
		Time:        now,
		Role:        role,
		Content:     content,
		ToolCalls:   calls,
		ToolCallID:  toolCallID,
		Model:       model,
		Usage:       usage,
		ToolOutcome: outcome,
	}
	if !s.persisted {
		s.session.Messages = append(s.session.Messages, message)
		s.session.UpdatedAt = now
		if err := s.repo.Create(ctx, s.session); err != nil {
			s.session.Messages = s.session.Messages[:len(s.session.Messages)-1]
			return Message{}, err
		}
		s.persisted = true
		s.pruneSessions(ctx)
		return message, nil
	}
	if err := s.repo.AppendMessage(ctx, s.session.ID, message); err != nil {
		return Message{}, err
	}
	s.session.Messages = append(s.session.Messages, message)
	s.session.UpdatedAt = now
	if err := s.repo.SaveMetadata(ctx, s.session); err != nil {
		return message, err
	}
	return message, nil
}

// BeforeCall compacts the session when the estimated context reaches the
// configured trigger. Summary failures keep the raw history and are reported
// through OnCompactionError.
func (s *Manager) CompactIfNeeded(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.session == nil || !s.opts.CompactionEnabled || s.compactor == nil {
		return nil
	}

	estimated := EstimateForTrigger(s.session, s.estimate)
	limit := s.opts.ContextWindowLimit
	if limit <= 0 {
		limit = s.opts.FallbackContextWindow
	}
	if s.opts.MaxOutputTokens > 0 && s.opts.MaxOutputTokens < limit {
		limit -= s.opts.MaxOutputTokens
	}
	if limit <= 0 {
		limit = s.opts.FallbackContextWindow
	}
	if float64(estimated) < float64(limit)*s.opts.CompactionTriggerRatio {
		return nil
	}

	latest := s.session.LatestCompaction()
	var priorSummary string
	var priorSummaryMessage *Message
	if latest != nil {
		priorSummary = latest.Summary
		message := summaryMessage(latest)
		priorSummaryMessage = &message
	}

	plan := PlanCompaction(PlanInput{
		Summary:          priorSummaryMessage,
		PriorSummary:     s.session.RawMessagesAfterCompaction(),
		RetainTailTokens: s.opts.RetainTailTokens,
	}, s.estimate)
	if !plan.Feasible {
		return nil
	}

	middle := plan.Middle
	if priorSummaryMessage != nil && len(middle) > 0 && middle[0].ID == priorSummaryMessage.ID {
		middle = middle[1:]
	}
	if len(middle) == 0 {
		return nil
	}

	tokensBefore := s.estimate.EstimateMessages(s.session.ModelMessages())
	result, err := s.compactor.Compact(ctx, CompactionRequest{
		PriorSummary: priorSummary,
		Messages:     middle,
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		if s.onCompactionError != nil && ctx.Err() == nil {
			s.onCompactionError(err)
		}
		return nil
	}
	summaryText := normalizeSummary(result.Summary)
	if summaryText == "" {
		return nil
	}

	now := s.clock.Now()
	compaction := Compaction{
		ID:           s.ids.NewCompactionID(now),
		Seq:          s.session.NextEventSeq(),
		Time:         now,
		Summary:      summaryText,
		Model:        result.Model,
		TokensBefore: tokensBefore,
	}
	if result.Usage.WithTotal().TotalTokens > 0 {
		usage := result.Usage.WithTotal()
		compaction.Usage = &usage
	}
	if latest != nil {
		compaction.PreviousCompactionID = latest.ID
		compaction.CoversFromSeq = latest.CoversFromSeq
		compaction.CoversToSeq = latest.CoversToSeq
	}
	if len(middle) > 0 {
		if compaction.CoversFromSeq == 0 {
			compaction.CoversFromSeq = middle[0].Seq
		}
		compaction.CoversToSeq = middle[len(middle)-1].Seq
	}

	afterView := make([]Message, 0, 1+len(plan.Tail))
	afterView = append(afterView, Message{Role: RoleUser, Content: summaryText})
	afterView = append(afterView, plan.Tail...)
	compaction.TokensAfter = s.estimate.EstimateMessages(afterView)

	if err := s.repo.AppendCompaction(ctx, s.session.ID, compaction); err != nil {
		return err
	}
	s.session.RecordCompaction(compaction)
	if compaction.Usage != nil {
		s.session.TokenUsage.Add(*compaction.Usage)
	}
	s.session.TokenAnchor = TokenAnchor{}
	s.session.UpdatedAt = now
	return s.repo.SaveMetadata(ctx, s.session)
}

// AfterResponse records provider usage and moves the token anchor. Missing
// usage invalidates the anchor so the next trigger uses a full estimate.
func (s *Manager) RecordResponseUsage(ctx context.Context, usage *Usage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.session == nil {
		return errors.New("no active session")
	}
	if usage == nil || usage.WithTotal().TotalTokens == 0 {
		s.session.TokenAnchor = TokenAnchor{}
		s.session.UpdatedAt = s.clock.Now()
		if !s.persisted {
			return nil
		}
		return s.repo.SaveMetadata(ctx, s.session)
	}

	normalized := usage.WithTotal()
	s.session.TokenAnchor = TokenAnchor{
		TotalTokens: normalized.TotalTokens,
		AfterSeq:    lastAssistantSeq(s.session),
		Valid:       true,
	}
	s.session.TokenUsage.Add(normalized)
	s.session.UpdatedAt = s.clock.Now()
	if !s.persisted {
		return nil
	}
	if err := s.repo.SaveMetadata(ctx, s.session); err != nil {
		return err
	}
	s.pruneSessions(ctx)
	return nil
}

// SetTitle assigns an explicit title to the active session.
func (s *Manager) SetTitle(ctx context.Context, title string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.session == nil {
		return errors.New("no active session")
	}
	trimmed := truncateRunes(strings.TrimSpace(title), s.opts.TitleMaxRunes)
	if trimmed == "" {
		return nil
	}
	s.session.Title = trimmed
	s.session.UpdatedAt = s.clock.Now()
	if !s.persisted {
		return nil
	}
	return s.repo.SaveMetadata(ctx, s.session)
}

// SetDerivedTitle derives a compact title from text and assigns it to the active session.
func (s *Manager) SetDerivedTitle(ctx context.Context, text string) error {
	return s.SetTitle(ctx, deriveTitle(text, s.opts.TitleMaxRunes))
}

// EnsureTitle derives and persists a title once from the first user message.
func (s *Manager) EnsureTitle(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.session == nil || strings.TrimSpace(s.session.Title) != "" {
		return nil
	}
	user := firstUserMessage(s.session)
	if user == nil {
		return nil
	}

	title := deriveTitle(user.Content, s.opts.TitleMaxRunes)
	if title == "" {
		return nil
	}

	s.session.Title = title
	s.session.UpdatedAt = s.clock.Now()
	if !s.persisted {
		return nil
	}
	if err := s.repo.SaveMetadata(ctx, s.session); err != nil {
		return err
	}
	s.pruneSessions(ctx)
	return nil
}

// List returns persisted session summaries.
func (s *Manager) List(ctx context.Context) ([]SessionInfo, error) {
	return s.repo.List(ctx)
}

// Delete removes a persisted session.
func (s *Manager) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func (s *Manager) pruneSessions(ctx context.Context) {
	if s.session == nil || s.opts.RetainSessions <= 0 {
		return
	}
	summaries, err := s.repo.List(ctx)
	if err != nil {
		s.reportPruneError(err)
		return
	}
	sort.SliceStable(summaries, func(i, j int) bool {
		return summaries[i].UpdatedAt.After(summaries[j].UpdatedAt)
	})
	if len(summaries) <= s.opts.RetainSessions {
		return
	}
	for _, summary := range summaries[s.opts.RetainSessions:] {
		if summary.ID == s.session.ID {
			continue
		}
		if err := s.repo.Delete(ctx, summary.ID); err != nil {
			s.reportPruneError(err)
		}
	}
}

func (s *Manager) reportPruneError(err error) {
	if err != nil && s.onPruneError != nil {
		s.onPruneError(err)
	}
}

func lastAssistantSeq(session *Session) int {
	for i := len(session.Messages) - 1; i >= 0; i-- {
		if session.Messages[i].Role == RoleAssistant {
			return session.Messages[i].Seq
		}
	}
	return 0
}

func firstUserMessage(session *Session) *Message {
	for i := range session.Messages {
		if session.Messages[i].Role == RoleUser {
			return &session.Messages[i]
		}
	}
	return nil
}

func normalizeSummary(summary string) string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return ""
	}
	if !strings.HasPrefix(summary, summaryMarker) {
		summary = summaryMarker + "\n" + summary + "\n</compressed_history>"
	}
	if !strings.Contains(summary, "</compressed_history>") {
		summary += "\n</compressed_history>"
	}
	return summary
}

func deriveTitle(text string, maxRunes int) string {
	var sb strings.Builder
	prevSpace := false
	for _, r := range text {
		if unicode.IsSpace(r) {
			if sb.Len() > 0 && !prevSpace {
				sb.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		if unicode.IsControl(r) {
			continue
		}
		sb.WriteRune(r)
		prevSpace = false
	}
	return truncateRunes(sb.String(), maxRunes)
}

func truncateRunes(text string, maxRunes int) string {
	trimmed := strings.TrimSpace(text)
	if maxRunes <= 0 {
		return trimmed
	}
	runes := []rune(trimmed)
	if len(runes) <= maxRunes {
		return trimmed
	}
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	return strings.TrimRightFunc(string(runes[:maxRunes-3]), unicode.IsSpace) + "..."
}
