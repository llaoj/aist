package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/types"
)

func createInviteCode(
	t *testing.T,
	app *tests.TestApp,
	code string,
	enabled bool,
	maxUses int,
	expiresAt time.Time,
	weeklyQuota int,
	settings any,
	allowedModels ...*core.Record,
) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId(inviteCodesCollection)
	if err != nil {
		t.Fatalf("invite_codes collection: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("code", code)
	rec.Set("enabled", enabled)
	rec.Set("max_uses", maxUses)
	rec.Set("weekly_token_quota", weeklyQuota)
	if !expiresAt.IsZero() {
		dt, _ := types.ParseDateTime(expiresAt.UTC())
		rec.Set("expires_at", dt)
	}
	if settings != nil {
		raw, err := json.Marshal(settings)
		if err != nil {
			t.Fatalf("marshal settings: %v", err)
		}
		rec.Set("settings", types.JSONRaw(raw))
	}
	modelIDs := make([]string, 0, len(allowedModels))
	for _, m := range allowedModels {
		modelIDs = append(modelIDs, m.Id)
	}
	rec.Set("allowed_models", modelIDs)
	if err := app.Save(rec); err != nil {
		t.Fatalf("save invite code: %v", err)
	}
	return rec
}

func postJSONRequest(t *testing.T, h http.Handler, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRegisterWithoutSMTPCreatesConfiguredUser(t *testing.T) {
	now := time.Date(2026, 11, 20, 4, 0, 0, 0, time.UTC)
	app, srv := setupServerWithOptions(t, APIOptions{Now: func() time.Time { return now }})
	h := buildMux(t, app, srv)

	provider := createProvider(t, app, "deepseek", "https://api.example.com/v1", "sk-test", true)
	model := createModel(t, app, provider, "deepseek-chat", 10, 64000, 4096, true)
	invite := createInviteCode(
		t,
		app,
		"TEAM2026",
		true,
		1,
		now.Add(24*time.Hour),
		500000,
		map[string]any{"blacklist": []string{"rm -rf /"}},
		model,
	)

	res := postJSONRequest(t, h, "/v1/register", map[string]any{
		"invite_code": " TEAM2026 ",
		"email":       "Alice@Example.com",
		"password":    "secret1234",
	})
	if res.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}

	var body registerResponse
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Email != "alice@example.com" || body.VerificationSent {
		t.Fatalf("unexpected response: %+v", body)
	}

	updatedInvite, err := app.FindRecordById(inviteCodesCollection, invite.Id)
	if err != nil {
		t.Fatalf("reload invite: %v", err)
	}
	if got := updatedInvite.GetInt("used_count"); got != 1 {
		t.Fatalf("used_count = %d, want 1", got)
	}

	user, err := app.FindAuthRecordByEmail(usersCollection, "alice@example.com")
	if err != nil {
		t.Fatalf("find created user: %v", err)
	}
	if got := user.GetInt("weekly_token_quota"); got != 500000 {
		t.Fatalf("weekly_token_quota = %d, want 500000", got)
	}
	if models := user.GetStringSlice("allowed_models"); len(models) != 1 || models[0] != model.Id {
		t.Fatalf("allowed_models = %v, want [%s]", models, model.Id)
	}

	loginRes := postJSONRequest(t, h, "/api/collections/users/auth-with-password", map[string]any{
		"identity": "alice@example.com",
		"password": "secret1234",
	})
	if loginRes.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", loginRes.Code, loginRes.Body.String())
	}
	if !strings.Contains(loginRes.Body.String(), `"rm -rf /"`) {
		t.Fatalf("expected blacklist in login meta config, got %s", loginRes.Body.String())
	}

	exhaustedRes := postJSONRequest(t, h, "/v1/register", map[string]any{
		"invite_code": "TEAM2026",
		"email":       "bob@example.com",
		"password":    "secret1234",
	})
	if exhaustedRes.Code != http.StatusBadRequest || !strings.Contains(exhaustedRes.Body.String(), registerErrorCodeInvalidInviteCode) {
		t.Fatalf("expected exhausted invite error, got %d %s", exhaustedRes.Code, exhaustedRes.Body.String())
	}
}

