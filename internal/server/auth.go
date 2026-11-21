package server

import (
	"github.com/pocketbase/pocketbase/core"
)

const usersCollection = "users"

// bindAuthHooks customizes PocketBase auth responses with the client account
// configuration and hides fields that are only meaningful on the server.
func (s *API) bindAuthHooks(app core.App) {
	app.OnRecordAuthRequest(usersCollection).BindFunc(s.attachAccountConfig)
}

func (s *API) attachAccountConfig(e *core.RecordAuthRequestEvent) error {
	cfg, err := buildClientAccountConfig(e.App, e.Record)
	if err != nil {
		return err
	}

	e.Record.Hide("allowed_models", "weekly_token_quota", "settings")

	meta, _ := e.Meta.(map[string]any)
	if meta == nil {
		meta = map[string]any{}
	}
	meta["config"] = cfg
	e.Meta = meta

	return e.Next()
}
