package memory

import "time"

// CatalogProviderOptions configures prompt injection.
type CatalogProviderOptions struct {
	MaxBytes int
	// Now is injected in tests to make relative timestamps deterministic.
	Now func() time.Time
}

// CatalogProvider exposes loaded memories as a bounded prompt block.
type CatalogProvider struct {
	catalog Catalog
	mode    InjectionMode
}

// NewCatalogProvider builds a provider from an immutable Memory snapshot.
func NewCatalogProvider(memories []Entry, opts CatalogProviderOptions) *CatalogProvider {
	builder := NewCatalogBuilder(CatalogOptions{
		MaxBytes: opts.MaxBytes,
		Now:      opts.Now,
	})
	catalog := builder.Build(memories)

	mode := InjectionModeNone
	if len(memories) > 0 {
		mode = InjectionModeCatalog
	}
	return &CatalogProvider{catalog: catalog, mode: mode}
}

// Catalog returns the current prompt catalog.
func (p *CatalogProvider) Catalog() Catalog {
	if p == nil {
		return Catalog{}
	}
	return p.catalog
}

// Mode reports how Memory is injected.
func (p *CatalogProvider) Mode() InjectionMode {
	if p == nil {
		return InjectionModeNone
	}
	return p.mode
}

// PromptBlock returns the block that replaces {{MEMORY}} in prompts.
func (p *CatalogProvider) PromptBlock() string {
	if p == nil {
		return Catalog{}.PromptBlock()
	}
	return p.catalog.PromptBlock()
}