func TestRegisterWithSMTPSendsNativePocketBaseVerificationEmail(t *testing.T) {
	now := time.Date(2026, 11, 20, 4, 0, 0, 0, time.UTC)
	app, srv := setupServerWithOptions(t, APIOptions{Now: func() time.Time { return now }})

	app.Settings().SMTP.Enabled = true
	app.Settings().Meta.AppName = "AIST"
	app.Settings().Meta.AppURL = "https://aistrun.cc"
	app.Settings().Meta.SenderName = "AIST"
	app.Settings().Meta.SenderAddress = "noreply@aistrun.cc"
	if err := app.Save(app.Settings()); err != nil {
		t.Fatalf("save settings: %v", err)
	}

	usersCol, err := app.FindCollectionByNameOrId(usersCollection)
	if err != nil {
		t.Fatalf("users collection: %v", err)
	}
	verifiedRule := "verified = true"
	usersCol.AuthRule = &verifiedRule
	if err := app.Save(usersCol); err != nil {
		t.Fatalf("save users authRule: %v", err)
	}

	h := buildMux(t, app, srv)
	provider := createProvider(t, app, "deepseek", "https://api.example.com/v1", "sk-test", true)
	model := createModel(t, app, provider, "deepseek-chat", 10, 64000, 4096, true)
	createInviteCode(t, app, "TRIAL001", true, 1, time.Time{}, 200000, nil, model)

	res := postJSONRequest(t, h, "/v1/register", map[string]any{
		"invite_code": "TRIAL001",
		"email":       "trial@example.com",
		"password":    "secret1234",
	})
	if res.Code != http.StatusCreated {
		t.Fatalf("register status = %d, body = %s", res.Code, res.Body.String())
	}
	var resp registerResponse
	if err := json.Unmarshal(res.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.VerificationSent || resp.Email != "trial@example.com" {
		t.Fatalf("unexpected response: %+v", resp)
	}

	unverifiedLogin := postJSONRequest(t, h, "/api/collections/users/auth-with-password", map[string]any{
		"identity": "trial@example.com",
		"password": "secret1234",
	})
	if unverifiedLogin.Code == http.StatusOK {
		t.Fatal("expected unverified user login to be blocked when authRule requires verified = true")
	}

	tokenRe := regexp.MustCompile(`confirm-verification/([A-Za-z0-9._-]+)`)
	matches := tokenRe.FindStringSubmatch(app.TestMailer.LastMessage().HTML)
	if len(matches) < 2 {
		t.Fatalf("failed to extract verification token from email HTML: %s", app.TestMailer.LastMessage().HTML)
	}

	confirmRes := postJSONRequest(t, h, "/api/collections/users/confirm-verification", map[string]any{
		"token": matches[1],
	})
	if confirmRes.Code != http.StatusNoContent {
		t.Fatalf("confirm-verification status = %d, body = %s", confirmRes.Code, confirmRes.Body.String())
	}

	verifiedLogin := postJSONRequest(t, h, "/api/collections/users/auth-with-password", map[string]any{
		"identity": "trial@example.com",
		"password": "secret1234",
	})
	if verifiedLogin.Code != http.StatusOK {
		t.Fatalf("verified user login status = %d, body = %s", verifiedLogin.Code, verifiedLogin.Body.String())
	}
}

func TestRegisterValidationDoesNotConsumeInvite(t *testing.T) {
	now := time.Date(2026, 11, 20, 4, 0, 0, 0, time.UTC)
	app, srv := setupServerWithOptions(t, APIOptions{Now: func() time.Time { return now }})
	h := buildMux(t, app, srv)

	createInviteCode(t, app, "DISABLED", false, 0, time.Time{}, 0, nil)
	createInviteCode(t, app, "EXPIRED1", true, 0, now.Add(-time.Minute), 0, nil)
	validInvite := createInviteCode(t, app, "VALID123", true, 5, time.Time{}, 0, nil)
	createUser(t, app, "existing", "secret1234")

	cases := []struct {
		name       string
		payload    map[string]any
		wantStatus int
		wantCode   string
	}{
		{
			name:       "disabled invite",
			payload:    map[string]any{"invite_code": "DISABLED", "email": "a@example.com", "password": "password123"},
			wantStatus: http.StatusBadRequest,
			wantCode:   registerErrorCodeInvalidInviteCode,
		},
		{
			name:       "expired invite",
			payload:    map[string]any{"invite_code": "EXPIRED1", "email": "a@example.com", "password": "password123"},
			wantStatus: http.StatusBadRequest,
			wantCode:   registerErrorCodeInvalidInviteCode,
		},
		{
			name:       "invalid email",
			payload:    map[string]any{"invite_code": "VALID123", "email": "Alice <a@example.com>", "password": "password123"},
			wantStatus: http.StatusBadRequest,
			wantCode:   registerErrorCodeInvalidEmail,
		},
		{
			name:       "localhost email without domain dot",
			payload:    map[string]any{"invite_code": "VALID123", "email": "alice@localhost", "password": "password123"},
			wantStatus: http.StatusBadRequest,
			wantCode:   registerErrorCodeInvalidEmail,
		},
		{
			name:       "short password",
			payload:    map[string]any{"invite_code": "VALID123", "email": "a@example.com", "password": "short"},
			wantStatus: http.StatusBadRequest,
			wantCode:   registerErrorCodeInvalidPassword,
		},
		{
			name:       "multibyte password under 8 runes",
			payload:    map[string]any{"invite_code": "VALID123", "email": "a@example.com", "password": "你好世界"},
			wantStatus: http.StatusBadRequest,
			wantCode:   registerErrorCodeInvalidPassword,
		},
		{
			name:       "duplicate email",
			payload:    map[string]any{"invite_code": "VALID123", "email": "existing@example.com", "password": "password123"},
			wantStatus: http.StatusConflict,
			wantCode:   registerErrorCodeEmailTaken,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := postJSONRequest(t, h, "/v1/register", tc.payload)
			if res.Code != tc.wantStatus || !strings.Contains(res.Body.String(), tc.wantCode) {
				t.Fatalf("got %d %s, want %d with %s", res.Code, res.Body.String(), tc.wantStatus, tc.wantCode)
			}
		})
	}

	reloadedInvite, _ := app.FindRecordById(inviteCodesCollection, validInvite.Id)
	if got := reloadedInvite.GetInt("used_count"); got != 0 {
		t.Fatalf("failed validations consumed invite: used_count = %d", got)
	}
}

