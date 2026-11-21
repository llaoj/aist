package memory

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeStore struct {
	memories    []Entry
	report      Report
	reloadCount int
}

func (f *fakeStore) Snapshot() []Entry { return append([]Entry(nil), f.memories...) }
func (f *fakeStore) Report() Report    { return f.report }
func (f *fakeStore) Lookup(name string) (Entry, bool) {
	for i := len(f.memories) - 1; i >= 0; i-- {
		if f.memories[i].Name == name {
			return f.memories[i], true
		}
	}
	return Entry{}, false
}
func (f *fakeStore) Reload(context.Context) error {
	f.reloadCount++
	return nil
}

type fakeWriter struct {
	appended  bool
	replaced  bool
	target    WriteDestination
	content   WriteContent
	operation string
}

func (f *fakeWriter) Append(_ context.Context, target WriteDestination, content WriteContent) (Entry, error) {
	f.appended = true
	f.operation = "append"
	f.target = target
	f.content = content
	return fakeWrittenMemory(target, content), nil
}

func (f *fakeWriter) Upsert(_ context.Context, target WriteDestination, content WriteContent) (Entry, error) {
	f.replaced = true
	f.operation = "replace"
	f.target = target
	f.content = content
	return fakeWrittenMemory(target, content), nil
}

func fakeWrittenMemory(target WriteDestination, content WriteContent) Entry {
	mem := Entry{
		Name:    target.Name,
		Path:    target.Path,
		Content: content.Content,
		Summary: content.Summary,
		Type:    content.Type,
		Source:  content.Source,
	}
	if mem.Summary == "" {
		mem.Summary = DeriveSummary(mem, DefaultSummaryLimit)
	}
	return mem
}

type fakeNotifier struct {
	results []ChangeResult
}

func (f *fakeNotifier) MemoryChanged(_ context.Context, result ChangeResult) {
	f.results = append(f.results, result)
}

func newTestManager(snapshot SnapshotStore, writer Writer, notifier Notifier) *Manager {
	return NewManager(ManagerOptions{
		Snapshot: snapshot,
		Writer:   writer,
		Notifier: notifier,
		MaxBytes: 4096,
	})
}

