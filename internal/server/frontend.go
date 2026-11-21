package server

import (
	"html"
	"net/http"
	"strings"

	"github.com/llaoj/aist/frontend"
	"github.com/llaoj/aist/internal/buildinfo"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

const (
	assetVersionPlaceholder = "__AIST_ASSET_VERSION__"
	buildVersionPlaceholder = "__AIST_BUILD_VERSION__"
	serverURLPlaceholder    = "__AIST_SERVER_URL__"

	registerPageCSP = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'"
	indexPageCSP    = "default-src 'self'; script-src 'self' https://*.tawk.to https://cdn.jsdelivr.net; style-src 'self' 'unsafe-inline' https://*.tawk.to https://cdn.jsdelivr.net; img-src 'self' data: https://*.tawk.to https://cdn.jsdelivr.net; font-src 'self' data: https://*.tawk.to https://cdn.jsdelivr.net; connect-src 'self' https://*.tawk.to wss://*.tawk.to; frame-src 'self' https://*.tawk.to; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'"
)

func registerFrontendRoutes(e *core.ServeEvent) {
	e.Router.GET("/{path...}", apis.Static(frontend.FS, false)).
		BindFunc(func(e *core.RequestEvent) error {
			switch e.Request.URL.Path {
			case "/", "/index.html":
				return serveHTMLPage(e, "index.html", indexPageCSP)
			case "/register", "/register.html":
				return serveHTMLPage(e, "register.html", registerPageCSP)
			case "/run.sh":
				return serveRunScript(e)
			case "/uninstall.sh":
				e.Response.Header().Set("Content-Type", "text/plain; charset=utf-8")
				e.Response.Header().Set("Cache-Control", "no-cache")
				return e.Next()
			}

			if isImmutableAsset(e.Request.URL.Path) && assetVersion() != "dev" {
				e.Response.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				e.Response.Header().Set("Cache-Control", "no-cache")
			}
			return e.Next()
		})
}

func serveHTMLPage(e *core.RequestEvent, filename, csp string) error {
	raw, err := frontend.FS.ReadFile(filename)
	if err != nil {
		return e.NotFoundError("", err)
	}

	body := strings.ReplaceAll(string(raw), assetVersionPlaceholder, html.EscapeString(assetVersion()))
	body = strings.ReplaceAll(body, buildVersionPlaceholder, html.EscapeString(buildVersion()))
	body = strings.ReplaceAll(body, serverURLPlaceholder, html.EscapeString(serverURL(e)))

	h := e.Response.Header()
	h.Set("Cache-Control", "no-cache")
	h.Set("Content-Security-Policy", csp)
	h.Set("Referrer-Policy", "same-origin")
	return e.HTML(http.StatusOK, body)
}

func serveRunScript(e *core.RequestEvent) error {
	raw, err := frontend.FS.ReadFile("run.sh")
	if err != nil {
		return e.NotFoundError("", err)
	}

	url := serverURL(e)
	if !isSafeScriptURL(url) {
		url = serverURLPlaceholder
	}
	body := strings.ReplaceAll(string(raw), serverURLPlaceholder, url)

	e.Response.Header().Set("Cache-Control", "no-cache")
	return e.Blob(http.StatusOK, "text/plain; charset=utf-8", []byte(body))
}

func serverURL(e *core.RequestEvent) string {
	return strings.TrimRight(strings.TrimSpace(e.App.Settings().Meta.AppURL), "/")
}

func isSafeScriptURL(raw string) bool {
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		return false
	}
	for _, r := range raw {
		if r <= 0x20 || r == 0x7f || strings.ContainsRune(`"'`+"`$\\;&|<>", r) {
			return false
		}
	}
	return true
}

func isImmutableAsset(path string) bool {
	return strings.HasPrefix(path, "/css/") ||
		strings.HasPrefix(path, "/js/") ||
		strings.HasPrefix(path, "/img/") ||
		path == "/logo.svg"
}

func assetVersion() string {
	if version := buildinfo.Default.Commit; version != "" && version != "unknown" {
		return version
	}
	return buildVersion()
}

func buildVersion() string {
	if version := buildinfo.Default.Version; version != "" && version != "unknown" {
		return version
	}
	return "dev"
}
