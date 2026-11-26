package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func decodeProxyError(t *testing.T, body []byte) proxyErrorDetail {
	t.Helper()
	var resp proxyErrorResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode proxy error: %v body=%s", err, body)
	}
	return resp.Error
}

func TestProxyStructuredErrorCodes(t *testing.T) {
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	mode := "context_400"

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch mode {
		case "context_400":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"This model's maximum context length is 65536 tokens, model Qwen-Secret.","code":"context_length_exceeded"}}`))
		case "quota_402":
			w.WriteHeader(http.StatusPaymentRequired)
			_, _ = w.Write([]byte(`{"error":{"message":"Insufficient Balance"}}`))
		case "transient_500":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"internal crash"}`))
		}
	}))
	defer upstream.Close()

	app, srv := setupServerWithOptions(t, APIOptions{Now: func() time.Time { return now }})
	provider := createProvider(t, app, "p", upstream.URL, "sk-test", true)
	m1 := createModel(t, app, provider, "m1", 10, 0, 0, true)

	noModelUser := createUser(t, app, "nomodel", "nomodelpw123")
	configureUser(t, app, noModelUser, nil)

	user := createUser(t, app, "erruser", "erruserpw123")
	configureUser(t, app, user, nil, m1)

	h := buildMux(t, app, srv)
	noModelLogin := loginWithPassword(t, h, "nomodel@example.com", "nomodelpw123")
	login := loginWithPassword(t, h, "erruser@example.com", "erruserpw123")
	reqBody := `{"model":"auto","messages":[{"role":"user","content":"hi"}]}`

	// 1. User with no models -> 403 no_available_model.
	w := doJSON(t, h, http.MethodPost, "/v1/proxy/chat/completions", "Bearer "+noModelLogin.Token, reqBody)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	if detail := decodeProxyError(t, w.Body.Bytes()); detail.Code != proxyErrorCodeNoAvailableModel || detail.RetryAt != "" {
		t.Fatalf("no-model detail = %+v", detail)
	}

	// 2. Malformed JSON -> 400 invalid_request.
	w = doJSON(t, h, http.MethodPost, "/v1/proxy/chat/completions", "Bearer "+login.Token, `{bad json`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if detail := decodeProxyError(t, w.Body.Bytes()); detail.Code != proxyErrorCodeInvalidRequest {
		t.Fatalf("malformed json detail = %+v", detail)
	}

	// 3. Upstream 400 -> 400 invalid_request with extracted upstream error message.
	w = doJSON(t, h, http.MethodPost, "/v1/proxy/chat/completions", "Bearer "+login.Token, reqBody)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	detail := decodeProxyError(t, w.Body.Bytes())
	if detail.Code != proxyErrorCodeInvalidRequest || !strings.Contains(detail.Message, "maximum context length") {
		t.Fatalf("context 400 detail = %+v", detail)
	}

	// 4. Upstream 500 -> 502 upstream_unavailable.
	mode = "transient_500"
	w = doJSON(t, h, http.MethodPost, "/v1/proxy/chat/completions", "Bearer "+login.Token, reqBody)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", w.Code)
	}
	if detail := decodeProxyError(t, w.Body.Bytes()); detail.Code != proxyErrorCodeUpstreamUnavailable {
		t.Fatalf("502 detail = %+v", detail)
	}

	// 5. After transient cooldown expires, upstream 402 on only model -> marks quota cooldown and returns 503 with retry_at.
	now = now.Add(defaultTransientCooldown + time.Second)
	mode = "quota_402"
	w = doJSON(t, h, http.MethodPost, "/v1/proxy/chat/completions", "Bearer "+login.Token, reqBody)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 body=%s", w.Code, w.Body.String())
	}
	detail = decodeProxyError(t, w.Body.Bytes())
	wantRetry := now.Add(defaultQuotaCooldown).Format(time.RFC3339)
	if detail.Code != proxyErrorCodeNoAvailableModel || detail.RetryAt != wantRetry {
		t.Fatalf("503 detail = %+v, want retry_at=%s", detail, wantRetry)
	}
}
