package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"

	"github.com/llaoj/aist/internal/buildinfo"
)

func TestOpenAIModelSendsInjectedToolDefinition(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		if err := json.Unmarshal(data, &body); err != nil {
			t.Fatalf("Unmarshal request: %v", err)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	model, err := NewOpenAICompatibleModel(ProviderSpec{
		ID:        "p/m",
		BaseURL:   server.URL,
		ModelName: "m",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := model.Call(context.Background(), CompletionRequest{
		Tools: []ToolDefinition{{
			Name:        "shell_execute",
			Description: "run a command",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
	}); err != nil {
		t.Fatalf("Call: %v", err)
	}

	tools, ok := body["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %#v", body["tools"])
	}
	tool := tools[0].(map[string]any)["function"].(map[string]any)
	if tool["name"] != "shell_execute" || tool["description"] != "run a command" {
		t.Fatalf("tool = %#v", tool)
	}
	if _, ok := tool["parameters"].(map[string]any); !ok {
		t.Fatalf("parameters = %#v", tool["parameters"])
	}
	if body["tool_choice"] != "auto" {
		t.Fatalf("tool_choice = %#v, want auto", body["tool_choice"])
	}
}

func TestOpenAIModelHonorsToolChoiceNoneWithToolsRetained(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		if err := json.Unmarshal(data, &body); err != nil {
			t.Fatalf("Unmarshal request: %v", err)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"summary"}}]}`))
	}))
	defer server.Close()

	model, err := NewOpenAICompatibleModel(ProviderSpec{
		ID:        "p/m",
		BaseURL:   server.URL,
		ModelName: "m",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := model.Call(context.Background(), CompletionRequest{
		Tools: []ToolDefinition{{
			Name:        "shell_execute",
			Description: "run a command",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
		ToolChoice: ToolChoiceNone,
	}); err != nil {
		t.Fatalf("Call: %v", err)
	}

	tools, ok := body["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %#v, want 1 tool definition", body["tools"])
	}
	if body["tool_choice"] != "none" {
		t.Fatalf("tool_choice = %#v, want none", body["tool_choice"])
	}
}

func TestOpenAIModelUsesProxyFromEnvironment(t *testing.T) {
	// http.ProxyFromEnvironment caches environment variables on first call, so
	// run the proxied request in a fresh child process.
	if os.Getenv("AIST_TEST_PROXY_CHILD") == "1" {
		model, err := NewOpenAICompatibleModel(ProviderSpec{
			ID:        "p/m",
			BaseURL:   "http://upstream.invalid/v1",
			ModelName: "m",
		})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		resp, err := model.Call(context.Background(), CompletionRequest{})
		if err != nil {
			t.Fatalf("Call: %v", err)
		}
		if resp.Content != "via-proxy" {
			t.Fatalf("Content = %q, want via-proxy", resp.Content)
		}
		return
	}

	var proxiedURI string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxiedURI = r.RequestURI
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"via-proxy"}}]}`))
	}))
	defer proxy.Close()

	cmd := exec.Command(os.Args[0], "-test.run=^TestOpenAIModelUsesProxyFromEnvironment$")
	cmd.Env = append(os.Environ(),
		"AIST_TEST_PROXY_CHILD=1",
		"HTTP_PROXY="+proxy.URL,
		"http_proxy="+proxy.URL,
		"NO_PROXY=",
		"no_proxy=",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child failed: %v\n%s", err, out)
	}
	if proxiedURI != "http://upstream.invalid/v1/chat/completions" {
		t.Fatalf("proxied URI = %q, want http://upstream.invalid/v1/chat/completions", proxiedURI)
	}
}

func TestOpenAIModelSetsDefaultUserAgent(t *testing.T) {
	model, err := NewOpenAICompatibleModel(ProviderSpec{
		ID:        "p/m",
		BaseURL:   "https://example.com/v1",
		ModelName: "m",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://example.com/v1/chat/completions", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if err := model.applyHeaders(context.Background(), req); err != nil {
		t.Fatalf("applyHeaders: %v", err)
	}
	if got := req.Header.Get("User-Agent"); got != buildinfo.UserAgent() {
		t.Fatalf("User-Agent = %q, want %q", got, buildinfo.UserAgent())
	}
}
