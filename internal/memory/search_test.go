package memory

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestSearchMatchesIPAndReturnsMultilineContent(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	body := "- master: k8s-master-01 (192.0.2.1)\n- workers: k8s-node-01, k8s-node-02\n- kubeconfig: ~/.kube/config"
	memories := []Entry{
		{
			Name:     "prod-k8s",
			Type:     "k8s",
			Summary:  "Kubernetes cluster",
			Path:     "/m/k8s.md",
			Content:  body,
			Modified: now,
		},
		{
			Name:     "prod-mysql",
			Type:     "db",
			Summary:  "MySQL primary",
			Path:     "/m/mysql.md",
			Content:  "- primary: 10.2.0.5",
			Modified: now.Add(-time.Hour),
		},
	}

	results := Search(memories, "192.0.2.1", SearchOptions{})
	if len(results) != 1 {
		t.Fatalf("results = %+v", results)
	}
	if results[0].Name != "prod-k8s" {
		t.Fatalf("name = %q", results[0].Name)
	}
	if results[0].Content != body {
		t.Fatalf("content = %q, want full multiline body %q", results[0].Content, body)
	}
	if results[0].Truncated {
		t.Fatal("expected Truncated=false for normal entry")
	}
}

func TestSearchByCatalogNameReturnsFullContentAndPrioritizesExactMatch(t *testing.T) {
	body := "- master: 10.0.0.1\n- node-01: 10.0.0.2\n- node-02: 10.0.0.3"
	memories := []Entry{
		{Name: "prod-k8s-staging", Type: "k8s", Summary: "staging cluster", Content: "- master: 10.1.0.1"},
		{Name: "prod-k8s", Type: "k8s", Summary: "Kubernetes cluster", Content: body},
		{Name: "notes", Summary: "misc", Content: "- prod-k8s is mentioned in a note"},
	}

	for _, query := range []string{"prod-k8s", "prod-k8s.md"} {
		results := Search(memories, query, SearchOptions{})
		if len(results) == 0 {
			t.Fatalf("query %q returned no results", query)
		}
		if results[0].Name != "prod-k8s" {
			t.Fatalf("query %q ranked %q first, want exact match prod-k8s", query, results[0].Name)
		}
		if results[0].Content != body {
			t.Fatalf("query %q content = %q, want %q", query, results[0].Content, body)
		}
		if results[0].Truncated {
			t.Fatalf("query %q unexpectedly truncated short body", query)
		}
	}
}

func TestSearchByTypeStillReturnsBody(t *testing.T) {
	memories := []Entry{
		{
			Name:    "cluster-a",
			Type:    "hadoop",
			Summary: "primary analytics cluster",
			Content: "- namenode: 10.10.0.1\n- datanode-01: 10.10.0.2",
		},
	}

	results := Search(memories, "hadoop", SearchOptions{})
	if len(results) != 1 {
		t.Fatalf("results = %+v", results)
	}
	if !strings.Contains(results[0].Content, "- namenode: 10.10.0.1\n- datanode-01: 10.10.0.2") {
		t.Fatalf("content = %q", results[0].Content)
	}
}

func TestSearchLongEntryWithinBudgetReturnsFullBody(t *testing.T) {
	var lines []string
	for i := 1; i <= 30; i++ {
		lines = append(lines, fmt.Sprintf("- node-%02d: 192.0.2.%d (worker role)", i, i))
	}
	body := strings.Join(lines, "\n")
	if len(body) <= 320 || len(body) > defaultMaxEntryRunes {
		t.Fatalf("unexpected test body length: %d", len(body))
	}

	results := Search([]Entry{{Name: "prod-k8s", Content: body}}, "prod-k8s", SearchOptions{})
	if len(results) != 1 {
		t.Fatalf("results = %+v", results)
	}
	if results[0].Content != body || results[0].Truncated {
		t.Fatalf("got truncated=%v content len=%d, want full len=%d", results[0].Truncated, len(results[0].Content), len(body))
	}
}

func TestSearchOversizedEntryExtractsMatchedLinesAndMarksTruncated(t *testing.T) {
	var lines []string
	for i := 1; i <= 60; i++ {
		lines = append(lines, fmt.Sprintf("- node-%02d: 192.0.2.%d", i, i))
	}
	body := strings.Join(lines, "\n")

	results := Search([]Entry{{Name: "prod-k8s", Content: body}}, "node-45", SearchOptions{MaxEntryRunes: 200})
	if len(results) != 1 {
		t.Fatalf("results = %+v", results)
	}
	if !results[0].Truncated {
		t.Fatal("expected Truncated=true for oversized entry")
	}
	for _, want := range []string{"- node-44: 192.0.2.44", "- node-45: 192.0.2.45", "- node-46: 192.0.2.46", "..."} {
		if !strings.Contains(results[0].Content, want) {
			t.Fatalf("missing %q in windowed content:\n%s", want, results[0].Content)
		}
	}
	if strings.Contains(results[0].Content, "node-01") {
		t.Fatalf("unexpected distant line in windowed content:\n%s", results[0].Content)
	}

	byName := Search([]Entry{{Name: "prod-k8s", Content: body}}, "prod-k8s", SearchOptions{MaxEntryRunes: 200})
	if len(byName) != 1 || !byName[0].Truncated {
		t.Fatalf("name search on oversized entry = %+v", byName)
	}
	if !strings.Contains(byName[0].Content, "- node-01: 192.0.2.1") {
		t.Fatalf("expected leading lines when matched by name:\n%s", byName[0].Content)
	}
}

func TestSearchLimitAndNoMatch(t *testing.T) {
	memories := []Entry{
		{Name: "a", Content: "- same"},
		{Name: "b", Content: "- same"},
		{Name: "c", Content: "- same"},
	}
	results := Search(memories, "same", SearchOptions{Limit: 2})
	if len(results) != 2 {
		t.Fatalf("limit not applied: %d", len(results))
	}
	if got := Search(memories, "missing", SearchOptions{}); len(got) != 0 {
		t.Fatalf("unexpected matches: %+v", got)
	}
}

func TestSearchRespectsMaxBytes(t *testing.T) {
	memories := []Entry{
		{Name: "a", Content: "- " + strings.Repeat("x", 100)},
		{Name: "b", Content: "- " + strings.Repeat("x", 100)},
	}
	results := Search(memories, "x", SearchOptions{Limit: 5, MaxBytes: 50})
	if len(results) != 1 {
		t.Fatalf("max bytes not applied: %d", len(results))
	}
}
