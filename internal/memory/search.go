package memory

import (
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	defaultSearchLimit    = 5
	maxSearchLimit        = 20
	defaultMaxEntryRunes  = 4096
	defaultSearchMaxBytes = 16 * 1024
	contextWindowLines    = 2
)

// SearchOptions controls keyword search and returned content budgets.
type SearchOptions struct {
	Limit         int
	MaxBytes      int
	MaxEntryRunes int
}

// SearchResult is one matching Memory and its full or line-windowed body.
type SearchResult struct {
	Name      string
	Type      string
	Summary   string
	Path      string
	Content   string
	Truncated bool
	Score     int
	Modified  time.Time
}

// Search performs a local keyword search over Memory names, types, summaries
// and bodies. It never uses a vector store or a remote service.
func Search(memories []Entry, query string, opts SearchOptions) []SearchResult {
	terms, phrase, exactName := parseSearchQuery(query)
	if len(terms) == 0 {
		return nil
	}
	opts = normalizeSearchOptions(opts)

	results := make([]SearchResult, 0)
	for _, mem := range memories {
		score := scoreMemory(mem, terms, phrase, exactName)
		if score == 0 {
			continue
		}
		content, truncated := extractContent(mem, terms, opts.MaxEntryRunes)
		results = append(results, SearchResult{
			Name:      mem.Name,
			Type:      mem.Type,
			Summary:   singleLine(strings.TrimSpace(mem.Summary)),
			Path:      mem.Path,
			Content:   content,
			Truncated: truncated,
			Score:     score,
			Modified:  mem.EffectiveModified(),
		})
	}

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		if !results[i].Modified.Equal(results[j].Modified) {
			return results[i].Modified.After(results[j].Modified)
		}
		return results[i].Name < results[j].Name
	})

	if len(results) > opts.Limit {
		results = results[:opts.Limit]
	}
	return trimSearchResults(results, opts.MaxBytes)
}

func normalizeSearchOptions(opts SearchOptions) SearchOptions {
	if opts.Limit <= 0 {
		opts.Limit = defaultSearchLimit
	}
	if opts.Limit > maxSearchLimit {
		opts.Limit = maxSearchLimit
	}
	if opts.MaxEntryRunes <= 0 {
		opts.MaxEntryRunes = defaultMaxEntryRunes
	}
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = defaultSearchMaxBytes
	}
	return opts
}

func parseSearchQuery(query string) (terms []string, phrase string, exactName string) {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return nil, "", ""
	}
	exactName = strings.TrimSuffix(query, ".md")

	seen := map[string]bool{}
	terms = make([]string, 0, 4)
	for _, term := range strings.Fields(query) {
		if term == "" || seen[term] {
			continue
		}
		seen[term] = true
		terms = append(terms, term)
	}
	if len(terms) > 1 {
		phrase = query
	}
	return terms, phrase, exactName
}

func scoreMemory(mem Entry, terms []string, phrase string, exactName string) int {
	name := strings.ToLower(strings.TrimSpace(mem.Name))
	memType := strings.ToLower(mem.Type)
	summary := strings.ToLower(mem.Summary)
	content := strings.ToLower(mem.Content)

	score := 0
	if exactName != "" && name == exactName {
		score += 100
	}
	for _, term := range terms {
		trimmedTerm := strings.TrimSuffix(term, ".md")
		if strings.Contains(name, term) || (trimmedTerm != "" && name == trimmedTerm) {
			score += 25
		}
		if strings.Contains(memType, term) {
			score += 8
		}
		if strings.Contains(summary, term) {
			score += 12
		}
		if count := strings.Count(content, term); count > 0 {
			if count > 3 {
				count = 3
			}
			score += 5 * count
		}
	}
	if phrase != "" {
		switch {
		case strings.Contains(name, phrase):
			score += 10
		case strings.Contains(summary, phrase):
			score += 8
		case strings.Contains(content, phrase):
			score += 6
		}
	}
	return score
}

type lineSpan struct {
	start int
	end   int
}

