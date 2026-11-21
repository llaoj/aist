package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	_ "github.com/llaoj/aist/migrations"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/types"
)

// setupServer bootstraps a test app, applies the schema, and registers routes.
func setupServer(t *testing.T) (*tests.TestApp, *API) {
	t.Helper()
	return setupServerWithOptions(t, APIOptions{})
}

func setupServerWithOptions(t *testing.T, opts APIOptions) (*tests.TestApp, *API) {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(app.Cleanup)
	srv := NewAPI(opts)
	srv.BindHooks(app)
	return app, srv
}

func createUser(t *testing.T, app *tests.TestApp, username, password string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("users collection: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("name", username)
	rec.SetEmail(username + "@example.com")
	rec.SetPassword(password)
	if err := app.Save(rec); err != nil {
		t.Fatalf("save user: %v", err)
	}
	return rec
}

func createProvider(t *testing.T, app *tests.TestApp, name, baseURL, apiKey string, enabled bool) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId(upstreamProvidersCollection)
	if err != nil {
		t.Fatalf("providers collection: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("name", name)
	rec.Set("base_url", baseURL)
	rec.Set(providerAPIKeyField, apiKey)
	rec.Set("enabled", enabled)
	if err := app.Save(rec); err != nil {
		t.Fatalf("save provider: %v", err)
	}
	return rec
}

func createModel(
	t *testing.T,
	app *tests.TestApp,
	provider *core.Record,
	name string,
	priority, contextWindow, maxOutputTokens int,
	enabled bool,
) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("upstream_models")
	if err != nil {
		t.Fatalf("models collection: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("provider", provider.Id)
	rec.Set("name", name)
	rec.Set("display_name", name)
	rec.Set("priority", priority)
	rec.Set("context_window", contextWindow)
	rec.Set("max_output_tokens", maxOutputTokens)
	rec.Set("enabled", enabled)
	if err := app.Save(rec); err != nil {
		t.Fatalf("save model: %v", err)
	}
	return rec
}

func configureUser(
	t *testing.T,
	app *tests.TestApp,
	user *core.Record,
	settings any,
	allowedModels ...*core.Record,
) {
	t.Helper()
	if settings != nil {
		raw, err := json.Marshal(settings)
		if err != nil {
			t.Fatalf("encode settings: %v", err)
		}
		user.Set("settings", types.JSONRaw(raw))
	}
	ids := make([]string, 0, len(allowedModels))
	for _, model := range allowedModels {
		ids = append(ids, model.Id)
	}
	user.Set("allowed_models", ids)
	if err := app.Save(user); err != nil {
		t.Fatalf("save user configuration: %v", err)
	}
}

// buildMux returns an http.Handler with all AIST routes plus PocketBase
// defaults registered against the given app.
func buildMux(t *testing.T, app core.App, srv *API) http.Handler {
	t.Helper()
	r, err := apis.NewRouter(app)
	if err != nil {
		t.Fatalf("apis.NewRouter: %v", err)
	}
	e := &core.ServeEvent{App: app, Router: r}
	srv.RegisterRoutes(e)
	mux, err := r.BuildMux()
	if err != nil {
		t.Fatalf("BuildMux: %v", err)
	}
	return mux
}

// doJSON issues a request through the given handler.
func doJSON(t *testing.T, h http.Handler, method, path, auth, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	if auth != "" {
		r.Header.Set("Authorization", auth)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

type pbAuthResponse struct {
	Token  string `json:"token"`
	Record struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	} `json:"record"`
	Meta struct {
		Config clientAccountConfig `json:"config"`
	} `json:"meta"`
}

func decodePBAuth(t *testing.T, data []byte) pbAuthResponse {
	t.Helper()
	var resp pbAuthResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("decode auth response: %v body=%s", err, data)
	}
	return resp
}

func passwordAuthBody(email, password string) string {
	return fmt.Sprintf(`{"identity":%q,"password":%q}`, email, password)
}

func loginWithPassword(t *testing.T, h http.Handler, email, password string) pbAuthResponse {
	t.Helper()
	w := doJSON(t, h, http.MethodPost, "/api/collections/users/auth-with-password", "", passwordAuthBody(email, password))
	if w.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", w.Code, w.Body.String())
	}
	return decodePBAuth(t, w.Body.Bytes())
}

func setAuthOptions(t *testing.T, app core.App, password, otp, mfa bool) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId(usersCollection)
	if err != nil {
		t.Fatalf("users collection: %v", err)
	}
	col.PasswordAuth.Enabled = password
	col.OTP.Enabled = otp
	col.MFA.Enabled = mfa
	if err := app.Save(col); err != nil {
		t.Fatalf("save users collection: %v", err)
	}
}

func captureOTP(t *testing.T, app core.App) <-chan string {
	t.Helper()
	codes := make(chan string, 1)
	app.OnMailerRecordOTPSend().BindFunc(func(e *core.MailerRecordEvent) error {
		code, _ := e.Meta["password"].(string)
		select {
		case codes <- code:
		default:
		}
		return nil
	})
	return codes
}

func requestOTP(t *testing.T, h http.Handler, email string) string {
	t.Helper()
	w := doJSON(t, h, http.MethodPost, "/api/collections/users/request-otp", "", fmt.Sprintf(`{"email":%q}`, email))
	if w.Code != http.StatusOK {
		t.Fatalf("request otp status = %d body=%s", w.Code, w.Body.String())
	}
	var out struct {
		OTPID string `json:"otpId"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode otp response: %v body=%s", err, w.Body.String())
	}
	if out.OTPID == "" {
		t.Fatal("empty otp id")
	}
	return out.OTPID
}

func authWithOTPBody(otpID, code, mfaID string) (string, string) {
	path := "/api/collections/users/auth-with-otp"
	if mfaID != "" {
		path += "?mfaId=" + url.QueryEscape(mfaID)
	}
	return path, fmt.Sprintf(`{"otpId":%q,"password":%q}`, otpID, code)
}

func waitOTPCode(t *testing.T, codes <-chan string) string {
	t.Helper()
	select {
	case code := <-codes:
		if code == "" {
			t.Fatal("empty otp code")
		}
		return code
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for otp email")
		return ""
	}
}

func TestUsersCollectionLockedDown(t *testing.T) {
	app, _ := setupServer(t)
	col, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("users collection: %v", err)
	}
	if col.ListRule != nil || col.ViewRule != nil || col.CreateRule != nil || col.UpdateRule != nil || col.DeleteRule != nil {
		t.Fatalf("users collection must be admin-only: list=%v view=%v create=%v update=%v delete=%v",
			col.ListRule, col.ViewRule, col.CreateRule, col.UpdateRule, col.DeleteRule)
	}
}

func TestLoginSuccessAndConfig(t *testing.T) {
	app, srv := setupServer(t)
	provider := createProvider(t, app, "deepseek", "https://example.com/v1", "", true)
	first := createModel(t, app, provider, "first", 10, 64000, 4096, true)
	second := createModel(t, app, provider, "second", 20, 32000, 2048, true)
	alice := createUser(t, app, "alice", "pw-secret")
	configureUser(t, app, alice, map[string]any{
		"session": map[string]any{
			"max_sessions": 7,
			"compaction": map[string]any{
				"keep_tokens":   1234,
				"trigger_ratio": 0.7,
			},
		},
		"blacklist": []string{"rm -rf /"},
	}, first, second)

	alice.Set("weekly_token_quota", 100000)
	if err := app.Save(alice); err != nil {
		t.Fatalf("save weekly_token_quota: %v", err)
	}

	h := buildMux(t, app, srv)
	w := doJSON(t, h, http.MethodPost, "/api/collections/users/auth-with-password", "", passwordAuthBody("alice@example.com", "pw-secret"))
	if w.Code != 200 {
		t.Fatalf("login status = %d body=%s", w.Code, w.Body.String())
	}

	var raw map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	recordView, _ := raw["record"].(map[string]any)
	for _, field := range []string{"allowed_models", "weekly_token_quota", "settings"} {
		if _, ok := recordView[field]; ok {
			t.Fatalf("auth record exposes %s: %s", field, w.Body.String())
		}
	}
	if recordView["email"] != "alice@example.com" {
		t.Fatalf("email = %v", recordView["email"])
	}

	resp := decodePBAuth(t, w.Body.Bytes())
	if resp.Token == "" {
		t.Fatal("missing auth token")
	}
	if resp.Record.Email != "alice@example.com" {
		t.Fatalf("record email = %q", resp.Record.Email)
	}

	configJSON, _ := json.Marshal(resp.Meta.Config)
	var view map[string]any
	if err := json.Unmarshal(configJSON, &view); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if _, ok := view["providers"]; ok {
		t.Fatalf("client config exposes providers: %s", configJSON)
	}
	if _, ok := view["language"]; ok {
		t.Fatalf("client config should not expose language: %s", configJSON)
	}
	limits, ok := view["limits"].(map[string]any)
	if !ok {
		t.Fatalf("client config missing limits: %s", configJSON)
	}
	if limits["context_window"] != float64(32000) || limits["max_output_tokens"] != float64(2048) {
		t.Fatalf("limits = %+v", limits)
	}
	session, ok := view["session"].(map[string]any)
	if !ok {
		t.Fatalf("client config missing session: %s", configJSON)
	}
	if session["max_sessions"] != float64(7) {
		t.Fatalf("session = %+v", session)
	}
	compaction, _ := session["compaction"].(map[string]any)
	if compaction["keep_tokens"] != float64(1234) || compaction["trigger_ratio"] != 0.7 {
		t.Fatalf("compaction = %+v", compaction)
	}
	blacklist, _ := view["blacklist"].([]any)
	if len(blacklist) != 1 || blacklist[0] != "rm -rf /" {
		t.Fatalf("blacklist = %+v", blacklist)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	app, srv := setupServer(t)
	createUser(t, app, "bob", "right-pw")

	h := buildMux(t, app, srv)
	w := doJSON(t, h, http.MethodPost, "/api/collections/users/auth-with-password", "", passwordAuthBody("bob@example.com", "wrong"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestLoginRejectsUsernameIdentity(t *testing.T) {
	app, srv := setupServer(t)
	createUser(t, app, "alice", "pw-secret")

	h := buildMux(t, app, srv)
	w := doJSON(t, h, http.MethodPost, "/api/collections/users/auth-with-password", "", `{"identity":"alice","password":"pw-secret"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestRefreshFlow(t *testing.T) {
	app, srv := setupServer(t)
	carol := createUser(t, app, "carol", "carolpw123")
	configureUser(t, app, carol, nil)

	h := buildMux(t, app, srv)
	login := loginWithPassword(t, h, "carol@example.com", "carolpw123")

	w := doJSON(t, h, http.MethodPost, "/api/collections/users/auth-refresh", "Bearer "+login.Token, "")
	if w.Code != 200 {
		t.Fatalf("refresh status = %d body=%s", w.Code, w.Body.String())
	}
	refreshed := decodePBAuth(t, w.Body.Bytes())
	if refreshed.Token == "" {
		t.Fatal("no auth token from refresh")
	}
	if refreshed.Meta.Config.SchemaVersion != 1 {
		t.Fatalf("config not returned: %+v", refreshed.Meta.Config)
	}

	w = doJSON(t, h, http.MethodPost, "/api/collections/users/auth-refresh", "Bearer bogus", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("bogus refresh status = %d, want 401", w.Code)
	}
}

func TestOTPOnlyLogin(t *testing.T) {
	app, srv := setupServer(t)
	user := createUser(t, app, "otp", "unused-password")
	configureUser(t, app, user, nil)
	setAuthOptions(t, app, false, true, false)
	codes := captureOTP(t, app)

	h := buildMux(t, app, srv)
	otpID := requestOTP(t, h, "otp@example.com")
	code := waitOTPCode(t, codes)

	path, body := authWithOTPBody(otpID, code, "")
	w := doJSON(t, h, http.MethodPost, path, "", body)
	if w.Code != http.StatusOK {
		t.Fatalf("otp auth status = %d body=%s", w.Code, w.Body.String())
	}
	resp := decodePBAuth(t, w.Body.Bytes())
	if resp.Token == "" || resp.Record.Email != "otp@example.com" {
		t.Fatalf("resp = %+v", resp)
	}
	if resp.Meta.Config.SchemaVersion != 1 {
		t.Fatalf("config = %+v", resp.Meta.Config)
	}
}

func TestOTPWrongCode(t *testing.T) {
	app, srv := setupServer(t)
	user := createUser(t, app, "otpwrong", "unused-password")
	configureUser(t, app, user, nil)
	setAuthOptions(t, app, false, true, false)
	codes := captureOTP(t, app)

	h := buildMux(t, app, srv)
	otpID := requestOTP(t, h, "otpwrong@example.com")
	_ = waitOTPCode(t, codes)

	path, body := authWithOTPBody(otpID, "00000000", "")
	w := doJSON(t, h, http.MethodPost, path, "", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("otp auth status = %d, want 400 body=%s", w.Code, w.Body.String())
	}
}

func TestPasswordWithMFAOTP(t *testing.T) {
	app, srv := setupServer(t)
	user := createUser(t, app, "mfa", "mfa-password")
	configureUser(t, app, user, nil)
	setAuthOptions(t, app, true, true, true)
	codes := captureOTP(t, app)

	h := buildMux(t, app, srv)
	w := doJSON(t, h, http.MethodPost, "/api/collections/users/auth-with-password", "", passwordAuthBody("mfa@example.com", "mfa-password"))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("password auth status = %d, want 401 body=%s", w.Code, w.Body.String())
	}
	var challenge struct {
		MfaID string `json:"mfaId"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &challenge); err != nil {
		t.Fatalf("decode mfa response: %v body=%s", err, w.Body.String())
	}
	if challenge.MfaID == "" {
		t.Fatal("missing mfaId")
	}

	otpID := requestOTP(t, h, "mfa@example.com")
	code := waitOTPCode(t, codes)

	path, body := authWithOTPBody(otpID, code, challenge.MfaID)
	w = doJSON(t, h, http.MethodPost, path, "", body)
	if w.Code != http.StatusOK {
		t.Fatalf("otp auth status = %d body=%s", w.Code, w.Body.String())
	}
	resp := decodePBAuth(t, w.Body.Bytes())
	if resp.Token == "" {
		t.Fatal("missing auth token")
	}
}

// TestProxyForwardsToUpstream verifies the proxy authenticates, authorizes and
// forwards to the configured upstream provider.
func TestProxyForwardsToUpstream(t *testing.T) {
	var gotAuth, gotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("upstream path = %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		if data, err := io.ReadAll(r.Body); err == nil {
			var body struct {
				Model string `json:"model"`
			}
			_ = json.Unmarshal(data, &body)
			gotModel = body.Model
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hi"}}]}`))
	}))
	defer upstream.Close()

	app, srv := setupServer(t)

	provider := createProvider(t, app, "deepseek", upstream.URL, "sk-upstream", true)
	model := createModel(t, app, provider, "deepseek-chat", 0, 0, 0, true)
	dave := createUser(t, app, "dave", "davepw123")
	configureUser(t, app, dave, nil, model)

	h := buildMux(t, app, srv)

	login := loginWithPassword(t, h, "dave@example.com", "davepw123")

	proxyBody := `{"model":"auto","messages":[{"role":"user","content":"hello"}]}`
	w := doJSON(t, h, http.MethodPost, "/v1/proxy/chat/completions", "Bearer "+login.Token, proxyBody)
	if w.Code != 200 {
		t.Fatalf("proxy status = %d body=%s", w.Code, w.Body.String())
	}
	if gotAuth != "Bearer sk-upstream" {
		t.Fatalf("upstream auth = %q", gotAuth)
	}
	if gotModel != "deepseek-chat" {
		t.Fatalf("upstream model = %q, want bare name", gotModel)
	}
}

