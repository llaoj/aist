// Package markdown encodes and decodes the small Memory file format.
package markdown

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/llaoj/aist/internal/memory"
	"gopkg.in/yaml.v3"
)

// Warning describes a non-fatal parsing problem.
type Warning struct {
	Line    int
	Message string
}

var dateLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

// Decode decodes a Memory Markdown document. Metadata is stored in YAML
// frontmatter only.
func Decode(path string, data []byte) (memory.Entry, []Warning, error) {
	mem := memory.Entry{Path: path}
	warnings := make([]Warning, 0)

	text := strings.TrimPrefix(strings.ReplaceAll(string(data), "\r\n", "\n"), "\ufeff")
	front, body, frontWarnings := splitFrontmatter(text)
	warnings = append(warnings, frontWarnings...)
	applyFrontmatter(&mem, front, &warnings)

	lines := strings.Split(body, "\n")
	titleIndex := findTitle(lines)
	if titleIndex < 0 {
		warnings = append(warnings, Warning{Line: 1, Message: "missing # title"})
		mem.Name = fallbackName(path)
		mem.Content = strings.TrimSpace(body)
	} else {
		mem.Name = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[titleIndex]), "# "))
		contentStart := findContentStart(lines, titleIndex+1)
		mem.Content = strings.TrimSpace(strings.Join(lines[contentStart:], "\n"))
	}

	if mem.Name == "" {
		mem.Name = fallbackName(path)
	}
	if mem.Modified.IsZero() {
		mem.Modified = mem.Updated
	}
	return mem, warnings, nil
}

func splitFrontmatter(text string) (map[string]any, string, []Warning) {
	front := map[string]any{}
	if !strings.HasPrefix(text, "---\n") {
		return front, text, nil
	}

	rest := strings.TrimPrefix(text, "---\n")
	index := strings.Index(rest, "\n---")
	if index < 0 {
		return front, text, []Warning{{Line: 1, Message: "unterminated YAML frontmatter"}}
	}

	raw := rest[:index]
	body := rest[index+4:]
	body = strings.TrimPrefix(body, "\n")
	if err := yaml.Unmarshal([]byte(raw), &front); err != nil {
		return map[string]any{}, body, []Warning{{Line: 1, Message: "invalid YAML frontmatter: " + err.Error()}}
	}
	return front, body, nil
}

func applyFrontmatter(mem *memory.Entry, front map[string]any, warnings *[]Warning) {
	for key, value := range front {
		if value == nil {
			continue
		}
		normalized := strings.ToLower(strings.TrimSpace(key))
		text := fmt.Sprint(value)
		switch normalized {
		case "updated":
			if parsed, ok := parseDate(value); ok {
				mem.Updated = parsed
			} else {
				*warnings = append(*warnings, Warning{Line: 1, Message: "invalid updated value: " + text})
			}
		case "modified":
			if parsed, ok := parseDate(value); ok {
				mem.Modified = parsed
			} else {
				*warnings = append(*warnings, Warning{Line: 1, Message: "invalid modified value: " + text})
			}
		case "source":
			mem.Source = text
		case "summary":
			mem.Summary = text
		case "type":
			mem.Type = text
		}
	}
}

func findTitle(lines []string) int {
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") && !strings.HasPrefix(trimmed, "## ") {
			return i
		}
	}
	return -1
}

func findContentStart(lines []string, start int) int {
	for i := start; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			heading := strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
			if strings.EqualFold(heading, "memory") {
				return i + 1
			}
		}
		return i
	}
	return len(lines)
}

func parseDate(value any) (time.Time, bool) {
	if typed, ok := value.(time.Time); ok {
		return typed, true
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	text = strings.Trim(text, `"'`)
	if text == "" {
		return time.Time{}, false
	}
	for _, layout := range dateLayouts {
		if parsed, err := time.Parse(layout, text); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func fallbackName(path string) string {
	name := filepath.Base(path)
	name = strings.TrimSuffix(name, filepath.Ext(name))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "memory"
	}
	return name
}
