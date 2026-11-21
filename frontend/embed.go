// Package frontend embeds the public AIST website served by the server binary.
package frontend

import "embed"

//go:embed index.html register.html logo.svg run.sh uninstall.sh css js
var FS embed.FS
