package session

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func msg(seq int, role Role, content string) Message {
	return Message{ID: string(rune('a' + seq)), Seq: seq, Role: role, Content: content}
}

func TestModelMessagesWithoutCompactionSkipsNonConversationRoles(t *testing.T) {
	s := &Session{Messages: []Message{
		msg(1, RoleSystem, "legacy-sys"),
		msg(2, Role("developer"), "injected-dev"),
		msg(3, Role(""), "corrupted-empty-role"),
		msg(4, RoleUser, "hi"),
		msg(5, RoleAssistant, "hello"),
		msg(6, RoleTool, "tool-out"),
	}}
	view := s.ModelMessages()
	if len(view) != 3 {
		t.Fatalf("view len = %d, want 3", len(view))
	}
	if view[0].Content != "hi" || view[1].Content != "hello" || view[2].Content != "tool-out" {
		t.Fatalf("view = %+v", view)
	}
	after := MessagesAfterSeq(s, 1)
	if len(after) != 3 {
		t.Fatalf("MessagesAfterSeq len = %d, want 3: %+v", len(after), after)
	}
}

func TestModelMessagesUsesLatestSummaryAndDropsLegacySystem(t *testing.T) {
	s := &Session{
		Messages: []Message{
			msg(1, RoleSystem, "sys1"),
			msg(2, RoleSystem, "sys2"),
			msg(3, RoleUser, "first objective"),
			msg(4, RoleAssistant, "old answer"),
			msg(5, RoleUser, "recent question"),
			msg(6, RoleAssistant, "recent answer"),
		},
		Compactions: []Compaction{{
			ID:            "cmp1",
			Seq:           7,
			CoversFromSeq: 3,
			CoversToSeq:   4,
			Summary:       summaryMarker + "\nsummary\n</compressed_history>",
		}},
		LatestCompactionID: "cmp1",
	}
	view := s.ModelMessages()
	if len(view) != 3 {
		t.Fatalf("view len = %d: %+v", len(view), view)
	}
	if !IsSummary(view[0]) || !strings.Contains(view[0].Content, "summary") {
		t.Fatalf("summary missing: %+v", view[0])
	}
	if view[1].Content != "recent question" || view[2].Content != "recent answer" {
		t.Fatalf("tail = %+v", view[1:])
	}
	for _, message := range view {
		if message.Role == RoleSystem || message.Content == "first objective" || message.Content == "old answer" {
			t.Fatalf("unexpected message leaked into model view: %+v", message)
		}
	}
	display := s.DisplayMessages()
	if len(display) != 4 {
		t.Fatalf("display len = %d", len(display))
	}
	for _, message := range display {
		if IsSummary(message) || message.Role == RoleSystem {
			t.Fatalf("non-conversation message leaked into display: %+v", message)
		}
	}
}

func TestSplitTurns(t *testing.T) {
	prefix, turns := SplitTurns([]Message{
		msg(1, RoleAssistant, "orphan"),
		msg(2, RoleUser, "u1"),
		msg(3, RoleAssistant, "a1"),
		msg(4, RoleTool, "t1"),
		msg(5, RoleUser, "u2"),
		msg(6, RoleAssistant, "a2"),
	})
	if len(prefix) != 1 || prefix[0].Content != "orphan" {
		t.Fatalf("prefix = %+v", prefix)
	}
	if len(turns) != 2 {
		t.Fatalf("turns = %+v", turns)
	}
	if turns[0].Start != 1 || turns[0].End != 4 || len(turns[0].Messages) != 3 {
		t.Fatalf("turn0 = %+v", turns[0])
	}
	if turns[1].Start != 4 || turns[1].End != 6 || len(turns[1].Messages) != 2 {
		t.Fatalf("turn1 = %+v", turns[1])
	}
}

func TestPlanCompactionKeepsWholeTailTurns(t *testing.T) {
	raw := []Message{
		msg(2, RoleUser, strings.Repeat("old ", 500)),
		msg(3, RoleAssistant, strings.Repeat("answer ", 500)),
		msg(4, RoleUser, "latest"),
		msg(5, RoleAssistant, "latest answer"),
		msg(6, RoleTool, "tool output"),
	}
	none := PlanCompaction(PlanInput{
		PriorSummary:     raw,
		RetainTailTokens: 1000000,
	}, HeuristicEstimator{})
	if none.Feasible {
		t.Fatal("nothing should be compacted when every turn fits the tail")
	}

	plan := PlanCompaction(PlanInput{
		PriorSummary:     raw,
		RetainTailTokens: 20,
	}, HeuristicEstimator{})
	if !plan.Feasible || len(plan.Tail) != 3 {
		t.Fatalf("tail = %d plan=%+v", len(plan.Tail), plan)
	}
	if plan.Tail[0].Role != RoleUser || plan.Tail[0].Content != "latest" {
		t.Fatalf("tail must start at a real user: %+v", plan.Tail)
	}
	if len(plan.Middle) != 2 {
		t.Fatalf("middle = %+v", plan.Middle)
	}
}

