package app

import (
	"github.com/llaoj/aist/internal/config"
	"github.com/llaoj/aist/internal/remote"
)

// toSettings converts the server account configuration into the runtime
// configuration snapshot.
func toSettings(account remote.AccountConfig) (*config.Settings, error) {
	settings := &config.Settings{
		SchemaVersion:   account.SchemaVersion,
		ContextWindow:   account.Limits.ContextWindow,
		MaxOutputTokens: account.Limits.MaxOutputTokens,
		Blacklist:       append([]string(nil), account.Blacklist...),
	}
	if account.Session != nil {
		settings.Session = config.SessionConfig{
			MaxSessions: account.Session.MaxSessions,
			Compaction: config.CompactionConfig{
				KeepTokens:   account.Session.Compaction.KeepTokens,
				TriggerRatio: account.Session.Compaction.TriggerRatio,
			},
		}
	}
	return settings, nil
}
