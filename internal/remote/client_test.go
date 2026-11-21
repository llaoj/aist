package remote

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClientTrimsServer(t *testing.T) {
	client := NewClient(ClientOptions{ServerURL: "https://example.com/"})
	if client.serverURL != "https://example.com" {
		t.Fatalf("serverURL = %q", client.serverURL)
	}
}

func TestNewClientDefaultServer(t *testing.T) {
	client := NewClient(ClientOptions{})
	if client.serverURL != DefaultServerURL {
		t.Fatalf("serverURL = %q, want %q", client.serverURL, DefaultServerURL)
	}
}

func TestAuthMethods(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != authMethodsPath {
			t.Errorf("path = %s, want %s", r.URL.Path, authMethodsPath)
		}
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		_, _ = w.Write([]byte(`{
			"password": {"enabled": true, "identityFields": ["email"]},
			"otp": {"enabled": true, "duration": 180},
			"mfa": {"enabled": false, "duration": 600}
		}`))
	}))
	defer srv.Close()

	client := NewClient(ClientOptions{ServerURL: srv.URL, HTTPClient: srv.Client()})
	methods, err := client.AuthMethods(context.Background())
	if err != nil {
		t.Fatalf("AuthMethods: %v", err)
	}
	if !methods.Password.Enabled || len(methods.Password.IdentityFields) != 1 || methods.Password.IdentityFields[0] != "email" {
		t.Fatalf("password methods = %+v", methods.Password)
	}
	if !methods.OTP.Enabled || methods.MFA.Enabled {
		t.Fatalf("methods = %+v", methods)
	}
}

func TestAuthWithPasswordOK(t *testing.T) {
	var gotIdentity, gotPassword string
	exp := time.Now().Add(time.Hour).Unix()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != authWithPasswordPath {
			t.Errorf("path = %s, want %s", r.URL.Path, authWithPasswordPath)
		}
		var body struct {
			Identity string `json:"identity"`
			Password string `json:"password"`
		}
		if err := decodeJSON(r, &body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		gotIdentity, gotPassword = body.Identity, body.Password
		_, _ = fmt.Fprintf(w, `{
			"token": %q,
			"record": {"id": "user1", "email": "alice@example.com"},
			"meta": {"config": {"schema_version": 1, "limits": {"context_window": 64000, "max_output_tokens": 4096}}}
		}`, testJWT(exp))
	}))
	defer srv.Close()

	client := NewClient(ClientOptions{ServerURL: srv.URL, HTTPClient: srv.Client()})
	resp, err := client.AuthWithPassword(context.Background(), "alice@example.com", "s3cret")
	if err != nil {
		t.Fatalf("AuthWithPassword: %v", err)
	}
	if gotIdentity != "alice@example.com" || gotPassword != "s3cret" {
		t.Fatalf("credentials sent = %q/%q", gotIdentity, gotPassword)
	}
	if resp.Token == "" || resp.Record.Email != "alice@example.com" {
		t.Fatalf("resp = %+v", resp)
	}
	if resp.Config.Limits.ContextWindow != 64000 || resp.Config.Limits.MaxOutputTokens != 4096 {
		t.Fatalf("config = %+v", resp.Config)
	}
	if want := srv.URL + proxyPath; client.ProxyBaseURL() != want {
		t.Fatalf("ProxyBaseURL = %q, want %q", client.ProxyBaseURL(), want)
	}
}

func TestAuthWithPasswordInvalidCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"status":400,"message":"Failed to authenticate."}`))
	}))
	defer srv.Close()

	client := NewClient(ClientOptions{ServerURL: srv.URL, HTTPClient: srv.Client()})
	_, err := client.AuthWithPassword(context.Background(), "alice@example.com", "bad")
	if err == nil {
		t.Fatal("expected error for 400")
	}
	if !IsBadRequest(err) {
		t.Fatalf("expected bad request APIError, got %v", err)
	}
}

func TestAuthWithPasswordMFARequired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"mfaId":"mfa-1"}`))
	}))
	defer srv.Close()

	client := NewClient(ClientOptions{ServerURL: srv.URL, HTTPClient: srv.Client()})
	_, err := client.AuthWithPassword(context.Background(), "alice@example.com", "pw")
	mfaID, ok := MFARequired(err)
	if !ok || mfaID != "mfa-1" {
		t.Fatalf("MFARequired(%v) = %q/%v", err, mfaID, ok)
	}
}

