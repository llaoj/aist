package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

const (
	upstreamProvidersCollection = "upstream_providers"
	providerProtocolField       = "protocol"
)

func init() {
	m.Register(addUpstreamProviderProtocol, removeUpstreamProviderProtocol)
}

func addUpstreamProviderProtocol(app core.App) error {
	col, err := app.FindCollectionByNameOrId(upstreamProvidersCollection)
	if err != nil {
		return err
	}
	if col.Fields.GetByName(providerProtocolField) != nil {
		return nil
	}
	col.Fields.Add(&core.SelectField{
		Name:      providerProtocolField,
		MaxSelect: 1,
		Values:    []string{"openai", "gemini"},
		Help:      "Upstream wire protocol. Empty or 'openai': OpenAI-compatible API, base_url is the /chat/completions root. 'gemini': native Google Gemini API, base_url is the API root such as https://generativelanguage.googleapis.com/v1beta.",
	})
	return app.Save(col)
}

func removeUpstreamProviderProtocol(app core.App) error {
	col, err := app.FindCollectionByNameOrId(upstreamProvidersCollection)
	if err != nil {
		return err
	}
	if col.Fields.GetByName(providerProtocolField) == nil {
		return nil
	}
	col.Fields.RemoveByName(providerProtocolField)
	return app.Save(col)
}
