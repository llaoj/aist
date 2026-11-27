package migrations

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

func TestUpstreamProviderProtocolMigration(t *testing.T) {
	app := pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDataDir: filepath.Join(t.TempDir(), "pb_data"),
	})
	if err := app.Bootstrap(); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("RunAllMigrations: %v", err)
	}

	col, err := app.FindCollectionByNameOrId(upstreamProvidersCollection)
	if err != nil {
		t.Fatalf("find collection: %v", err)
	}
	field, ok := col.Fields.GetByName(providerProtocolField).(*core.SelectField)
	if !ok || field == nil {
		t.Fatalf("protocol is not a SelectField")
	}
	for _, want := range []string{"openai", "gemini"} {
		if !slices.Contains(field.Values, want) {
			t.Fatalf("protocol values = %v, missing %q", field.Values, want)
		}
	}

	if err := addUpstreamProviderProtocol(app); err != nil {
		t.Fatalf("re-apply addUpstreamProviderProtocol: %v", err)
	}

	if err := removeUpstreamProviderProtocol(app); err != nil {
		t.Fatalf("removeUpstreamProviderProtocol: %v", err)
	}
	col, err = app.FindCollectionByNameOrId(upstreamProvidersCollection)
	if err != nil {
		t.Fatalf("find collection after down: %v", err)
	}
	if col.Fields.GetByName(providerProtocolField) != nil {
		t.Fatalf("protocol field still present after down migration")
	}
}
