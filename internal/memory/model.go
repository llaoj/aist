// Package memory contains the Memory domain model and pure operations.
//
// Memory is environment knowledge, not instructions. It is stored as small
// Markdown files and is intentionally independent from configuration, LLM and
// UI packages. File I/O lives in memory/file; Markdown encoding lives in
// memory/markdown; snapshot storage lives in memory/store.
package memory

import "time"

// DefaultMaxBytes is the default total Memory budget.
const DefaultMaxBytes = 32 * 1024

// InjectionMode describes how Memory is exposed to the model.
type InjectionMode string

const (
	// InjectionModeNone means no Memory was loaded. It is the zero value.
	InjectionModeNone InjectionMode = ""
	// InjectionModeCatalog injects only names and summaries; details are fetched
	// on demand. This is the default when Memory exists.
	InjectionModeCatalog InjectionMode = "catalog"
)

// Entry is one environment description loaded from a Markdown file.
type Entry struct {
	Path     string
	Name     string
	Updated  time.Time
	Modified time.Time
	Type     string
	Source   string
	Summary  string
	Content  string
}

// EffectiveModified returns the freshness timestamp used for ordering.
func (m Entry) EffectiveModified() time.Time {
	if !m.Modified.IsZero() {
		return m.Modified
	}
	if !m.Updated.IsZero() {
		return m.Updated
	}
	return time.Time{}
}

// Bytes returns the body size that counts against the Memory budget.
func (m Entry) Bytes() int {
	return len(m.Content)
}

// CatalogEntry is the compact representation injected into the prompt.
type CatalogEntry struct {
	Name     string
	Type     string
	Summary  string
	Path     string
	Bytes    int
	Modified time.Time

	line string
}

// Catalog is the bounded list of Memory entries shown to the model.
type Catalog struct {
	Entries   []CatalogEntry
	Bytes     int
	MaxBytes  int
	Truncated bool
	Omitted   int
}

// FileSource identifies where a Memory file came from.
type FileSource string

const (
	FileSourceUnknown FileSource = ""
	FileSourceGlobal  FileSource = "global"
)

// FileStatus records one candidate file discovered during loading.
type FileStatus struct {
	Path   string
	Source FileSource
	Bytes  int
	Loaded bool
	Reason string
}

// Report describes the result of a Memory loading pass. It is data only;
// rendering belongs to the CLI.
type Report struct {
	Files    []FileStatus
	Loaded   int
	Bytes    int
	MaxBytes int
	Warnings []string
}

// LoadResult is one loader pass.
type LoadResult struct {
	Memories []Entry
	Report   Report
}