func TestProxyRequiresValidToken(t *testing.T) {
	app, srv := setupServer(t)
	h := buildMux(t, app, srv)
	w := doJSON(t, h, http.MethodPost, "/v1/proxy/chat/completions", "Bearer bad.token.here", `{"model":"auto","messages":[]}`)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestProxyAutoModelFallback(t *testing.T) {
	var models []string
	var maxTokens []int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model     string `json:"model"`
			MaxTokens int    `json:"max_tokens"`
		}
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)
		models = append(models, body.Model)
		maxTokens = append(maxTokens, body.MaxTokens)
		if body.Model == "first" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer upstream.Close()

	app, srv := setupServer(t)

	provider := createProvider(t, app, "p", upstream.URL, "sk-upstream", true)
	first := createModel(t, app, provider, "first", 10, 0, 4096, true)
	second := createModel(t, app, provider, "second", 20, 0, 2048, true)
	auto := createUser(t, app, "auto", "autopw123")
	configureUser(t, app, auto, nil, second, first)

	h := buildMux(t, app, srv)
	login := loginWithPassword(t, h, "auto@example.com", "autopw123")

	w := doJSON(t, h, http.MethodPost, "/v1/proxy/chat/completions", "Bearer "+login.Token,
		`{"model":"auto","max_tokens":99999,"messages":[{"role":"user","content":"hello"}]}`)
	if w.Code != 200 {
		t.Fatalf("proxy status = %d body=%s", w.Code, w.Body.String())
	}
	if len(models) != 2 || models[0] != "first" || models[1] != "second" {
		t.Fatalf("upstream models = %+v", models)
	}
	if maxTokens[0] != 4096 || maxTokens[1] != 2048 {
		t.Fatalf("max_tokens = %+v", maxTokens)
	}
}

