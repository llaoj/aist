// Package branding embeds brand assets for the PocketBase superuser dashboard.
package branding

import "embed"

//go:embed logo.svg logo-bw.svg main.js
var FS embed.FS
