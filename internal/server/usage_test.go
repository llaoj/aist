package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pocketbase/dbx"
)

func TestParseUpstreamUsage(t *testing.T) {
	tests := []struct {
		name string
		body string
		want tokenUsage
	}{
		{
			name: "standard_usage",
			body: `{"usage":{"prompt_tokens":120,"completion_tokens":45,"total_tokens":165}}`,
			want: tokenUsage{PromptTokens: 120, CompletionTokens: 45, TotalTokens: 165},
		},
		{
			name: "computes_total_when_zero",
			body: `{"usage":{"prompt_tokens":80,"completion_tokens":20}}`,
			want: tokenUsage{PromptTokens: 80, CompletionTokens: 20, TotalTokens: 100},
		},
		{
			name: "missing_usage_returns_zeros",
			body: `{"choices":[{"message":{"role":"assistant","content":"hi"}}]}`,
			want: tokenUsage{},
		},
		{
			name: "clamps_negative_values",
			body: `{"usage":{"prompt_tokens":-5,"completion_tokens":15,"total_tokens":0}}`,
			want: tokenUsage{PromptTokens: 0, CompletionTokens: 15, TotalTokens: 15},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseUpstreamUsage([]byte(tt.body)); got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDailyModelUsageAggregationAndLifecycle(t *testing.T) {
	now := time.Date(2026, 11, 19, 8, 30, 0, 0, time.UTC)
	failPrimaryOnce := true
	callIndex := 0

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)

		if body.Model == "m-primary" && failPrimaryOnce {
			failPrimaryOnce = false
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		callIndex++
		switch callIndex {
		case 1:
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"1"}}],"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}}`))
		case 2:
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"2"}}],"usage":{"prompt_tokens":40,"completion_tokens":10}}`))
		case 3:
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"3"}}],"usage":{"prompt_tokens":60,"completion_tokens":20,"total_tokens":80}}`))
		default:
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"4"}}],"usage":{"prompt_tokens":25,"completion_tokens":5,"total_tokens":30}}`))
		}
	}))
	defer upstream.Close()

	app, srv := setupServerWithOptions(t, APIOptions{Now: func() time.Time { return now }})
	provider := createProvider(t, app, "deepseek", upstream.URL, "sk-test", true)
	primary := createModel(t, app, provider, "m-primary", 10, 0, 0, true)
	backup := createModel(t, app, provider, "m-backup", 20, 0, 0, true)
	user := createUser(t, app, "meter", "meterpw1234")
	configureUser(t, app, user, nil, primary, backup)

	h := buildMux(t, app, srv)
	login := loginWithPassword(t, h, "meter@example.com", "meterpw1234")
	reqBody := `{"model":"auto","messages":[{"role":"user","content":"hi"}]}`

	// 1. Day 1: m-primary 500s -> falls back to m-backup (150 tokens). Only m-backup is recorded.
	if w := doJSON(t, h, http.MethodPost, "/v1/proxy/chat/completions", "Bearer "+login.Token, reqBody); w.Code != http.StatusOK {
		t.Fatalf("req 1 status = %d", w.Code)
	}
	// 2. Day 1 (after transient cooldown expires): m-primary succeeds twice (50 + 80 = 130 tokens, 2 requests).
	now = now.Add(defaultTransientCooldown + time.Second)
	if w := doJSON(t, h, http.MethodPost, "/v1/proxy/chat/completions", "Bearer "+login.Token, reqBody); w.Code != http.StatusOK {
		t.Fatalf("req 2 status = %d", w.Code)
	}
	if w := doJSON(t, h, http.MethodPost, "/v1/proxy/chat/completions", "Bearer "+login.Token, reqBody); w.Code != http.StatusOK {
		t.Fatalf("req 3 status = %d", w.Code)
	}
	// 3. Day 2: m-primary succeeds once (30 tokens).
	now = now.Add(24 * time.Hour)
	if w := doJSON(t, h, http.MethodPost, "/v1/proxy/chat/completions", "Bearer "+login.Token, reqBody); w.Code != http.StatusOK {
		t.Fatalf("req 4 status = %d", w.Code)
	}

	records, err := app.FindAllRecords(dailyModelUsageCollection, dbx.HashExp{"user": user.Id})
	if err != nil {
		t.Fatalf("FindAllRecords: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("usage row count = %d, want 3", len(records))
	}

	byKey := make(map[string]struct {
		requests, prompt, completion, total int
	})
	for _, rec := range records {
		key := rec.GetString("provider") + "/" + rec.GetString("model") + "@" + rec.GetString("date")
		byKey[key] = struct {
			requests, prompt, completion, total int
		}{
			requests:   rec.GetInt("requests"),
			prompt:     rec.GetInt("prompt_tokens"),
			completion: rec.GetInt("completion_tokens"),
			total:      rec.GetInt("total_tokens"),
		}
	}

	if got := byKey["deepseek/m-backup@2026-11-19"]; got.requests != 1 || got.total != 150 {
		t.Fatalf("m-backup day1 = %+v", got)
	}
	if got := byKey["deepseek/m-primary@2026-11-19"]; got.requests != 2 || got.prompt != 100 || got.completion != 30 || got.total != 130 {
		t.Fatalf("m-primary day1 = %+v", got)
	}
	if got := byKey["deepseek/m-primary@2026-11-20"]; got.requests != 1 || got.total != 30 {
		t.Fatalf("m-primary day2 = %+v", got)
	}

	// Deleting both models must succeed and preserve historical usage rows.
	if err := app.Delete(primary); err != nil {
		t.Fatalf("delete primary model: %v", err)
	}
	if err := app.Delete(backup); err != nil {
		t.Fatalf("delete backup model: %v", err)
	}
	records, _ = app.FindAllRecords(dailyModelUsageCollection, dbx.HashExp{"user": user.Id})
	if len(records) != 3 {
		t.Fatalf("expected 3 usage rows preserved after deleting models, got %d", len(records))
	}

	// Deleting the user cascade-deletes their usage rows.
	if err := app.Delete(user); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	records, _ = app.FindAllRecords(dailyModelUsageCollection, dbx.HashExp{"user": user.Id})
	if len(records) != 0 {
		t.Fatalf("expected 0 usage rows after deleting user, got %d", len(records))
	}
}

func TestCurrentUTCWeekWindow(t *testing.T) {
	tests := []struct {
		name      string
		now       time.Time
		wantStart string
		wantNext  string
		wantReset time.Time
	}{
		{
			name:      "monday_00_00",
			now:       time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC),
			wantStart: "2026-11-16",
			wantNext:  "2026-11-23",
			wantReset: time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "thursday_midday",
			now:       time.Date(2026, 11, 19, 12, 30, 0, 0, time.UTC),
			wantStart: "2026-11-16",
			wantNext:  "2026-11-23",
			wantReset: time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "sunday_23_59",
			now:       time.Date(2026, 11, 22, 23, 59, 59, 0, time.UTC),
			wantStart: "2026-11-16",
			wantNext:  "2026-11-23",
			wantReset: time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, next, reset := currentUTCWeekWindow(tt.now)
			if start != tt.wantStart || next != tt.wantNext || !reset.Equal(tt.wantReset) {
				t.Fatalf("got (%s, %s, %v), want (%s, %s, %v)",
					start, next, reset, tt.wantStart, tt.wantNext, tt.wantReset)
			}
		})
	}
}

func TestWeeklyTokenQuotaEnforcementAndMondayReset(t *testing.T) {
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC) // Thursday
	var upstreamCalls int

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":40,"completion_tokens":20,"total_tokens":60}}`))
	}))
	defer upstream.Close()

	app, srv := setupServerWithOptions(t, APIOptions{Now: func() time.Time { return now }})
	provider := createProvider(t, app, "deepseek", upstream.URL, "sk-test", true)
	m1 := createModel(t, app, provider, "m1", 10, 0, 0, true)
	m2 := createModel(t, app, provider, "m2", 20, 0, 0, true)

	capped := createUser(t, app, "capped", "cappedpw123")
	configureUser(t, app, capped, nil, m1, m2)
	capped.Set("weekly_token_quota", 100)
	if err := app.Save(capped); err != nil {
		t.Fatalf("save capped user: %v", err)
	}

	unlimited := createUser(t, app, "unlim", "unlimpw1234")
	configureUser(t, app, unlimited, nil, m1)

	h := buildMux(t, app, srv)
	cappedLogin := loginWithPassword(t, h, "capped@example.com", "cappedpw123")
	unlimLogin := loginWithPassword(t, h, "unlim@example.com", "unlimpw1234")
	reqBody := `{"model":"auto","messages":[{"role":"user","content":"hi"}]}`

	// 1. Thursday: 60 tokens on m1 (< 100 quota) -> allowed.
	if w := doJSON(t, h, http.MethodPost, "/v1/proxy/chat/completions", "Bearer "+cappedLogin.Token, reqBody); w.Code != http.StatusOK {
		t.Fatalf("req 1 status = %d", w.Code)
	}

	// 2. Friday: switch priority so m2 runs (60 + 60 = 120 >= 100 across models) -> allowed, crosses quota.
	now = time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	m2.Set("priority", 5)
	_ = app.Save(m2)
	if w := doJSON(t, h, http.MethodPost, "/v1/proxy/chat/completions", "Bearer "+cappedLogin.Token, reqBody); w.Code != http.StatusOK {
		t.Fatalf("req 2 status = %d", w.Code)
	}

	// 3. Sunday 23:59: quota exceeded -> rejected with 429 and retry_at = 2026-11-23T00:00:00Z, 0 upstream calls.
	now = time.Date(2026, 11, 22, 23, 59, 0, 0, time.UTC)
	callsBeforeReject := upstreamCalls
	w := doJSON(t, h, http.MethodPost, "/v1/proxy/chat/completions", "Bearer "+cappedLogin.Token, reqBody)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("req 3 status = %d, want 429 body=%s", w.Code, w.Body.String())
	}
	if upstreamCalls != callsBeforeReject {
		t.Fatalf("upstream called despite exceeded quota")
	}
	var errResp struct {
		Error struct {
			Code    string `json:"code"`
			RetryAt string `json:"retry_at"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("decode 429 body: %v", err)
	}
	if errResp.Error.Code != "quota_exceeded" || errResp.Error.RetryAt != "2026-11-23T00:00:00Z" {
		t.Fatalf("429 error = %+v", errResp.Error)
	}

	// Unlimited user (weekly_token_quota == 0) is unaffected.
	for range 3 {
		if uw := doJSON(t, h, http.MethodPost, "/v1/proxy/chat/completions", "Bearer "+unlimLogin.Token, reqBody); uw.Code != http.StatusOK {
			t.Fatalf("unlimited user status = %d", uw.Code)
		}
	}

	// 4. Next Monday 00:00:00 UTC -> quota automatically resets, capped user succeeds again.
	now = time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)
	w = doJSON(t, h, http.MethodPost, "/v1/proxy/chat/completions", "Bearer "+cappedLogin.Token, reqBody)
	if w.Code != http.StatusOK {
		t.Fatalf("after Monday UTC reset: status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestAccountUsageEndpoint(t *testing.T) {
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC) // Thursday

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":40,"completion_tokens":20,"total_tokens":60}}`))
	}))
	defer upstream.Close()

	app, srv := setupServerWithOptions(t, APIOptions{Now: func() time.Time { return now }})
	provider := createProvider(t, app, "deepseek", upstream.URL, "sk-test", true)
	m1 := createModel(t, app, provider, "m1", 10, 0, 0, true)

	capped := createUser(t, app, "usagecap", "cappedpw123")
	configureUser(t, app, capped, nil, m1)
	capped.Set("weekly_token_quota", 100)
	if err := app.Save(capped); err != nil {
		t.Fatalf("save capped user: %v", err)
	}

	unlimited := createUser(t, app, "usageunlim", "unlimpw1234")
	configureUser(t, app, unlimited, nil, m1)

	h := buildMux(t, app, srv)

	if w := doJSON(t, h, http.MethodGet, "/v1/account/usage", "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401", w.Code)
	}

	cappedLogin := loginWithPassword(t, h, "usagecap@example.com", "cappedpw123")
	unlimLogin := loginWithPassword(t, h, "usageunlim@example.com", "unlimpw1234")
	reqBody := `{"model":"auto","messages":[{"role":"user","content":"hi"}]}`

	w := doJSON(t, h, http.MethodGet, "/v1/account/usage", "Bearer "+cappedLogin.Token, "")
	if w.Code != http.StatusOK {
		t.Fatalf("initial usage status = %d body=%s", w.Code, w.Body.String())
	}
	var got accountUsageResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode initial usage: %v", err)
	}
	if got.WeekStart != "2026-11-16" || got.ResetAt != "2026-11-23T00:00:00Z" || got.UsedTokens != 0 || got.Requests != 0 || got.WeeklyTokenQuota != 100 || got.RemainingTokens == nil || *got.RemainingTokens != 100 {
		t.Fatalf("initial usage = %+v", got)
	}

	// Non-nil *int(0) ensures capped accounts still expose remaining_tokens: 0 when exhausted.
	for range 2 {
		if pw := doJSON(t, h, http.MethodPost, "/v1/proxy/chat/completions", "Bearer "+cappedLogin.Token, reqBody); pw.Code != http.StatusOK {
			t.Fatalf("proxy status = %d", pw.Code)
		}
	}
	w = doJSON(t, h, http.MethodGet, "/v1/account/usage", "Bearer "+cappedLogin.Token, "")
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode over-quota usage: %v", err)
	}
	if got.UsedTokens != 120 || got.PromptTokens != 80 || got.CompletionTokens != 40 || got.Requests != 2 || got.RemainingTokens == nil || *got.RemainingTokens != 0 {
		t.Fatalf("over-quota usage = %+v body=%s", got, w.Body.String())
	}

	if pw := doJSON(t, h, http.MethodPost, "/v1/proxy/chat/completions", "Bearer "+unlimLogin.Token, reqBody); pw.Code != http.StatusOK {
		t.Fatalf("unlimited proxy status = %d", pw.Code)
	}
	w = doJSON(t, h, http.MethodGet, "/v1/account/usage", "Bearer "+unlimLogin.Token, "")
	var unlimGot accountUsageResponse
	if err := json.Unmarshal(w.Body.Bytes(), &unlimGot); err != nil {
		t.Fatalf("decode unlimited usage: %v", err)
	}
	if unlimGot.UsedTokens != 60 || unlimGot.Requests != 1 || unlimGot.WeeklyTokenQuota != 0 || unlimGot.RemainingTokens != nil {
		t.Fatalf("unlimited usage = %+v body=%s", unlimGot, w.Body.String())
	}

	now = time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)
	w = doJSON(t, h, http.MethodGet, "/v1/account/usage", "Bearer "+cappedLogin.Token, "")
	var resetGot accountUsageResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resetGot); err != nil {
		t.Fatalf("decode reset usage: %v", err)
	}
	if resetGot.WeekStart != "2026-11-23" || resetGot.ResetAt != "2026-11-30T00:00:00Z" || resetGot.UsedTokens != 0 || resetGot.RemainingTokens == nil || *resetGot.RemainingTokens != 100 {
		t.Fatalf("reset usage = %+v", resetGot)
	}
}
