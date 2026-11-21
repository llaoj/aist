package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/security"
)

var testEncryptionKey = strings.Repeat("a", 32)

func TestProviderAPIKeyPlaintextAndMaskedInAdminAPI(t *testing.T) {
	app, srv := setupServer(t)
	t.Setenv(app.EncryptionEnv(), "")

	provider := createProvider(t, app, "hook", "https://example.com", "sk-plain", true)

	record, err := app.FindRecordById(upstreamProvidersCollection, provider.Id)
	if err != nil {
		t.Fatalf("find provider: %v", err)
	}
	if got := record.GetString(providerAPIKeyField); got != "sk-plain" {
		t.Fatalf("stored = %q, want plaintext when encryption env is unset", got)
	}
	resolved, err := resolveProviderAPIKey(app, record)
	if err != nil || resolved != "sk-plain" {
		t.Fatalf("resolveProviderAPIKey = %q, %v", resolved, err)
	}

	superCol, err := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		t.Fatalf("superusers collection: %v", err)
	}
	super := core.NewRecord(superCol)
	super.SetEmail("admin@example.com")
	super.SetPassword("admin-password-123")
	if err := app.Save(super); err != nil {
		t.Fatalf("save superuser: %v", err)
	}
	superToken, err := super.NewAuthToken()
	if err != nil {
		t.Fatalf("NewAuthToken: %v", err)
	}

	h := buildMux(t, app, srv)
	path := "/api/collections/upstream_providers/records/" + provider.Id

	getResp := doJSON(t, h, http.MethodGet, path, "Bearer "+superToken, "")
	if getResp.Code != http.StatusOK {
		t.Fatalf("GET status = %d body=%s", getResp.Code, getResp.Body.String())
	}
	var view map[string]any
	if err := json.Unmarshal(getResp.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode view: %v", err)
	}
	if view[providerAPIKeyField] != maskedAPIKey {
		t.Fatalf("admin response api_key = %v, want %q", view[providerAPIKeyField], maskedAPIKey)
	}

	patchResp := doJSON(t, h, http.MethodPatch, path, "Bearer "+superToken,
		`{"enabled":false,"api_key":"`+maskedAPIKey+`"}`)
	if patchResp.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d body=%s", patchResp.Code, patchResp.Body.String())
	}

	record, err = app.FindRecordById(upstreamProvidersCollection, provider.Id)
	if err != nil {
		t.Fatalf("find provider after patch: %v", err)
	}
	if got := record.GetString(providerAPIKeyField); got != "sk-plain" {
		t.Fatalf("masked PATCH overwrote api_key: %q", got)
	}
}

func TestProviderAPIKeyEncryptedWhenEncryptionEnvSet(t *testing.T) {
	app, _ := setupServer(t)
	t.Setenv(app.EncryptionEnv(), testEncryptionKey)

	provider := createProvider(t, app, "hook", "https://example.com", "sk-plain", true)

	record, err := app.FindRecordById(upstreamProvidersCollection, provider.Id)
	if err != nil {
		t.Fatalf("find provider: %v", err)
	}
	stored := record.GetString(providerAPIKeyField)
	if stored == "sk-plain" {
		t.Fatal("api key stored in plaintext despite encryption key")
	}

	decrypted, err := security.Decrypt(stored, testEncryptionKey)
	if err != nil {
		t.Fatalf("security.Decrypt: %v", err)
	}
	if string(decrypted) != "sk-plain" {
		t.Fatalf("decrypted = %q", decrypted)
	}

	record.Set("enabled", false)
	if err := app.Save(record); err != nil {
		t.Fatalf("save provider: %v", err)
	}
	record, err = app.FindRecordById(upstreamProvidersCollection, provider.Id)
	if err != nil {
		t.Fatalf("find provider: %v", err)
	}
	if got := record.GetString(providerAPIKeyField); got != stored {
		t.Fatalf("unchanged update re-encrypted key: %q -> %q", stored, got)
	}

	record.Set(providerAPIKeyField, "sk-rotated")
	if err := app.Save(record); err != nil {
		t.Fatalf("save provider: %v", err)
	}
	record, err = app.FindRecordById(upstreamProvidersCollection, provider.Id)
	if err != nil {
		t.Fatalf("find provider: %v", err)
	}
	resolved, err := resolveProviderAPIKey(app, record)
	if err != nil || resolved != "sk-rotated" {
		t.Fatalf("resolveProviderAPIKey = %q, %v", resolved, err)
	}

	t.Setenv(app.EncryptionEnv(), strings.Repeat("b", 32))
	if _, err := resolveProviderAPIKey(app, record); err == nil {
		t.Fatal("expected decrypt failure with wrong encryption key")
	}

	shortRec := core.NewRecord(record.Collection())
	shortRec.Set(providerAPIKeyField, "abcd1234")
	if _, err := resolveProviderAPIKey(app, shortRec); err == nil {
		t.Fatal("expected error for short unencrypted value when encryption key is set")
	}
}
