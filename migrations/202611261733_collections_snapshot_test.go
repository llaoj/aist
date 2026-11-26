package migrations

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

func TestInitialCollectionsSnapshot(t *testing.T) {
	app := pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDataDir: filepath.Join(t.TempDir(), "pb_data"),
	})
	if err := app.Bootstrap(); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("RunAllMigrations: %v", err)
	}

	assertSuperuserOnly := func(t *testing.T, col *core.Collection) {
		t.Helper()
		if col.ListRule != nil || col.ViewRule != nil || col.CreateRule != nil || col.UpdateRule != nil || col.DeleteRule != nil {
			t.Fatalf("collection %s rules must be nil (superuser-only)", col.Name)
		}
	}

	assertFields := func(t *testing.T, col *core.Collection, wantPresent []string, wantAbsent []string) {
		t.Helper()
		for _, name := range wantPresent {
			if col.Fields.GetByName(name) == nil {
				t.Fatalf("collection %s missing expected field %q", col.Name, name)
			}
		}
		for _, name := range wantAbsent {
			if col.Fields.GetByName(name) != nil {
				t.Fatalf("collection %s unexpectedly contains removed field %q", col.Name, name)
			}
		}
	}

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("find users: %v", err)
	}
	assertSuperuserOnly(t, users)
	assertFields(t, users,
		[]string{"email", "allowed_models", "weekly_token_quota", "settings", "created", "updated"},
		[]string{"default_model", "language"},
	)
	if emailField, ok := users.Fields.GetByName("email").(*core.EmailField); !ok || emailField == nil || !emailField.Presentable {
		t.Fatalf("users.email must be presentable")
	}
	if nameField, ok := users.Fields.GetByName("name").(*core.TextField); !ok || nameField == nil || nameField.Presentable {
		t.Fatalf("users.name must not be presentable")
	}

	providers, err := app.FindCollectionByNameOrId("upstream_providers")
	if err != nil {
		t.Fatalf("find upstream_providers: %v", err)
	}
	assertSuperuserOnly(t, providers)
	assertFields(t, providers,
		[]string{"name", "base_url", "api_key", "enabled"},
		[]string{"api_key_enc"},
	)

	models, err := app.FindCollectionByNameOrId("upstream_models")
	if err != nil {
		t.Fatalf("find upstream_models: %v", err)
	}
	assertSuperuserOnly(t, models)
	assertFields(t, models,
		[]string{
			"provider", "display_name", "name", "priority",
			"context_window", "max_output_tokens", "extra_body", "enabled",
			"unavailable_until", "unavailable_reason", "last_error",
			"created", "updated",
		},
		nil,
	)
	if extraBodyField, ok := models.Fields.GetByName("extra_body").(*core.JSONField); !ok || extraBodyField == nil {
		t.Fatalf("extra_body is not a JSONField")
	}
	reasonField, ok := models.Fields.GetByName("unavailable_reason").(*core.SelectField)
	if !ok || reasonField == nil {
		t.Fatalf("unavailable_reason is not a SelectField")
	}
	for _, want := range []string{"quota", "rate_limit", "auth", "transient"} {
		if !slices.Contains(reasonField.Values, want) {
			t.Fatalf("unavailable_reason values = %v, missing %q", reasonField.Values, want)
		}
	}

	usage, err := app.FindCollectionByNameOrId("daily_model_usage")
	if err != nil {
		t.Fatalf("find daily_model_usage: %v", err)
	}
	assertSuperuserOnly(t, usage)
	assertFields(t, usage,
		[]string{"user", "provider", "model", "date", "requests", "prompt_tokens", "completion_tokens", "total_tokens", "created", "updated"},
		nil,
	)
	if len(usage.Indexes) != 2 {
		t.Fatalf("daily_model_usage indexes = %d, want 2", len(usage.Indexes))
	}

	invites, err := app.FindCollectionByNameOrId("invite_codes")
	if err != nil {
		t.Fatalf("find invite_codes: %v", err)
	}
	assertSuperuserOnly(t, invites)
	assertFields(t, invites,
		[]string{"code", "enabled", "max_uses", "used_count", "expires_at", "allowed_models", "weekly_token_quota", "settings", "note", "created", "updated"},
		nil,
	)
	if len(invites.Indexes) != 1 {
		t.Fatalf("invite_codes indexes = %d, want 1", len(invites.Indexes))
	}

	if got := app.Settings().Meta.AppName; got != brandAppName {
		t.Fatalf("fresh DB AppName = %q, want %q", got, brandAppName)
	}
	if got := app.Settings().Meta.AccentColor; got != brandAccentColor {
		t.Fatalf("fresh DB AccentColor = %q, want %q", got, brandAccentColor)
	}
}