func TestRequestOTP(t *testing.T) {
	var gotEmail string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != requestOTPPath {
			t.Errorf("path = %s, want %s", r.URL.Path, requestOTPPath)
		}
		var body struct {
			Email string `json:"email"`
		}
		if err := decodeJSON(r, &body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		gotEmail = body.Email
		_, _ = w.Write([]byte(`{"otpId":"otp-1"}`))
	}))
	defer srv.Close()

	client := NewClient(ClientOptions{ServerURL: srv.URL, HTTPClient: srv.Client()})
	otpID, err := client.RequestOTP(context.Background(), "alice@example.com")
	if err != nil {
		t.Fatalf("RequestOTP: %v", err)
	}
	if gotEmail != "alice@example.com" || otpID != "otp-1" {
		t.Fatalf("email/otp = %q/%q", gotEmail, otpID)
	}
}

func TestAuthWithOTP(t *testing.T) {
	var gotMFAID, gotOTPID, gotCode string
	exp := time.Now().Add(time.Hour).Unix()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != authWithOTPPath {
			t.Errorf("path = %s, want %s", r.URL.Path, authWithOTPPath)
		}
		gotMFAID = r.URL.Query().Get("mfaId")
		var body struct {
			OTPID    string `json:"otpId"`
			Password string `json:"password"`
		}
		if err := decodeJSON(r, &body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		gotOTPID, gotCode = body.OTPID, body.Password
		_, _ = fmt.Fprintf(w, `{"token": %q, "record": {"id": "user1", "email": "alice@example.com"}, "meta": {"config": {"schema_version": 1}}}`, testJWT(exp))
	}))
	defer srv.Close()

	client := NewClient(ClientOptions{ServerURL: srv.URL, HTTPClient: srv.Client()})
	resp, err := client.AuthWithOTP(context.Background(), "otp-1", "12345678", "mfa-1")
	if err != nil {
		t.Fatalf("AuthWithOTP: %v", err)
	}
	if gotMFAID != "mfa-1" || gotOTPID != "otp-1" || gotCode != "12345678" {
		t.Fatalf("mfa/otp/code = %q/%q/%q", gotMFAID, gotOTPID, gotCode)
	}
	if resp.Token == "" {
		t.Fatal("empty token")
	}
}

func TestRefreshOK(t *testing.T) {
	var gotAuth string
	exp := time.Now().Add(time.Hour).Unix()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != authRefreshPath {
			t.Errorf("path = %s, want %s", r.URL.Path, authRefreshPath)
		}
		gotAuth = r.Header.Get("Authorization")
		_, _ = fmt.Fprintf(w, `{"token": %q, "record": {"id": "user1", "email": "alice@example.com"}, "meta": {"config": {"schema_version": 1}}}`, testJWT(exp))
	}))
	defer srv.Close()

	client := NewClient(ClientOptions{ServerURL: srv.URL, HTTPClient: srv.Client()})
	resp, err := client.Refresh(context.Background(), "token-1")
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if gotAuth != "Bearer token-1" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if resp.Token == "" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestRefreshUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := NewClient(ClientOptions{ServerURL: srv.URL, HTTPClient: srv.Client()})
	_, err := client.Refresh(context.Background(), "expired")
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !IsUnauthorized(err) {
		t.Fatalf("expected unauthorized APIError, got %v", err)
	}
}

func TestTokenExpiry(t *testing.T) {
	want := time.Unix(2000000000, 0)
	if got := TokenExpiry(testJWT(2000000000)); !got.Equal(want) {
		t.Fatalf("TokenExpiry = %v, want %v", got, want)
	}
	if got := TokenExpiry("invalid"); !got.IsZero() {
		t.Fatalf("TokenExpiry(invalid) = %v, want zero", got)
	}
}

func testJWT(exp int64) string {
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d}`, exp)))
	return "header." + payload + ".signature"
}

func decodeJSON(r *http.Request, out any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(out)
}