func TestServiceAppendCreatesWithoutConfirmation(t *testing.T) {
	snapshot := &fakeStore{}
	writer := &fakeWriter{}
	notifier := &fakeNotifier{}
	manager := newTestManager(snapshot, writer, notifier)

	result, err := manager.Change(context.Background(), ChangeRequest{
		Name:    "prod-k8s",
		Content: "- master: 10.0.0.1",
		Summary: "Kubernetes cluster",
		Type:    "k8s",
		Source:  "user",
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !writer.appended || writer.replaced {
		t.Fatalf("writer operation = %q", writer.operation)
	}
	if result.Action != ChangeActionCreated || result.Operation != ChangeOperationAppend {
		t.Fatalf("result = %+v", result)
	}
	if len(notifier.results) != 1 {
		t.Fatalf("notifications = %d", len(notifier.results))
	}
	if snapshot.reloadCount != 1 {
		t.Fatalf("reload count = %d", snapshot.reloadCount)
	}
}

func TestServiceAppendUsesExistingPath(t *testing.T) {
	snapshot := &fakeStore{memories: []Entry{{Name: "prod", Path: "/project/.aist/memory/prod.md"}}}
	writer := &fakeWriter{}
	manager := newTestManager(snapshot, writer, &fakeNotifier{})

	if _, err := manager.Change(context.Background(), ChangeRequest{Name: "prod", Content: "- new fact"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if writer.target.Path != "/project/.aist/memory/prod.md" {
		t.Fatalf("target path = %q", writer.target.Path)
	}
}

func TestServiceReplaceNeedsNoConfirmation(t *testing.T) {
	snapshot := &fakeStore{memories: []Entry{{Name: "prod", Path: "/m/prod.md", Content: "- old"}}}
	writer := &fakeWriter{}
	notifier := &fakeNotifier{}
	manager := newTestManager(snapshot, writer, notifier)

	result, err := manager.Change(context.Background(), ChangeRequest{Name: "prod", Content: "- new", Operation: ChangeOperationReplace})
	if err != nil {
		t.Fatalf("Change: %v", err)
	}
	if !writer.replaced || result.Action != ChangeActionReplaced {
		t.Fatalf("replace result = %+v, writer=%+v", result, writer)
	}
	if len(notifier.results) != 1 {
		t.Fatalf("notifications = %d", len(notifier.results))
	}
}

func TestServiceReplaceNewCreatesWithoutConfirmation(t *testing.T) {
	snapshot := &fakeStore{}
	writer := &fakeWriter{}
	manager := newTestManager(snapshot, writer, &fakeNotifier{})

	result, err := manager.Change(context.Background(), ChangeRequest{Name: "new", Content: "- fact", Operation: ChangeOperationReplace})
	if err != nil {
		t.Fatalf("Change: %v", err)
	}
	if !writer.replaced || result.Action != ChangeActionCreated {
		t.Fatalf("result = %+v", result)
	}
}

func TestServiceRejectsSecretsAndStripsPrivateBlocks(t *testing.T) {
	writer := &fakeWriter{}
	manager := newTestManager(&fakeStore{}, writer, &fakeNotifier{})

	_, err := manager.Change(context.Background(), ChangeRequest{
		Name:    "prod",
		Content: "- master: 10.0.0.1\npassword: hunter2",
	})
	if !errors.Is(err, ErrSensitiveContent) {
		t.Fatalf("err = %v, want sensitive", err)
	}
	if writer.appended {
		t.Fatal("sensitive content must not be written")
	}

	if _, err := manager.Change(context.Background(), ChangeRequest{Name: "prod", Content: "- 密码：hunter2"}); !errors.Is(err, ErrSensitiveContent) {
		t.Fatalf("chinese secret err = %v", err)
	}

	result, err := manager.Change(context.Background(), ChangeRequest{
		Name:    "prod",
		Content: "- visible fact\n<private>hidden secret</private>",
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if writer.content.Content != "- visible fact" {
		t.Fatalf("body = %q", writer.content.Content)
	}
	if result.Entry.Summary == "" {
		t.Fatal("summary should be derived or preserved")
	}
}

func TestServiceRejectsOversizedRawOutput(t *testing.T) {
	writer := &fakeWriter{}
	manager := newTestManager(&fakeStore{}, writer, &fakeNotifier{})

	_, err := manager.Change(context.Background(), ChangeRequest{
		Name:    "prod",
		Content: strings.Repeat("raw output line\n", MaxSaveContentBytes),
	})
	if err == nil {
		t.Fatal("expected oversized content error")
	}
	if writer.appended {
		t.Fatal("oversized content must not be written")
	}
}

func TestServiceSearchUsesSnapshot(t *testing.T) {
	snapshot := &fakeStore{memories: []Entry{{
		Name:    "prod-k8s",
		Content: "- master: 10.0.0.1",
	}}}
	manager := newTestManager(snapshot, &fakeWriter{}, &fakeNotifier{})

	results, err := manager.Search(context.Background(), "10.0.0.1", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 || results[0].Name != "prod-k8s" {
		t.Fatalf("results = %+v", results)
	}
}

func TestServiceRejectsInvalidMode(t *testing.T) {
	manager := newTestManager(&fakeStore{}, &fakeWriter{}, &fakeNotifier{})
	for _, mode := range []ChangeOperation{"bogus", "delete"} {
		if _, err := manager.Change(context.Background(), ChangeRequest{Name: "prod", Content: "- fact", Operation: mode}); err == nil {
			t.Fatalf("expected invalid mode error for %q", mode)
		}
	}
}
