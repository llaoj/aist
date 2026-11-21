package server

import (
	"errors"
	"fmt"
	"os"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/security"
)

const (
	upstreamProvidersCollection = "upstream_providers"
	providerAPIKeyField         = "api_key"
	maskedAPIKey                = "••••••••"
	minEncryptedAPIKeyLen       = 40
)

func (s *API) bindProviderHooks(app core.App) {
	app.OnRecordCreate(upstreamProvidersCollection).BindFunc(s.prepareProviderAPIKey)
	app.OnRecordUpdate(upstreamProvidersCollection).BindFunc(s.prepareProviderAPIKey)
	app.OnRecordEnrich(upstreamProvidersCollection).BindFunc(s.maskProviderAPIKey)
}

func (s *API) prepareProviderAPIKey(e *core.RecordEvent) error {
	record := e.Record
	wasNew := record.IsNew()
	value := record.GetString(providerAPIKeyField)
	original := record.Original().GetString(providerAPIKeyField)
	if value == maskedAPIKey {
		record.Set(providerAPIKeyField, original)
		return e.Next()
	}
	changed := value != original
	if value != "" && changed {
		if key := encryptionKey(e.App); key != "" {
			encrypted, err := security.Encrypt([]byte(value), key)
			if err != nil {
				return err
			}
			record.Set(providerAPIKeyField, encrypted)
		}
	}
	if err := e.Next(); err != nil {
		return err
	}
	if changed && !wasNew {
		clearProviderAuthUnavailable(e.App, record.Id)
	}
	return nil
}

func (s *API) maskProviderAPIKey(e *core.RecordEnrichEvent) error {
	if e.Record.GetString(providerAPIKeyField) != "" {
		e.Record.Set(providerAPIKeyField, maskedAPIKey)
	}
	return e.Next()
}

func resolveProviderAPIKey(app core.App, record *core.Record) (string, error) {
	stored := record.GetString(providerAPIKeyField)
	if stored == "" {
		return "", nil
	}
	key := encryptionKey(app)
	if key == "" {
		return stored, nil
	}
	if len(stored) < minEncryptedAPIKeyLen {
		return "", errors.New("ciphertext too short")
	}
	decrypted, err := security.Decrypt(stored, key)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt upstream key: %w", err)
	}
	return string(decrypted), nil
}

func encryptionKey(app core.App) string {
	envName := app.EncryptionEnv()
	if envName == "" {
		return ""
	}
	return os.Getenv(envName)
}
