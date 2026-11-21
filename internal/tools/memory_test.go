package tools

import (
	"strings"
	"testing"
)

func TestMemoryToolDefinitions(t *testing.T) {
	search := MemorySearchDefinition()
	if search.Name != "memory_search" || len(search.InputSchema) == 0 {
		t.Fatalf("search definition = %+v", search)
	}
	if !strings.Contains(search.Description, "<memory_catalog>") || !strings.Contains(search.Description, "full content") {
		t.Fatalf("search description missing catalog/full-content guidance: %s", search.Description)
	}
	save := MemorySaveDefinition()
	if save.Name != "memory_save" || !strings.Contains(save.Description, "Never save secrets") {
		t.Fatalf("save definition = %+v", save)
	}
	if !strings.Contains(save.Description, "append adds to the existing body") {
		t.Fatalf("description missing append rule: %s", save.Description)
	}
}

func TestParseMemorySearchArgs(t *testing.T) {
	args, err := ParseMemorySearchArgs(`{"query":" prod-k8s 10.0.0.1 ","limit":50}`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if args.Query != "prod-k8s 10.0.0.1" || args.MaxResults != 20 {
		t.Fatalf("args = %+v", args)
	}
	if _, err := ParseMemorySearchArgs(`{"query":""}`); err == nil {
		t.Fatal("expected empty query error")
	}
}

func TestParseMemorySaveArgs(t *testing.T) {
	args, err := ParseMemorySaveArgs(`{"name":"prod-k8s","content":"- fact","mode":"REPLACE"}`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if args.Operation != "replace" {
		t.Fatalf("mode = %q", args.Operation)
	}
	if _, err := ParseMemorySaveArgs(`{"name":"prod","content":"- fact","mode":"delete"}`); err == nil {
		t.Fatal("expected unsupported mode error")
	}
	if _, err := ParseMemorySaveArgs(`{"name":"","content":"- fact"}`); err == nil {
		t.Fatal("expected missing name error")
	}
}