func TestProxyModelPriorityOrdering(t *testing.T) {
	var tried []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)
		tried = append(tried, body.Model)
		if body.Model != "aaa-paid" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer upstream.Close()

	app, srv := setupServer(t)

	alpha := createProvider(t, app, "alpha", upstream.URL, "sk-alpha", true)
	beta := createProvider(t, app, "beta", upstream.URL, "sk-beta", true)
	zeta := createProvider(t, app, "zeta", upstream.URL, "sk-zeta", true)

	paid := createModel(t, app, alpha, "aaa-paid", 100, 0, 0, true)
	trial := createModel(t, app, zeta, "zzz-trial", 20, 0, 0, true)
	freeZeta := createModel(t, app, zeta, "free-b", 10, 0, 0, true)
	freeBetaC := createModel(t, app, beta, "free-c", 10, 0, 0, true)
	freeBetaA := createModel(t, app, beta, "free-a", 10, 0, 0, true)
	disabledTop := createModel(t, app, alpha, "disabled-top", 1, 0, 0, false)

	user := createUser(t, app, "prio", "priopw123")
	configureUser(t, app, user, nil, paid, trial, freeZeta, freeBetaC, freeBetaA, disabledTop)

	h := buildMux(t, app, srv)
	login := loginWithPassword(t, h, "prio@example.com", "priopw123")

	w := doJSON(t, h, http.MethodPost, "/v1/proxy/chat/completions", "Bearer "+login.Token,
		`{"model":"auto","messages":[{"role":"user","content":"hello"}]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("proxy status = %d body=%s", w.Code, w.Body.String())
	}

	want := []string{"free-a", "free-c", "free-b", "zzz-trial", "aaa-paid"}
	if strings.Join(tried, ",") != strings.Join(want, ",") {
		t.Fatalf("tried models = %v, want %v", tried, want)
	}
}

func TestProxyAutoModelAllFail(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"model exploded"}`))
	}))
	defer upstream.Close()

	app, srv := setupServer(t)

	provider := createProvider(t, app, "p", upstream.URL, "sk-upstream", true)
	only := createModel(t, app, provider, "only", 0, 0, 0, true)
	fail := createUser(t, app, "fail", "failpw123")
	configureUser(t, app, fail, nil, only)

	h := buildMux(t, app, srv)
	login := loginWithPassword(t, h, "fail@example.com", "failpw123")

	w := doJSON(t, h, http.MethodPost, "/v1/proxy/chat/completions", "Bearer "+login.Token,
		`{"model":"auto","messages":[{"role":"user","content":"hello"}]}`)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("proxy status = %d body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "model exploded") {
		t.Fatalf("upstream error leaked: %s", w.Body.String())
	}
}

func TestProxyBadRequestDoesNotFallback(t *testing.T) {
	var calls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"missing tool_call_id secret detail"}`))
	}))
	defer upstream.Close()

	app, srv := setupServer(t)

	provider := createProvider(t, app, "p", upstream.URL, "sk-upstream", true)
	first := createModel(t, app, provider, "first", 10, 0, 0, true)
	second := createModel(t, app, provider, "second", 20, 0, 0, true)
	user := createUser(t, app, "badreq", "badreqpw123")
	configureUser(t, app, user, nil, first, second)

	h := buildMux(t, app, srv)
	login := loginWithPassword(t, h, "badreq@example.com", "badreqpw123")

	w := doJSON(t, h, http.MethodPost, "/v1/proxy/chat/completions", "Bearer "+login.Token,
		`{"model":"auto","messages":[{"role":"user","content":"hello"}]}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("proxy status = %d body=%s", w.Code, w.Body.String())
	}
	if calls != 1 {
		t.Fatalf("upstream calls = %d, want 1 (400 must not trigger fallback)", calls)
	}
	if strings.Contains(w.Body.String(), "secret detail") {
		t.Fatalf("upstream error leaked: %s", w.Body.String())
	}
}