func extractContent(mem Entry, terms []string, maxRunes int) (string, bool) {
	text := strings.TrimSpace(strings.ReplaceAll(mem.Content, "\r\n", "\n"))
	if text == "" {
		text = singleLine(strings.TrimSpace(mem.Summary))
	}
	if text == "" {
		return "", false
	}

	if utf8.RuneCountInString(text) <= maxRunes {
		return text, false
	}

	lines := strings.Split(text, "\n")
	spans := matchedLineSpans(lines, terms)
	if len(spans) == 0 {
		return truncateLinesHead(lines, maxRunes), true
	}
	return renderLineSpans(lines, spans, terms, maxRunes), true
}

func matchedLineSpans(lines []string, terms []string) []lineSpan {
	spans := make([]lineSpan, 0)
	for i, line := range lines {
		if !containsAny(strings.ToLower(line), terms) {
			continue
		}
		start := i - contextWindowLines
		if start < 0 {
			start = 0
		}
		end := i + contextWindowLines
		if end >= len(lines) {
			end = len(lines) - 1
		}
		if n := len(spans); n > 0 && start <= spans[n-1].end+1 {
			if end > spans[n-1].end {
				spans[n-1].end = end
			}
			continue
		}
		spans = append(spans, lineSpan{start: start, end: end})
	}
	return spans
}

func truncateLinesHead(lines []string, maxRunes int) string {
	var sb strings.Builder
	used := 0
	for i, line := range lines {
		sep := 0
		if i > 0 {
			sep = 1
		}
		lineRunes := utf8.RuneCountInString(line)
		if used+sep+lineRunes > maxRunes {
			if used == 0 {
				return truncateRunes(line, maxRunes)
			}
			break
		}
		if sep > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(line)
		used += sep + lineRunes
	}
	return sb.String()
}

func renderLineSpans(lines []string, spans []lineSpan, terms []string, maxRunes int) string {
	var sb strings.Builder
	used := 0
	prevEnd := -1

	appendLine := func(line string) bool {
		sep := 0
		if used > 0 {
			sep = 1
		}
		lineRunes := utf8.RuneCountInString(line)
		if used+sep+lineRunes > maxRunes {
			return false
		}
		if sep > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(line)
		used += sep + lineRunes
		return true
	}

	for _, span := range spans {
		if span.start > prevEnd+1 {
			if !appendLine("...") {
				break
			}
		}
		stopped := false
		for i := span.start; i <= span.end; i++ {
			if !appendLine(lines[i]) {
				if used == 0 || (used == 3 && sb.String() == "...") {
					sb.Reset()
					return windowAroundMatch(lines[i], terms, maxRunes)
				}
				stopped = true
				break
			}
			prevEnd = i
		}
		if stopped {
			break
		}
	}
	if prevEnd >= 0 && prevEnd < len(lines)-1 {
		_ = appendLine("...")
	}
	return sb.String()
}

func windowAroundMatch(line string, terms []string, limit int) string {
	lower := strings.ToLower(line)
	index := -1
	for _, term := range terms {
		if at := strings.Index(lower, term); at >= 0 && (index < 0 || at < index) {
			index = at
		}
	}
	if index < 0 {
		return truncateRunes(line, limit)
	}

	runes := []rune(line)
	center := utf8.RuneCountInString(line[:index])
	start := center - limit/4
	if start < 0 {
		start = 0
	}
	end := start + limit
	if end > len(runes) {
		end = len(runes)
		start = end - limit
		if start < 0 {
			start = 0
		}
	}
	excerpt := string(runes[start:end])
	if start > 0 {
		excerpt = "…" + excerpt
	}
	if end < len(runes) {
		excerpt += "…"
	}
	return excerpt
}

func containsAny(text string, terms []string) bool {
	for _, term := range terms {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}

func trimSearchResults(results []SearchResult, maxBytes int) []SearchResult {
	used := 0
	kept := results[:0]
	for _, result := range results {
		cost := len(result.Name) + len(result.Type) + len(result.Summary) + len(result.Path) + len(result.Content)
		if len(kept) > 0 && used+cost > maxBytes {
			break
		}
		kept = append(kept, result)
		used += cost
	}
	return kept
}
