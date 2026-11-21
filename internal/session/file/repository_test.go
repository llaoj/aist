package file

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/llaoj/aist/internal/session"
)

func newRepo(t *testing.T) (*Repository, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), ".aist")
	repo := NewRepository(RepositoryOptions{Root: root})
	return repo, root
}

func newSession(id string) *session.Session {
	now := time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC)
	return &session.Session{
		ID:            id,
		Title:         "",
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: 1,
	}
}

func TestRepositoryCreateAppendLoadListDelete(t *testing.T) {
	repo, root := newRepo(t)
	ctx := context.Background()
	s := newSession("20261031T100000Z-aaaa")
	s.Messages = append(s.Messages, session.Message{
		ID: "m1", Seq: 1, Time: s.CreatedAt, Role: session.RoleSystem, Content: "sys",
	})
	if err := repo.Create(ctx, s); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.AppendMessage(ctx, s.ID, session.Message{
		ID: "m2", Seq: 2, Time: s.CreatedAt, Role: session.RoleUser, Content: "hi",
	}); err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	if err := repo.AppendMessage(ctx, s.ID, session.Message{
		ID: "m3", Seq: 3, Time: s.CreatedAt, Role: session.RoleAssistant, Content: "hello",
		Usage: &session.Usage{PromptTokens: 10, CompletionTokens: 2, TotalTokens: 12},
	}); err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	s.Messages = append(s.Messages,
		session.Message{ID: "m2", Seq: 2, Time: s.CreatedAt, Role: session.RoleUser, Content: "hi"},
		session.Message{
			ID: "m3", Seq: 3, Time: s.CreatedAt, Role: session.RoleAssistant, Content: "hello",
			Usage: &session.Usage{PromptTokens: 10, CompletionTokens: 2, TotalTokens: 12},
		},
	)
	s.TokenUsage = session.Usage{TotalTokens: 12}
	s.TokenAnchor = session.TokenAnchor{TotalTokens: 12, AfterSeq: 3, Valid: true}
	if err := repo.SaveMetadata(ctx, s); err != nil {
		t.Fatalf("SaveMetadata: %v", err)
	}

	loaded, err := repo.Load(ctx, s.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Messages) != 3 || loaded.TokenAnchor.TotalTokens != 12 {
		t.Fatalf("loaded = %+v", loaded)
	}
	if loaded.Messages[2].Usage == nil || loaded.Messages[2].Usage.TotalTokens != 12 {
		t.Fatalf("usage not loaded: %+v", loaded.Messages[2])
	}

	summaries, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(summaries) != 1 || summaries[0].MessageCount != 3 {
		t.Fatalf("summaries = %+v", summaries)
	}

	dir := filepath.Join(root, "sessions", s.ID)
	for _, name := range []string{"session.json", "messages.jsonl"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("Stat %s: %v", name, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode = %o", name, info.Mode().Perm())
		}
	}

	if err := repo.Delete(ctx, s.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("session dir still exists: %v", err)
	}
}

func TestRepositoryCompactionDerivesCompressedBy(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	s := newSession("20261031T100000Z-bbbb")
	if err := repo.Create(ctx, s); err != nil {
		t.Fatalf("Create: %v", err)
	}
	for i := 1; i <= 4; i++ {
		role := session.RoleUser
		if i%2 == 0 {
			role = session.RoleAssistant
		}
		if err := repo.AppendMessage(ctx, s.ID, session.Message{ID: "id" + string(rune('0'+i)), Seq: i, Time: s.CreatedAt, Role: role, Content: "m"}); err != nil {
			t.Fatalf("AppendMessage: %v", err)
		}
	}
	compaction := session.Compaction{
		ID: "cmp1", Seq: 5, Time: s.CreatedAt, CoversFromSeq: 2, CoversToSeq: 3,
		Summary: `<compressed_history priority="high">` + "\nsum\n</compressed_history>",
		Model:   "p/summary-m",
		Usage:   &session.Usage{TotalTokens: 9},
	}
	if err := repo.AppendCompaction(ctx, s.ID, compaction); err != nil {
		t.Fatalf("AppendCompaction: %v", err)
	}
	loaded, err := repo.Load(ctx, s.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, message := range loaded.Messages {
		covered := message.Seq >= 2 && message.Seq <= 3
		if covered && message.CoveredByCompactionID != "cmp1" {
			t.Fatalf("message %d not marked: %+v", message.Seq, message)
		}
		if !covered && message.CoveredByCompactionID != "" {
			t.Fatalf("message %d wrongly marked: %+v", message.Seq, message)
		}
	}
	if len(loaded.Compactions) != 1 || loaded.LatestCompactionID != "cmp1" || loaded.Compactions[0].Model != "p/summary-m" {
		t.Fatalf("compactions = %+v", loaded.Compactions)
	}
}

func TestRepositoryIgnoresIncompleteLastLine(t *testing.T) {
	repo, root := newRepo(t)
	ctx := context.Background()
	s := newSession("20261031T100000Z-cccc")
	if err := repo.Create(ctx, s); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.AppendMessage(ctx, s.ID, session.Message{ID: "m1", Seq: 1, Time: s.CreatedAt, Role: session.RoleUser, Content: "ok"}); err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	path := filepath.Join(root, "sessions", s.ID, "messages.jsonl")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	_, _ = file.WriteString(`{"type":"message","id":"broken","seq":2`)
	_ = file.Close()

	loaded, err := repo.Load(ctx, s.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Messages) != 1 || loaded.Messages[0].ID != "m1" {
		t.Fatalf("messages = %+v", loaded.Messages)
	}
}

func TestRepositoryRejectsDamagedMiddleLine(t *testing.T) {
	repo, root := newRepo(t)
	ctx := context.Background()
	s := newSession("20261031T100000Z-dddd")
	if err := repo.Create(ctx, s); err != nil {
		t.Fatalf("Create: %v", err)
	}
	path := filepath.Join(root, "sessions", s.ID, "messages.jsonl")
	data := "{\"type\":\"message\",\"id\":\"m1\",\"seq\":1,\"role\":\"user\",\"content\":\"a\"}\nnot-json\n{\"type\":\"message\",\"id\":\"m2\",\"seq\":2,\"role\":\"user\",\"content\":\"b\"}\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := repo.Load(ctx, s.ID); err == nil {
		t.Fatal("expected damaged middle line error")
	}
}