func TestRegisterPageServesInjectedServerURL(t *testing.T) {
	app, srv := setupServer(t)
	app.Settings().Meta.AppURL = "https://aist.example.org/"
	if err := app.Save(app.Settings()); err != nil {
		t.Fatalf("save AppURL: %v", err)
	}
	h := buildMux(t, app, srv)

	for _, path := range []string{"/register", "/register.html"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200", path, rec.Code)
		}
		body := rec.Body.String()
		if strings.Contains(body, "__AIST_") {
			t.Fatalf("GET %s contains unreplaced placeholder", path)
		}
		if !strings.Contains(body, "https://aist.example.org/run.sh") {
			t.Fatalf("GET %s missing injected install URL", path)
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
			t.Fatalf("GET %s Cache-Control = %q, want no-cache", path, got)
		}
		if got := rec.Header().Get("Content-Security-Policy"); !strings.Contains(got, "script-src 'self'") || !strings.Contains(got, "frame-ancestors 'none'") {
			t.Fatalf("GET %s Content-Security-Policy = %q", path, got)
		}
		if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
			t.Fatalf("GET %s X-Content-Type-Options = %q, want nosniff", path, got)
		}
		if got := rec.Header().Get("X-Frame-Options"); got != "SAMEORIGIN" {
			t.Fatalf("GET %s X-Frame-Options = %q, want SAMEORIGIN", path, got)
		}
		if got := rec.Header().Get("Referrer-Policy"); got != "same-origin" {
			t.Fatalf("GET %s Referrer-Policy = %q, want same-origin", path, got)
		}
	}
}

func TestRegisterRejectsNonJSONContentTypeAndUnsafeScriptURL(t *testing.T) {
	app, srv := setupServer(t)
	h := buildMux(t, app, srv)

	for _, contentType := range []string{"", "text/plain", "application/x-www-form-urlencoded"} {
		req := httptest.NewRequest(http.MethodPost, "/v1/register", strings.NewReader(`{"invite_code":"X"}`))
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("POST /v1/register (%q) status = %d, want 415", contentType, rec.Code)
		}
	}

	oversized := `{"invite_code":"` + strings.Repeat("A", int(maxRegisterBodyBytes)+128) + `"}`
	bigReq := httptest.NewRequest(http.MethodPost, "/v1/register", strings.NewReader(oversized))
	bigReq.Header.Set("Content-Type", "application/json")
	bigRec := httptest.NewRecorder()
	h.ServeHTTP(bigRec, bigReq)
	if bigRec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized POST /v1/register status = %d, want 413", bigRec.Code)
	}

	if !isSafeScriptURL("https://aist.example.org") || !isSafeScriptURL("http://127.0.0.1:8090/sub") {
		t.Fatal("expected clean http/https URLs to be safe")
	}
	for _, bad := range []string{"", "ftp://example.com", `https://example.com";rm`, "https://example.com/$(id)", "https://example.com\n"} {
		if isSafeScriptURL(bad) {
			t.Fatalf("expected %q to be rejected by isSafeScriptURL", bad)
		}
	}
}
