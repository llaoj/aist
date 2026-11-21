package config

import (
	"strings"
	"time"
)

// Language constants.
const (
	LanguageEnglish = "en"
	LanguageChinese = "zh"
)

// IsSupportedLanguage reports whether language is a supported client language.
func IsSupportedLanguage(language string) bool {
	return language == LanguageEnglish || language == LanguageChinese
}

// SchemaVersion is the only supported account configuration schema version.
const SchemaVersion = 1

// Default values for account configuration.
const (
	DefaultMaxSessions     = 10
	DefaultKeepTokens      = 20000
	DefaultTriggerRatio    = 0.85
	DefaultContextWindow   = 128000
	DefaultMaxOutputTokens = 8192
)

// Default request timeouts for proxy calls.
const (
	DefaultTimeout       = 10 * time.Minute
	DefaultHeaderTimeout = 5 * time.Minute
	DefaultChunkTimeout  = 15 * time.Minute
)

// SessionConfig controls durable session behavior.
type SessionConfig struct {
	MaxSessions int
	Compaction  CompactionConfig
}

// CompactionConfig controls token-based history compaction.
type CompactionConfig struct {
	KeepTokens   int
	TriggerRatio float64
}

// Settings is the account configuration snapshot provided by the server.
type Settings struct {
	SchemaVersion   int
	ProxyBaseURL    string
	ContextWindow   int
	MaxOutputTokens int
	Session         SessionConfig
	Blacklist       []string
}

// Clone returns a deep copy of the configuration.
func (c *Settings) Clone() *Settings {
	if c == nil {
		return nil
	}
	out := *c
	out.Blacklist = append([]string(nil), c.Blacklist...)
	return &out
}

// EffectiveBlacklist returns the command blacklist.
func (c *Settings) EffectiveBlacklist() []string {
	if c == nil {
		return nil
	}
	return c.Blacklist
}

// EffectiveProxyBaseURL returns the proxy base URL without a trailing slash.
func (c *Settings) EffectiveProxyBaseURL() string {
	if c == nil {
		return ""
	}
	return strings.TrimRight(c.ProxyBaseURL, "/")
}

// EffectiveContextWindow returns the context window budget.
func (c *Settings) EffectiveContextWindow() int {
	if c == nil || c.ContextWindow <= 0 {
		return DefaultContextWindow
	}
	return c.ContextWindow
}

// EffectiveMaxOutputTokens returns the max output token budget.
func (c *Settings) EffectiveMaxOutputTokens() int {
	if c == nil || c.MaxOutputTokens <= 0 {
		return DefaultMaxOutputTokens
	}
	return c.MaxOutputTokens
}
