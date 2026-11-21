package server

import (
	"io/fs"
	"net/http"
	"os"
	"path"

	"github.com/pocketbase/pocketbase/core"
)

const defaultClientDir = "/usr/share/aist/client"

// serveClientRelease serves client bundles laid out as run.sh expects:
//
//	latest.txt, latest.json
//	<version>/aist-<version>-<os>-<arch>.tar.gz
//	<version>/checksums.txt
func (s *API) serveClientRelease(e *core.RequestEvent) error {
	name := e.Request.PathValue("path")
	if !fs.ValidPath(name) || name == "." {
		return e.NotFoundError("", nil)
	}

	fsys := os.DirFS(s.clientDir)
	info, err := fs.Stat(fsys, name)
	if err != nil || !info.Mode().IsRegular() {
		return e.NotFoundError("", nil)
	}

	switch path.Base(name) {
	case "latest.txt", "latest.json":
		e.Response.Header().Set("Cache-Control", "no-cache")
	default:
		e.Response.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	if path.Ext(name) == ".gz" {
		e.Response.Header().Set("Content-Type", "application/gzip")
	}

	http.ServeFileFS(e.Response, e.Request, fsys, name)
	return nil
}
