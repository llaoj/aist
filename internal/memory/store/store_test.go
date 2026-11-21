package store

import (
	"context"
	"testing"

	"github.com/llaoj/aist/internal/memory"
)

type fakeLoader struct {
	result memory.LoadResult
}

func (f fakeLoader) Load(context.Context) (memory.LoadResult, error) {
	return f.result, nil
}

func TestStoreReloadAndSnapshot(t *testing.T) {
	loader := fakeLoader{result: memory.LoadResult{
		Memories: []memory.Entry{
			{Name: "prod", Content: "- first"},
			{Name: "prod", Content: "- second"},
		},
		Report: memory.Report{Loaded: 2},
	}}
	store := NewSnapshot(loader)
	if err := store.Reload(context.Background()); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	snapshot := store.Snapshot()
	if len(snapshot) != 2 {
		t.Fatalf("snapshot = %d", len(snapshot))
	}
	snapshot[0].Name = "mutated"
	if store.Snapshot()[0].Name != "prod" {
		t.Fatal("snapshot mutation leaked into store")
	}
	if store.Report().Loaded != 2 {
		t.Fatalf("report = %+v", store.Report())
	}

	mem, ok := store.Lookup("prod")
	if !ok || mem.Content != "- second" {
		t.Fatalf("Get = %+v, %v", mem, ok)
	}
	if _, ok := store.Lookup("missing"); ok {
		t.Fatal("unexpected missing match")
	}
}
