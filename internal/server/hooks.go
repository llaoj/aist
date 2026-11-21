package server

import "github.com/pocketbase/pocketbase/core"

// BindHooks registers all server-side PocketBase hooks.
func (s *API) BindHooks(app core.App) {
	s.bindProviderHooks(app)
	s.bindModelHooks(app)
	s.bindAuthHooks(app)
}
