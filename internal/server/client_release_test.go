package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeClientRelease(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"latest.txt":                             "v1.2.3\n",
		"latest.json":                            `{"version":"v1.2.3"}` + "\n",
		"v1.2.3/aist-v1.2.3-linux-amd64.tar.gz":  "archive-bytes",
		"v1.2.3/checksums.txt":                   "abc  aist-v1.2.3-linux-amd64.tar.gz\n",
		"v1.2.3/nested/.keep":                    "",
		"v1.2.3/aist-v1.2.3-darwin-arm64.tar.gz": "archive-bytes-darwin",
	}
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

func TestClientReleaseRoutes(t *testing.T) {
	app, srv := setupServerWithOptions(t, APIOptions{ClientDir: writeClientRelease(t)})
	h := buildMux(t, app, srv)

	tests := []struct {
		path         string
		status       int
		body         string
		contentType  string
		cacheControl string
	}{
		{"/releases/client/latest.txt", http.StatusOK, "v1.2.3", "text/plain", "no-cache"},
		{"/releases/client/latest.json", http.StatusOK, `"v1.2.3"`, "application/json", "no-cache"},
		{"/releases/client/v1.2.3/aist-v1.2.3-linux-amd64.tar.gz", http.StatusOK, "archive-bytes", "application/gzip", "public, max-age=31536000, immutable"},
		{"/releases/client/v1.2.3/checksums.txt", http.StatusOK, "aist-v1.2.3-linux-amd64.tar.gz", "text/plain", "public, max-age=31536000, immutable"},
		{"/releases/client/v1.2.3/aist-v1.2.3-linux-arm64.tar.gz", http.StatusNotFound, "", "", ""},
		{"/releases/client/v1.2.3/", http.StatusNotFound, "", "", ""},
		{"/releases/client/v1.2.3/nested", http.StatusNotFound, "", "", ""},
		{"/releases/client/", http.StatusNotFound, "", "", ""},
		{"/releases/client/v1.2.3/..%2f..%2fgo.mod", http.StatusNotFound, "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := doJSON(t, h, http.MethodGet, tt.path, "", "")
			if got.Code != tt.status {
				t.Fatalf("status = %d, want %d", got.Code, tt.status)
			}
			if tt.body != "" && !strings.Contains(got.Body.String(), tt.body) {
				t.Fatalf("body = %q, want it to contain %q", got.Body.String(), tt.body)
			}
			if tt.contentType != "" && !strings.Contains(got.Header().Get("Content-Type"), tt.contentType) {
				t.Fatalf("content type = %q, want %q", got.Header().Get("Content-Type"), tt.contentType)
			}
			if tt.cacheControl != "" && got.Header().Get("Cache-Control") != tt.cacheControl {
				t.Fatalf("cache control = %q, want %q", got.Header().Get("Cache-Control"), tt.cacheControl)
			}
		})
	}
}

func TestClientReleaseMissingDirReturnsNotFound(t *testing.T) {
	app, srv := setupServerWithOptions(t, APIOptions{
		ClientDir: filepath.Join(t.TempDir(), "missing"),
	})
	h := buildMux(t, app, srv)

	got := doJSON(t, h, http.MethodGet, "/releases/client/latest.txt", "", "")
	if got.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", got.Code, http.StatusNotFound)
	}
}

func TestServerURLInjectionFromAppSettings(t *testing.T) {
	app, srv := setupServer(t)
	app.Settings().Meta.AppURL = "https://aist.example.org/"
	h := buildMux(t, app, srv)

	runSh := doJSON(t, h, http.MethodGet, "/run.sh", "", "")
	if runSh.Code != http.StatusOK {
		t.Fatalf("run.sh status = %d, want %d", runSh.Code, http.StatusOK)
	}
	if !strings.Contains(runSh.Body.String(), `DEFAULT_SERVER_URL="https://aist.example.org"`) {
		t.Fatal("run.sh does not contain trimmed Meta.AppURL")
	}
	if got := runSh.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("cache control = %q, want no-cache", got)
	}
	if got := runSh.Header().Get("Content-Type"); !strings.Contains(got, "text/plain") {
		t.Fatalf("content type = %q, want text/plain", got)
	}

	index := doJSON(t, h, http.MethodGet, "/", "", "")
	if index.Code != http.StatusOK {
		t.Fatalf("index status = %d, want %d", index.Code, http.StatusOK)
	}
	if strings.Contains(index.Body.String(), serverURLPlaceholder) {
		t.Fatalf("index still contains %q", serverURLPlaceholder)
	}
	if !strings.Contains(index.Body.String(), "curl -fL -# https://aist.example.org/run.sh | sh") {
		t.Fatal("index does not contain the injected install command")
	}
}
