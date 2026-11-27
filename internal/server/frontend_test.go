package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/llaoj/aist/internal/buildinfo"
)

func TestFrontendRoutes(t *testing.T) {
	app, srv := setupServer(t)
	h := buildMux(t, app, srv)

	tests := []struct {
		path        string
		status      int
		contentType string
		body        string
	}{
		{"/", http.StatusOK, "text/html", "AI Shell assistanT"},
		{"/logo.svg", http.StatusOK, "", "<svg"},
		{"/run.sh", http.StatusOK, "text/plain", "AIST_RELEASE_BASE_URL"},
		{"/uninstall.sh", http.StatusOK, "text/plain", "AIST_YES"},
		{"/css/style.css", http.StatusOK, "text/css", ""},
		{"/js/chat.js", http.StatusOK, "javascript", "AIST_CHAT"},
		{"/missing.txt", http.StatusNotFound, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := doJSON(t, h, http.MethodGet, tt.path, "", "")
			if got.Code != tt.status {
				t.Fatalf("status = %d, want %d", got.Code, tt.status)
			}
			if tt.contentType != "" {
				if contentType := got.Header().Get("Content-Type"); !strings.Contains(contentType, tt.contentType) {
					t.Fatalf("content type = %q, want %q", contentType, tt.contentType)
				}
			}
			if tt.body != "" && !strings.Contains(got.Body.String(), tt.body) {
				t.Fatalf("body does not contain %q", tt.body)
			}
		})
	}
}

func TestFrontendVersionAndCacheHeaders(t *testing.T) {
	previous := buildinfo.Default
	t.Cleanup(func() { buildinfo.Default = previous })
	buildinfo.Set("v0.5.0-test", "abc1234", "2026-11-12")

	app, srv := setupServer(t)
	h := buildMux(t, app, srv)

	t.Run("index injects asset and build versions", func(t *testing.T) {
		got := doJSON(t, h, http.MethodGet, "/", "", "")
		if got.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", got.Code, http.StatusOK)
		}
		body := got.Body.String()
		if strings.Contains(body, assetVersionPlaceholder) {
			t.Fatalf("body still contains %q", assetVersionPlaceholder)
		}
		if strings.Contains(body, buildVersionPlaceholder) {
			t.Fatalf("body still contains %q", buildVersionPlaceholder)
		}
		if !strings.Contains(body, "?v=abc1234") {
			t.Fatalf("body does not contain injected asset version")
		}
		if !strings.Contains(body, "v0.5.0-test") {
			t.Fatalf("body does not contain injected build version")
		}
		if cacheControl := got.Header().Get("Cache-Control"); cacheControl != "no-cache" {
			t.Fatalf("cache control = %q, want no-cache", cacheControl)
		}
	})

	t.Run("versioned assets are immutable", func(t *testing.T) {
		got := doJSON(t, h, http.MethodGet, "/css/style.css?v=abc1234", "", "")
		if got.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", got.Code, http.StatusOK)
		}
		cacheControl := got.Header().Get("Cache-Control")
		if !strings.Contains(cacheControl, "max-age=31536000") || !strings.Contains(cacheControl, "immutable") {
			t.Fatalf("cache control = %q, want long-lived immutable", cacheControl)
		}
	})

	t.Run("dev build does not cache assets", func(t *testing.T) {
		buildinfo.Default.Commit = "unknown"
		buildinfo.Default.Version = "unknown"
		t.Cleanup(func() { buildinfo.Set("v0.5.0-test", "abc1234", "2026-11-12") })

		got := doJSON(t, h, http.MethodGet, "/css/style.css?v=dev", "", "")
		if got.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", got.Code, http.StatusOK)
		}
		if cacheControl := got.Header().Get("Cache-Control"); cacheControl != "no-cache" {
			t.Fatalf("cache control = %q, want no-cache", cacheControl)
		}
	})

	t.Run("scripts are never cached", func(t *testing.T) {
		for _, path := range []string{"/run.sh", "/uninstall.sh"} {
			got := doJSON(t, h, http.MethodGet, path, "", "")
			if got.Code != http.StatusOK {
				t.Fatalf("%s status = %d, want %d", path, got.Code, http.StatusOK)
			}
			if cacheControl := got.Header().Get("Cache-Control"); cacheControl != "no-cache" {
				t.Fatalf("%s cache control = %q, want no-cache", path, cacheControl)
			}
		}
	})
}

func TestFrontendDoesNotShadowAPI(t *testing.T) {
	app, srv := setupServer(t)
	h := buildMux(t, app, srv)

	got := doJSON(t, h, http.MethodGet, "/v1/healthz", "", "")
	if got.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", got.Code, http.StatusOK)
	}
	if !strings.Contains(got.Body.String(), `"status":"ok"`) {
		t.Fatalf("unexpected body: %s", got.Body.String())
	}
}