func TestPlanKeepsOversizedLatestTurn(t *testing.T) {
	raw := []Message{
		msg(2, RoleUser, "old"),
		msg(3, RoleAssistant, "old answer"),
		msg(4, RoleUser, strings.Repeat("latest ", 5000)),
		msg(5, RoleAssistant, "latest answer"),
	}
	plan := PlanCompaction(PlanInput{PriorSummary: raw, RetainTailTokens: 1}, HeuristicEstimator{})
	if !plan.Feasible {
		t.Fatal("plan should be feasible")
	}
	if len(plan.Tail) != 2 || plan.Tail[0].Content != raw[2].Content {
		t.Fatalf("latest turn not preserved: %+v", plan.Tail)
	}
}

func TestPlanNoUserTurnKeepsEmptyTail(t *testing.T) {
	plan := PlanCompaction(PlanInput{
		PriorSummary:     []Message{msg(1, RoleAssistant, "orphan"), msg(2, RoleTool, "out")},
		RetainTailTokens: 100,
	}, HeuristicEstimator{})
	if !plan.Feasible || len(plan.Tail) != 0 || len(plan.Middle) != 2 {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestTokenEstimatorSeparatesNaturalAndStructured(t *testing.T) {
	est := HeuristicEstimator{}
	natural := est.EstimateText("hello world this is natural language", ContentKindNatural)
	structured := est.EstimateText("{\"a\":1,\"b\":2,\"c\":3}", ContentKindStructured)
	if natural <= 0 || structured <= 0 {
		t.Fatalf("natural=%d structured=%d", natural, structured)
	}
	if est.EstimateText("服务器负载很高", ContentKindNatural) <= 0 {
		t.Fatal("CJK estimate must be positive")
	}
}

func TestEstimateForTriggerUsesAnchorDelta(t *testing.T) {
	s := &Session{
		Messages: []Message{
			msg(1, RoleUser, "old"),
			msg(2, RoleAssistant, "answer"),
			msg(3, RoleUser, "new"),
		},
		TokenAnchor: TokenAnchor{TotalTokens: 1000, AfterSeq: 2, Valid: true},
	}
	est := HeuristicEstimator{}
	got := EstimateForTrigger(s, est)
	want := 1000 + est.EstimateMessages([]Message{s.Messages[2]})
	if got != want {
		t.Fatalf("got %d want %d", got, want)
	}
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type seqIDs struct{ n int }

func (g *seqIDs) next(prefix string) string {
	g.n++
	return prefix + string(rune('0'+g.n))
}
func (g *seqIDs) NewSessionID(time.Time) string    { g.n++; return "s1" }
func (g *seqIDs) NewMessageID(time.Time) string    { return g.next("m") }
func (g *seqIDs) NewCompactionID(time.Time) string { return g.next("c") }

type fakeCompactor struct {
	requests []CompactionRequest
	summary  string
	model    string
	usage    Usage
	err      error
}

func (f *fakeCompactor) Compact(_ context.Context, req CompactionRequest) (CompactionResult, error) {
	f.requests = append(f.requests, req)
	return CompactionResult{Summary: f.summary, Model: f.model, Usage: f.usage}, f.err
}

func newTestManager(t *testing.T, compactor Compactor, opts Settings) *Manager {
	t.Helper()
	return NewManager(ManagerOptions{
		Repository:  NewInMemoryRepository(),
		Compactor:   compactor,
		Clock:       fixedClock{now: time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC)},
		IDGenerator: &seqIDs{},
		Settings:    opts,
	})
}

func TestServiceCompactsAndResetsAnchor(t *testing.T) {
	compactor := &fakeCompactor{
		summary: summaryMarker + "\nkept facts\n</compressed_history>",
		model:   "p/summary-m",
		usage:   Usage{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120},
	}
	manager := newTestManager(t, compactor, Settings{
		ContextWindowLimit:     100,
		RetainTailTokens:       1,
		CompactionTriggerRatio: 0.5,
		CompactionEnabled:      true,
	})
	ctx := context.Background()
	if _, err := manager.Create(ctx); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := manager.AppendUser(ctx, strings.Repeat("old question ", 200)); err != nil {
		t.Fatalf("AppendUser: %v", err)
	}
	if _, err := manager.AppendAssistant(ctx, strings.Repeat("old answer ", 200), nil, &Usage{TotalTokens: 5000}, "p/m"); err != nil {
		t.Fatalf("AppendAssistant: %v", err)
	}
	if err := manager.RecordResponseUsage(ctx, &Usage{TotalTokens: 5000}); err != nil {
		t.Fatalf("AfterResponse: %v", err)
	}
	if _, err := manager.AppendUser(ctx, "latest question"); err != nil {
		t.Fatalf("AppendUser: %v", err)
	}

	if err := manager.CompactIfNeeded(ctx); err != nil {
		t.Fatalf("BeforeCall: %v", err)
	}
	if len(compactor.requests) != 1 {
		t.Fatalf("summarize calls = %d", len(compactor.requests))
	}
	s := manager.Current()
	if len(s.Compactions) != 1 {
		t.Fatalf("compactions = %+v", s.Compactions)
	}
	if s.Compactions[0].Model != "p/summary-m" {
		t.Fatalf("compaction model = %q, want p/summary-m", s.Compactions[0].Model)
	}
	if s.TokenAnchor.Valid {
		t.Fatal("token anchor must be reset after compaction")
	}
	if s.TokenUsage.TotalTokens != 5120 {
		t.Fatalf("token usage = %+v", s.TokenUsage)
	}
	view := s.ModelMessages()
	if !IsSummary(view[0]) {
		t.Fatalf("model view missing summary: %+v", view)
	}
	foundOld := false
	for _, message := range s.DisplayMessages() {
		if strings.Contains(message.Content, "old question") {
			foundOld = true
		}
	}
	if !foundOld {
		t.Fatal("display view must keep compressed raw messages")
	}
}

func TestServiceSecondCompactionCarriesPriorSummary(t *testing.T) {
	compactor := &fakeCompactor{summary: summaryMarker + "\nsummary\n</compressed_history>"}
	manager := newTestManager(t, compactor, Settings{
		ContextWindowLimit:     100,
		RetainTailTokens:       1,
		CompactionTriggerRatio: 0.1,
		CompactionEnabled:      true,
	})
	ctx := context.Background()
	_, _ = manager.Create(ctx)
	turn := func(user, answer string) {
		_, _ = manager.AppendUser(ctx, user)
		_, _ = manager.AppendAssistant(ctx, answer, nil, nil, "p/m")
		_ = manager.RecordResponseUsage(ctx, nil)
	}
	turn(strings.Repeat("first ", 200), strings.Repeat("answer ", 200))
	turn(strings.Repeat("second ", 200), strings.Repeat("answer ", 200))
	_ = manager.CompactIfNeeded(ctx)
	turn(strings.Repeat("third ", 200), strings.Repeat("answer ", 200))
	_ = manager.CompactIfNeeded(ctx)

	if len(compactor.requests) < 2 {
		t.Fatalf("summarize calls = %d", len(compactor.requests))
	}
	if compactor.requests[1].PriorSummary == "" {
		t.Fatal("second compaction must carry the prior summary")
	}
	s := manager.Current()
	if len(s.Compactions) < 2 {
		t.Fatalf("compactions = %+v", s.Compactions)
	}
	last := s.Compactions[len(s.Compactions)-1]
	if last.PreviousCompactionID == "" {
		t.Fatal("last compaction must reference the prior compaction")
	}
}

func TestServiceDerivesTitle(t *testing.T) {
	manager := newTestManager(t, nil, Settings{})
	ctx := context.Background()
	_, _ = manager.Create(ctx)
	_, _ = manager.AppendUser(ctx, "帮我排查 nginx 502 错误，先看 upstream 和后端服务状态")
	_, _ = manager.AppendAssistant(ctx, "先检查后端服务。", nil, nil, "p/m")
	if err := manager.EnsureTitle(ctx); err != nil {
		t.Fatalf("EnsureTitle: %v", err)
	}
	want := "帮我排查nginx502错误先看upstream和后端服务状态"
	if got := manager.Current().Title; got != want {
		t.Fatalf("title = %q, want %q", got, want)
	}
}

func TestCompactIfNeededPropagatesCancellation(t *testing.T) {
	interruptErr := errors.New("interrupted by user")
	compactor := &fakeCompactor{
		err: errors.Join(interruptErr, context.Canceled),
	}
	var callbackErr error
	manager := NewManager(ManagerOptions{
		Repository:  NewInMemoryRepository(),
		Compactor:   compactor,
		Clock:       fixedClock{now: time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC)},
		IDGenerator: &seqIDs{},
		Settings: Settings{
			ContextWindowLimit:     100,
			RetainTailTokens:       1,
			CompactionTriggerRatio: 0.5,
			CompactionEnabled:      true,
		},
		OnCompactionError: func(err error) { callbackErr = err },
	})
	ctx := context.Background()
	_, _ = manager.Create(ctx)
	_, _ = manager.AppendUser(ctx, strings.Repeat("old question ", 200))
	_, _ = manager.AppendAssistant(ctx, strings.Repeat("old answer ", 200), nil, &Usage{TotalTokens: 5000}, "p/m")
	_ = manager.RecordResponseUsage(ctx, &Usage{TotalTokens: 5000})
	_, _ = manager.AppendUser(ctx, "latest question")

	err := manager.CompactIfNeeded(ctx)
	if !errors.Is(err, interruptErr) || !errors.Is(err, context.Canceled) {
		t.Fatalf("CompactIfNeeded err = %v, want joined interrupt + context.Canceled", err)
	}
	if callbackErr != nil {
		t.Fatalf("OnCompactionError must not fire on cancellation, got %v", callbackErr)
	}
}
