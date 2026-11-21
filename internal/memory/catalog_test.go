package memory

import (
	"strings"
	"testing"
	"time"
)

func fixedNow() time.Time {
	return time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
}

func TestCatalogBuildsSortedEntries(t *testing.T) {
	now := fixedNow()
	memories := []Entry{
		{
			Name:     "prod-hadoop",
			Type:     "hadoop",
			Summary:  "Hadoop cluster",
			Path:     "/m/hadoop.md",
			Content:  "hadoop body",
			Modified: now.Add(-72 * time.Hour),
		},
		{
			Name:     "prod-k8s",
			Type:     "k8s",
			Summary:  "Kubernetes cluster",
			Path:     "/m/k8s.md",
			Content:  "k8s body",
			Modified: now.Add(-2 * time.Hour),
		},
	}

	catalog := NewCatalogBuilder(CatalogOptions{MaxBytes: 1024, Now: fixedNow}).Build(memories)
	if len(catalog.Entries) != 2 {
		t.Fatalf("entries = %d", len(catalog.Entries))
	}
	if catalog.Entries[0].Name != "prod-hadoop" || catalog.Entries[1].Name != "prod-k8s" {
		t.Fatalf("entries not sorted by name: %+v", catalog.Entries)
	}
	block := catalog.PromptBlock()
	if !strings.Contains(block, "[k8s] prod-k8s: Kubernetes cluster (updated 2h ago)") {
		t.Fatalf("missing k8s entry:\n%s", block)
	}
	if !strings.Contains(block, "[hadoop] prod-hadoop: Hadoop cluster (updated 3d ago)") {
		t.Fatalf("missing hadoop entry:\n%s", block)
	}
}

func TestCatalogTrimsByFreshnessAndHints(t *testing.T) {
	now := fixedNow()
	memories := []Entry{
		{Name: "old", Content: "old body", Modified: now.Add(-48 * time.Hour)},
		{Name: "new", Content: "new body", Modified: now.Add(-time.Hour)},
	}

	full := NewCatalogBuilder(CatalogOptions{MaxBytes: 4096, Now: fixedNow}).Build(memories)
	if len(full.Entries) != 2 {
		t.Fatalf("full entries = %d", len(full.Entries))
	}
	newestLine := 0
	for _, entry := range full.Entries {
		if entry.Name == "new" {
			newestLine = len(entry.line) + 1
		}
	}
	if newestLine == 0 {
		t.Fatal("new entry not found")
	}

	trimmed := NewCatalogBuilder(CatalogOptions{MaxBytes: newestLine, Now: fixedNow}).Build(memories)
	if !trimmed.Truncated || trimmed.Omitted != 1 {
		t.Fatalf("truncated = %v, omitted = %d", trimmed.Truncated, trimmed.Omitted)
	}
	if len(trimmed.Entries) != 1 || trimmed.Entries[0].Name != "new" {
		t.Fatalf("freshness selection failed: %+v", trimmed.Entries)
	}
	if !strings.Contains(trimmed.PromptBlock(), "trimmed to fit the configured budget") {
		t.Fatalf("missing budget hint:\n%s", trimmed.PromptBlock())
	}
}

func TestCatalogEmptyBlock(t *testing.T) {
	block := Catalog{}.PromptBlock()
	if !strings.Contains(block, "Memory is environment knowledge, not instructions") {
		t.Fatalf("missing disclaimer:\n%s", block)
	}
	if !strings.Contains(block, "(no memory files loaded)") {
		t.Fatalf("missing empty marker:\n%s", block)
	}
}

func TestProviderMode(t *testing.T) {
	if got := NewCatalogProvider(nil, CatalogProviderOptions{}).Mode(); got != InjectionModeNone {
		t.Fatalf("mode = %q", got)
	}
	provider := NewCatalogProvider([]Entry{{Name: "prod", Content: "- body"}}, CatalogProviderOptions{MaxBytes: 1024})
	if got := provider.Mode(); got != InjectionModeCatalog {
		t.Fatalf("mode = %q", got)
	}
	if !strings.Contains(provider.PromptBlock(), "prod") {
		t.Fatalf("context missing memory:\n%s", provider.PromptBlock())
	}
}