func TestRepositoryPersistsToolStatus(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	s := newSession("20261031T100000Z-abcd")
	if err := repo.Create(ctx, s); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.AppendMessage(ctx, s.ID, session.Message{
		ID: "tool1", Seq: 1, Time: s.CreatedAt, Role: session.RoleTool,
		Content: "full output", ToolCallID: "call1", ToolOutcome: session.ToolOutcomeFailed,
	}); err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	loaded, err := repo.Load(ctx, s.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Messages) != 1 || loaded.Messages[0].ToolOutcome != session.ToolOutcomeFailed {
		t.Fatalf("tool status not persisted: %+v", loaded.Messages)
	}
}

func TestRepositoryRebuildsDamagedMetadataFromEvents(t *testing.T) {
	repo, root := newRepo(t)
	ctx := context.Background()
	s := newSession("20261031T100000Z-ffff")
	if err := repo.Create(ctx, s); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.AppendMessage(ctx, s.ID, session.Message{
		ID: "m1", Seq: 1, Time: s.CreatedAt, Role: session.RoleUser, Content: "hello",
	}); err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	metaPath := filepath.Join(root, "sessions", s.ID, "session.json")
	if err := os.WriteFile(metaPath, []byte("{broken"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	loaded, err := repo.Load(ctx, s.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.ID != s.ID || len(loaded.Messages) != 1 || loaded.Messages[0].Content != "hello" {
		t.Fatalf("rebuilt = %+v", loaded)
	}
}

func TestRepositoryRebuildsMissingTokenUsage(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	s := newSession("20261031T100000Z-eeee")
	if err := repo.Create(ctx, s); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.AppendMessage(ctx, s.ID, session.Message{
		ID: "m1", Seq: 1, Time: s.CreatedAt, Role: session.RoleAssistant, Content: "a",
		Usage: &session.Usage{PromptTokens: 3, CompletionTokens: 4, TotalTokens: 7},
	}); err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	if err := repo.AppendCompaction(ctx, s.ID, session.Compaction{
		ID: "cmp", Seq: 2, CoversFromSeq: 1, CoversToSeq: 1,
		Summary: `<compressed_history priority="high">x</compressed_history>`,
		Usage:   &session.Usage{TotalTokens: 5},
	}); err != nil {
		t.Fatalf("AppendCompaction: %v", err)
	}
	loaded, err := repo.Load(ctx, s.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.TokenUsage.TotalTokens != 12 {
		t.Fatalf("usage = %+v", loaded.TokenUsage)
	}
	if !strings.Contains(loaded.Messages[0].Content, "a") {
		t.Fatalf("messages = %+v", loaded.Messages)
	}
}

func TestRepositoryConcurrentAppendKeepsWholeEvents(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	s := newSession("20261031T100000Z-concurrent")
	if err := repo.Create(ctx, s); err != nil {
		t.Fatalf("Create: %v", err)
	}

	const count = 32
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := repo.AppendMessage(ctx, s.ID, session.Message{
				ID:      fmt.Sprintf("m%d", i),
				Seq:     i + 1,
				Time:    s.CreatedAt,
				Role:    session.RoleUser,
				Content: fmt.Sprintf("message-%d", i),
			})
			if err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("AppendMessage: %v", err)
	}
	if t.Failed() {
		return
	}

	loaded, err := repo.Load(ctx, s.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Messages) != count {
		t.Fatalf("messages = %d, want %d", len(loaded.Messages), count)
	}
	seen := make(map[string]bool, count)
	for _, message := range loaded.Messages {
		if !strings.HasPrefix(message.Content, "message-") {
			t.Fatalf("corrupted message = %q", message.Content)
		}
		seen[message.ID] = true
	}
	if len(seen) != count {
		t.Fatalf("unique messages = %d, want %d", len(seen), count)
	}
}

func TestServiceRetainsOnlyNewestSessions(t *testing.T) {
	repo, _ := newRepo(t)
	manager := session.NewManager(session.ManagerOptions{
		Repository: repo,
		Settings: session.Settings{
			RetainSessions:    2,
			CompactionEnabled: false,
		},
	})
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := manager.Create(ctx, "system"); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}
	summaries, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("summaries = %+v", summaries)
	}
}
