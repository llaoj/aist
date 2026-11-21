package migrations

import (
	"slices"
	"strings"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

const (
	defaultPocketBaseAppName     = "Acme"
	brandAppName                 = "AIST"
	defaultPocketBaseAccentColor = "#1055c9"
	brandAccentColor             = "#000000"
	registerRateLimitLabel       = "POST /v1/register"
)

func init() {
	m.Register(applyInitialSettings, revertInitialSettings)
}

func applyInitialSettings(app core.App) error {
	settings := app.Settings()
	changed := false

	if settings.Meta.AppName == defaultPocketBaseAppName {
		settings.Meta.AppName = brandAppName
		changed = true
	}

	accent := strings.ToLower(strings.TrimSpace(settings.Meta.AccentColor))
	if accent == "" || accent == defaultPocketBaseAccentColor {
		settings.Meta.AccentColor = brandAccentColor
		changed = true
	}

	hasRegisterRule := slices.ContainsFunc(settings.RateLimits.Rules, func(r core.RateLimitRule) bool {
		return r.Label == registerRateLimitLabel
	})
	if !hasRegisterRule {
		settings.RateLimits.Rules = append(settings.RateLimits.Rules, core.RateLimitRule{
			Label:       registerRateLimitLabel,
			MaxRequests: 5,
			Duration:    600,
		})
		changed = true
	}

	if !changed {
		return nil
	}
	return app.Save(settings)
}

func revertInitialSettings(app core.App) error {
	settings := app.Settings()
	changed := false

	if settings.Meta.AppName == brandAppName {
		settings.Meta.AppName = defaultPocketBaseAppName
		changed = true
	}

	if strings.ToLower(strings.TrimSpace(settings.Meta.AccentColor)) == brandAccentColor {
		settings.Meta.AccentColor = defaultPocketBaseAccentColor
		changed = true
	}

	origLen := len(settings.RateLimits.Rules)
	settings.RateLimits.Rules = slices.DeleteFunc(settings.RateLimits.Rules, func(r core.RateLimitRule) bool {
		return r.Label == registerRateLimitLabel
	})
	if len(settings.RateLimits.Rules) != origLen {
		changed = true
	}

	if !changed {
		return nil
	}
	return app.Save(settings)
}
