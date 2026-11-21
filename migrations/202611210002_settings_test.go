package migrations

import (
	"slices"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func TestApplyAndRevertInitialSettings(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(app.Cleanup)

	hasRegisterRule := func() bool {
		return slices.ContainsFunc(app.Settings().RateLimits.Rules, func(r core.RateLimitRule) bool {
			return r.Label == registerRateLimitLabel
		})
	}

	if got := app.Settings().Meta.AccentColor; got != brandAccentColor {
		t.Fatalf("expected accent color %q, got %q", brandAccentColor, got)
	}
	if !hasRegisterRule() {
		t.Fatalf("expected %s rate limit rule present", registerRateLimitLabel)
	}

	// NewTestApp seeds AppName="acme_test"; reset to PB's production default ("Acme")
	// to verify AppName branding alongside AccentColor and RateLimits.
	app.Settings().Meta.AppName = defaultPocketBaseAppName
	if err := app.Save(app.Settings()); err != nil {
		t.Fatalf("save default app name: %v", err)
	}
	if err := applyInitialSettings(app); err != nil {
		t.Fatalf("applyInitialSettings: %v", err)
	}
	if got := app.Settings().Meta.AppName; got != brandAppName {
		t.Fatalf("expected app name %q, got %q", brandAppName, got)
	}

	if err := applyInitialSettings(app); err != nil {
		t.Fatalf("applyInitialSettings (repeat): %v", err)
	}

	if err := revertInitialSettings(app); err != nil {
		t.Fatalf("revertInitialSettings: %v", err)
	}
	if got := app.Settings().Meta.AppName; got != defaultPocketBaseAppName {
		t.Fatalf("expected reverted app name %q, got %q", defaultPocketBaseAppName, got)
	}
	if got := app.Settings().Meta.AccentColor; got != defaultPocketBaseAccentColor {
		t.Fatalf("expected reverted accent color %q, got %q", defaultPocketBaseAccentColor, got)
	}
	if hasRegisterRule() {
		t.Fatalf("expected %s rule removed after revert", registerRateLimitLabel)
	}

	if err := applyInitialSettings(app); err != nil {
		t.Fatalf("applyInitialSettings after revert: %v", err)
	}
	if got := app.Settings().Meta.AppName; got != brandAppName {
		t.Fatalf("expected app name %q after re-apply, got %q", brandAppName, got)
	}
	if got := app.Settings().Meta.AccentColor; got != brandAccentColor {
		t.Fatalf("expected accent color %q after re-apply, got %q", brandAccentColor, got)
	}
	if !hasRegisterRule() {
		t.Fatalf("expected %s rule restored after re-apply", registerRateLimitLabel)
	}

	// Custom admin values must not be overwritten on repeat apply.
	app.Settings().Meta.AppName = "CustomOps"
	app.Settings().Meta.AccentColor = "#ff5500"
	if err := app.Save(app.Settings()); err != nil {
		t.Fatalf("save custom settings: %v", err)
	}
	if err := applyInitialSettings(app); err != nil {
		t.Fatalf("applyInitialSettings (custom): %v", err)
	}
	if got := app.Settings().Meta.AppName; got != "CustomOps" {
		t.Fatalf("expected custom app name preserved, got %q", got)
	}
	if got := app.Settings().Meta.AccentColor; got != "#ff5500" {
		t.Fatalf("expected custom accent color preserved, got %q", got)
	}
}
