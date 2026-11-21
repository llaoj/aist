package config

import "strings"

// Normalize fills account configuration defaults in place.
func (c *Settings) Normalize() {
	if c == nil {
		return
	}
	if c.SchemaVersion == 0 {
		c.SchemaVersion = SchemaVersion
	}
	if c.ContextWindow <= 0 {
		c.ContextWindow = DefaultContextWindow
	}
	if c.MaxOutputTokens <= 0 {
		c.MaxOutputTokens = DefaultMaxOutputTokens
	}
	if c.Session.MaxSessions <= 0 {
		c.Session.MaxSessions = DefaultMaxSessions
	}
	if c.Session.Compaction.KeepTokens <= 0 {
		c.Session.Compaction.KeepTokens = DefaultKeepTokens
	}
	if c.Session.Compaction.TriggerRatio <= 0 {
		c.Session.Compaction.TriggerRatio = DefaultTriggerRatio
	}
	c.ProxyBaseURL = strings.TrimRight(strings.TrimSpace(c.ProxyBaseURL), "/")
}
