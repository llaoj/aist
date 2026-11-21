// Package adminui injects AIST branding into the PocketBase superuser dashboard.
package adminui

import (
	"github.com/llaoj/aist/internal/server/adminui/branding"
	"github.com/pocketbase/pocketbase/core"
)

// Name is the UI extension name used in the public asset URL.
const Name = "aist"

// Bind appends the branding extension to the serve event.
func Bind(e *core.ServeEvent) {
	e.UIExtensions = append(e.UIExtensions, core.UIExtension{Name: Name, FS: branding.FS})
}
