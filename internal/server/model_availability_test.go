package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/tools/types"
)

func TestModelQuotaCooldownBackoffAndRecovery(t *testing.T) {
	t0 := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	now := t0
	freeHealthy := false
	var tried []string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)
		tried = append(tried, body.Model)

		if body.Model == "free" && !freeHealthy {
			w.WriteHeader(http.StatusPaymentRequired)
			_, _ = w.Write([]byte(`{"error":{"message":"Insufficient Balance"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer upstream.Close()

	app, srv := setupServerWithOptions(t, APIOptions{Now: func() time.Time { return now }})
	provider := createProvider(t, app, "p", upstream.URL, "sk-test", true)
	free := createModel(t, app, provider, "free", 10, 64000, 4096, true)
	paid := createModel(t, app, provider, "paid", 100, 128000, 8192, true)
	user := createUser(t, app, "quotauser", "quotapw123")
	configureUser(t, app, user, nil, free, paid)

	h := buildMux(t, app, srv)
	login := loginWithPassword(t, h, "quotauser@example.com", "quotapw123")
	reqBody := `{"model":"auto","messages":[{"role":"user","content":"hi"}]}`

	// 1. First 402 -> falls back to paid, cools down free for defaultQuotaCooldown (6h).
	w := doJSON(t, h, http.MethodPost, "/v1/proxy/chat/completions", "Bearer "+login.Token, reqBody)
	if w.Code != http.StatusOK {
		t.Fatalf("req 1 status = %d body=%s", w.Code, w.Body.String())
	}
	if len(tried) != 2 || tried[0] != "free" || tried[1] != "paid" {
		t.Fatalf("req 1 tried = %v, want [free paid]", tried)
	}

	freeRec, _ := app.FindRecordById(upstreamModelsCollection, free.Id)
	wantFirstDeadline := t0.Add(defaultQuotaCooldown)
	if freeRec.GetString("unavailable_reason") != modelUnavailableReasonQuota {
		t.Fatalf("reason = %q, want quota", freeRec.GetString("unavailable_reason"))
	}
	if !freeRec.GetDateTime("unavailable_until").Time().Equal(wantFirstDeadline) {
		t.Fatalf("until = %v, want %v", freeRec.GetDateTime("unavailable_until").Time(), wantFirstDeadline)
	}
	if freeRec.GetString("last_error") != "Insufficient Balance" {
		t.Fatalf("last_error = %q", freeRec.GetString("last_error"))
	}

	// Login/refresh limits still reflect all configured models even while free is cooling down.
	refreshed := loginWithPassword(t, h, "quotauser@example.com", "quotapw123")
	if refreshed.Meta.Config.Limits.ContextWindow != 64000 {
		t.Fatalf("limits context_window = %d, want 64000", refreshed.Meta.Config.Limits.ContextWindow)
	}

	// 2. Within 6h cooldown -> free is skipped directly, only paid is called.
	tried = nil
	now = t0.Add(2 * time.Hour)
	w = doJSON(t, h, http.MethodPost, "/v1/proxy/chat/completions", "Bearer "+login.Token, reqBody)
	if w.Code != http.StatusOK {
		t.Fatalf("req 2 status = %d", w.Code)
	}
	if len(tried) != 1 || tried[0] != "paid" {
		t.Fatalf("req 2 tried = %v, want [paid]", tried)
	}

	// 3. After 6h cooldown expires -> free is probed, still 402 -> backs off for backoffQuotaCooldown (24h).
	tried = nil
	tProbe := wantFirstDeadline.Add(time.Minute)
	now = tProbe
	w = doJSON(t, h, http.MethodPost, "/v1/proxy/chat/completions", "Bearer "+login.Token, reqBody)
	if w.Code != http.StatusOK {
		t.Fatalf("req 3 status = %d", w.Code)
	}
	if len(tried) != 2 || tried[0] != "free" || tried[1] != "paid" {
		t.Fatalf("req 3 tried = %v, want [free paid]", tried)
	}
	freeRec, _ = app.FindRecordById(upstreamModelsCollection, free.Id)
	wantBackoffDeadline := tProbe.Add(backoffQuotaCooldown)
	if !freeRec.GetDateTime("unavailable_until").Time().Equal(wantBackoffDeadline) {
		t.Fatalf("backoff until = %v, want %v", freeRec.GetDateTime("unavailable_until").Time(), wantBackoffDeadline)
	}

	// 4. After backoff expires and free quota is replenished -> succeeds and clears availability state.
	tried = nil
	freeHealthy = true
	now = wantBackoffDeadline.Add(time.Minute)
	w = doJSON(t, h, http.MethodPost, "/v1/proxy/chat/completions", "Bearer "+login.Token, reqBody)
	if w.Code != http.StatusOK {
		t.Fatalf("req 4 status = %d", w.Code)
	}
	if len(tried) != 1 || tried[0] != "free" {
		t.Fatalf("req 4 tried = %v, want [free]", tried)
	}
	freeRec, _ = app.FindRecordById(upstreamModelsCollection, free.Id)
	if freeRec.GetString("unavailable_reason") != "" || !freeRec.GetDateTime("unavailable_until").IsZero() || freeRec.GetString("last_error") != "" {
		t.Fatalf("expected state cleared, got reason=%q until=%v err=%q",
			freeRec.GetString("unavailable_reason"),
			freeRec.GetDateTime("unavailable_until"),
			freeRec.GetString("last_error"),
		)
	}
}

func TestModelRateLimitRetryAfterAndAuthManualOrKeyRotationRecovery(t *testing.T) {
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	mode := "ratelimit"
	var tried []string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)
		tried = append(tried, body.Model)

		if body.Model == "primary" {
			switch mode {
			case "ratelimit":
				w.Header().Set("Retry-After", "45")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":{"message":"Rate limit reached"}}`))
				return
			case "auth":
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":{"message":"Invalid API key"}}`))
				return
			}
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer upstream.Close()

	app, srv := setupServerWithOptions(t, APIOptions{Now: func() time.Time { return now }})
	p1 := createProvider(t, app, "p1", upstream.URL, "sk-bad", true)
	p2 := createProvider(t, app, "p2", upstream.URL, "sk-good", true)
	primary := createModel(t, app, p1, "primary", 10, 0, 0, true)
	backup := createModel(t, app, p2, "backup", 20, 0, 0, true)
	user := createUser(t, app, "rluser", "rluserpw123")
	configureUser(t, app, user, nil, primary, backup)

	h := buildMux(t, app, srv)
	login := loginWithPassword(t, h, "rluser@example.com", "rluserpw123")
	reqBody := `{"model":"auto","messages":[{"role":"user","content":"hi"}]}`

	// 1. Rate-limit with Retry-After: 45s.
	w := doJSON(t, h, http.MethodPost, "/v1/proxy/chat/completions", "Bearer "+login.Token, reqBody)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	rec, _ := app.FindRecordById(upstreamModelsCollection, primary.Id)
	if rec.GetString("unavailable_reason") != modelUnavailableReasonRateLimit {
		t.Fatalf("reason = %q, want rate_limit", rec.GetString("unavailable_reason"))
	}
	if !rec.GetDateTime("unavailable_until").Time().Equal(now.Add(45 * time.Second)) {
		t.Fatalf("until = %v, want %v", rec.GetDateTime("unavailable_until").Time(), now.Add(45*time.Second))
	}

	// 20s later: still skipped.
	tried = nil
	now = now.Add(20 * time.Second)
	_ = doJSON(t, h, http.MethodPost, "/v1/proxy/chat/completions", "Bearer "+login.Token, reqBody)
	if len(tried) != 1 || tried[0] != "backup" {
		t.Fatalf("tried during rate_limit cooldown = %v, want [backup]", tried)
	}

	// 2. 46s later: cooldown expired, now returns 401 -> marked auth (does NOT auto-recover even 7 days later).
	tried = nil
	mode = "auth"
	now = now.Add(30 * time.Second)
	_ = doJSON(t, h, http.MethodPost, "/v1/proxy/chat/completions", "Bearer "+login.Token, reqBody)
	if len(tried) != 2 || tried[0] != "primary" || tried[1] != "backup" {
		t.Fatalf("tried = %v, want [primary backup]", tried)
	}
	rec, _ = app.FindRecordById(upstreamModelsCollection, primary.Id)
	if rec.GetString("unavailable_reason") != modelUnavailableReasonAuth || !rec.GetDateTime("unavailable_until").IsZero() {
		t.Fatalf("auth state: reason=%q until=%v", rec.GetString("unavailable_reason"), rec.GetDateTime("unavailable_until"))
	}

	tried = nil
	now = now.AddDate(0, 0, 7)
	_ = doJSON(t, h, http.MethodPost, "/v1/proxy/chat/completions", "Bearer "+login.Token, reqBody)
	if len(tried) != 1 || tried[0] != "backup" {
		t.Fatalf("auth failure must not auto-recover after 7 days: tried=%v", tried)
	}

	// 3. Admin manually clears unavailable_reason -> hook clears last_error as well.
	rec.Set("unavailable_reason", "")
	if err := app.Save(rec); err != nil {
		t.Fatalf("manual clear save: %v", err)
	}
	rec, _ = app.FindRecordById(upstreamModelsCollection, primary.Id)
	if rec.GetString("last_error") != "" {
		t.Fatalf("expected last_error cleared when admin clears reason, got %q", rec.GetString("last_error"))
	}

	// Trigger auth block once more, then rotate provider API key -> auto-clears model auth block.
	_ = doJSON(t, h, http.MethodPost, "/v1/proxy/chat/completions", "Bearer "+login.Token, reqBody)
	p1Rec, _ := app.FindRecordById(upstreamProvidersCollection, p1.Id)
	p1Rec.Set(providerAPIKeyField, "sk-fixed")
	if err := app.Save(p1Rec); err != nil {
		t.Fatalf("rotate provider key: %v", err)
	}
	rec, _ = app.FindRecordById(upstreamModelsCollection, primary.Id)
	if rec.GetString("unavailable_reason") != "" || rec.GetString("last_error") != "" {
		t.Fatalf("expected provider key rotation to clear auth block, got reason=%q err=%q",
			rec.GetString("unavailable_reason"), rec.GetString("last_error"))
	}

	tried = nil
	mode = "ok"
	w = doJSON(t, h, http.MethodPost, "/v1/proxy/chat/completions", "Bearer "+login.Token, reqBody)
	if w.Code != http.StatusOK || len(tried) != 1 || tried[0] != "primary" {
		t.Fatalf("after key rotation: status=%d tried=%v", w.Code, tried)
	}
}

func TestProxyReturns503WhenAllModelsUnavailable(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	var calls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
	}))
	defer upstream.Close()

	app, srv := setupServerWithOptions(t, APIOptions{Now: func() time.Time { return now }})
	provider := createProvider(t, app, "p", upstream.URL, "sk", true)
	model := createModel(t, app, provider, "m", 10, 0, 0, true)
	until, _ := types.ParseDateTime(now.Add(time.Hour))
	model.Set("unavailable_reason", modelUnavailableReasonQuota)
	model.Set("unavailable_until", until)
	if err := app.Save(model); err != nil {
		t.Fatalf("save model state: %v", err)
	}

	user := createUser(t, app, "u503", "u503pw12345")
	configureUser(t, app, user, nil, model)

	h := buildMux(t, app, srv)
	login := loginWithPassword(t, h, "u503@example.com", "u503pw12345")

	w := doJSON(t, h, http.MethodPost, "/v1/proxy/chat/completions", "Bearer "+login.Token,
		`{"model":"auto","messages":[{"role":"user","content":"hi"}]}`)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 body=%s", w.Code, w.Body.String())
	}
	if calls != 0 {
		t.Fatalf("upstream calls = %d, want 0", calls)
	}
}
