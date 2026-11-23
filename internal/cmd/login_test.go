package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/llaoj/aist/internal/preferences"
	"github.com/llaoj/aist/internal/state"
	"github.com/llaoj/aist/internal/token"
)

func TestNormalizeServerURL(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty", raw: "", want: ""},
		{name: "https", raw: "https://api.example.com/", want: "https://api.example.com"},
		{name: "http", raw: "http://api.example.com/", want: "http://api.example.com"},
		{name: "bare host", raw: "api.example.com", want: "https://api.example.com"},
		{name: "bare host with port", raw: "api.example.com:443", want: "https://api.example.com:443"},
		{name: "bare host with path", raw: "api.example.com/aist/", want: "https://api.example.com/aist"},
		{name: "localhost", raw: "localhost:8090", want: "http://localhost:8090"},
		{name: "ipv4 loopback", raw: "127.0.0.1:8090", want: "http://127.0.0.1:8090"},
		{name: "ipv6 loopback", raw: "[::1]:8090", want: "http://[::1]:8090"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeServerURL(tc.raw); got != tc.want {
				t.Fatalf("normalizeServerURL(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestVersionSkipsStateInit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	rootCmd.SetArgs([]string{"version"})
	defer rootCmd.SetArgs(nil)
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute version: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, state.DefaultRootName)); !os.IsNotExist(err) {
		t.Fatalf("version created state directory: %v", err)
	}
}

func TestWhoamiAndLogoutShareInitializedStateRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv(token.EnvServer, "")

	stateRoot, err := state.EnsureRoot("")
	if err != nil {
		t.Fatalf("EnsureRoot: %v", err)
	}
	store := token.NewStore(token.StoreOptions{State: stateRoot})
	if err := store.Save(token.Credentials{
		ServerURL: "http://127.0.0.1:1",
		Token:     "test-jwt",
		Email:     "alice@example.com",
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	rootCmd.SetArgs([]string{"whoami"})
	defer rootCmd.SetArgs(nil)
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute whoami: %v", err)
	}

	rootCmd.SetArgs([]string{"logout"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute logout: %v", err)
	}
	if _, err := os.Stat(stateRoot.Paths.TokenFile); !os.IsNotExist(err) {
		t.Fatalf("token file still exists after logout: %v", err)
	}
	if _, err := os.Stat(stateRoot.Paths.PreferencesFile); err != nil {
		t.Fatalf("preferences file should survive logout: %v", err)
	}
}

func TestRunLoginPasswordMFAInvalidVerificationCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/collections/users/auth-methods":
			_, _ = w.Write([]byte(`{
				"password": {"enabled": true, "identityFields": ["email"]},
				"otp": {"enabled": true, "duration": 180},
				"mfa": {"enabled": true, "duration": 600}
			}`))
		case "/api/collections/users/auth-with-password":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"mfaId":"mfa-1"}`))
		case "/api/collections/users/request-otp":
			_, _ = w.Write([]byte(`{"otpId":"otp-1"}`))
		case "/api/collections/users/auth-with-otp":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"status":400,"message":"Failed to authenticate."}`))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	stateRoot := newTestStateRoot(t)
	ctx := withStateRoot(context.Background(), stateRoot)

	inputs := []string{"alice@example.com", "1171066"}
	err := runLoginWithPrompter(ctx, []string{srv.URL}, loginPrompter{
		interactive: true,
		promptInput: func(string) (string, error) {
			next := inputs[0]
			inputs = inputs[1:]
			return next, nil
		},
		promptSecret: func(string) (string, error) {
			return "test-password", nil
		},
	})
	if err == nil {
		t.Fatal("expected error for invalid verification code")
	}
	want := "login failed: invalid or expired verification code"
	if err.Error() != want {
		t.Fatalf("err = %q, want %q", err.Error(), want)
	}
	assertRememberedServerOnly(t, stateRoot, srv.URL)
}

// assertRememberedServerOnly checks that a failed login stored the attempted
// server but no token.
func assertRememberedServerOnly(t *testing.T, stateRoot *state.Root, wantServer string) {
	t.Helper()
	creds, err := token.NewStore(token.StoreOptions{
		State:     stateRoot,
		LookupEnv: func(string) (string, bool) { return "", false },
	}).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if creds.Token != "" {
		t.Fatalf("token stored after failed login: %q", creds.Token)
	}
	if creds.ServerURL != wantServer {
		t.Fatalf("remembered server = %q, want %q", creds.ServerURL, wantServer)
	}
}

func TestRunLoginRemembersServerAfterCancel(t *testing.T) {
	stateRoot := newTestStateRoot(t)
	ctx := withStateRoot(context.Background(), stateRoot)

	errCancelled := errors.New("cancelled")
	cancel := loginPrompter{
		interactive:  true,
		promptInput:  func(string) (string, error) { return "", errCancelled },
		promptSecret: func(string) (string, error) { return "", errCancelled },
	}

	if err := runLoginWithPrompter(ctx, []string{"aist.example.com"}, cancel); !errors.Is(err, errCancelled) {
		t.Fatalf("err = %v, want %v", err, errCancelled)
	}
	assertRememberedServerOnly(t, stateRoot, "https://aist.example.com")

	if err := runLoginWithPrompter(ctx, nil, cancel); !errors.Is(err, errCancelled) {
		t.Fatalf("second login err = %v, want %v", err, errCancelled)
	}
	assertRememberedServerOnly(t, stateRoot, "https://aist.example.com")

	if err := printWhoami(ctx, io.Discard, defaultWhoamiTimeout); err == nil {
		t.Fatal("whoami should report not logged in")
	}
}

func TestRunLoginKeepsExistingLoginOnCancel(t *testing.T) {
	stateRoot := newTestStateRoot(t)
	ctx := withStateRoot(context.Background(), stateRoot)

	store := token.NewStore(token.StoreOptions{State: stateRoot})
	existing := token.Credentials{ServerURL: "https://a.example.com", Token: "token-a", Email: "alice@example.com"}
	if err := store.Save(existing); err != nil {
		t.Fatalf("Save: %v", err)
	}

	errCancelled := errors.New("cancelled")
	err := runLoginWithPrompter(ctx, []string{"https://b.example.com"}, loginPrompter{
		interactive: true,
		promptInput: func(string) (string, error) { return "", errCancelled },
	})
	if !errors.Is(err, errCancelled) {
		t.Fatalf("err = %v, want %v", err, errCancelled)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != existing {
		t.Fatalf("credentials = %+v, want %+v", got, existing)
	}
}

func TestRunLoginPasswordMFASuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/collections/users/auth-methods":
			_, _ = w.Write([]byte(`{
				"password": {"enabled": true, "identityFields": ["email"]},
				"otp": {"enabled": true, "duration": 180},
				"mfa": {"enabled": true, "duration": 600}
			}`))
		case "/api/collections/users/auth-with-password":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"mfaId":"mfa-1"}`))
		case "/api/collections/users/request-otp":
			_, _ = w.Write([]byte(`{"otpId":"otp-1"}`))
		case "/api/collections/users/auth-with-otp":
			if got := r.URL.Query().Get("mfaId"); got != "mfa-1" {
				t.Errorf("mfaId = %q, want mfa-1", got)
			}
			_, _ = fmt.Fprintf(w, `{"token":"test-jwt","record":{"id":"u1","email":"alice@example.com"},"meta":{"config":{"schema_version":1}}}`)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "zh_CN.UTF-8")

	stateRoot := newTestStateRoot(t)
	ctx := withStateRoot(context.Background(), stateRoot)

	origStdout := os.Stdout
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	os.Stdout = pw
	t.Cleanup(func() {
		os.Stdout = origStdout
		_ = pw.Close()
		_ = pr.Close()
	})

	inputs := []string{"alice@example.com", "12345678"}
	err = runLoginWithPrompter(ctx, []string{srv.URL}, loginPrompter{
		interactive: true,
		promptInput: func(string) (string, error) {
			next := inputs[0]
			inputs = inputs[1:]
			return next, nil
		},
		promptSecret: func(string) (string, error) {
			return "test-password", nil
		},
	})
	_ = pw.Close()
	printed, _ := io.ReadAll(pr)
	_ = pr.Close()
	if err != nil {
		t.Fatalf("runLoginWithPrompter: %v", err)
	}
	if !strings.Contains(string(printed), "aist lang en") {
		t.Fatalf("expected quick start language hint in login output:\n%s", string(printed))
	}

	store := token.NewStore(token.StoreOptions{State: stateRoot})
	creds, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if creds.Token != "test-jwt" || creds.Email != "alice@example.com" {
		t.Fatalf("creds = %+v", creds)
	}
}

func TestRunLoginPasswordServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/collections/users/auth-methods":
			_, _ = w.Write([]byte(`{"password": {"enabled": true, "identityFields": ["email"]}}`))
		case "/api/collections/users/auth-with-password":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"internal error"}`))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	stateRoot := newTestStateRoot(t)
	ctx := withStateRoot(context.Background(), stateRoot)

	err := runLoginWithPrompter(ctx, []string{srv.URL}, loginPrompter{
		interactive: true,
		promptInput: func(string) (string, error) {
			return "alice@example.com", nil
		},
		promptSecret: func(string) (string, error) {
			return "test-password", nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "login failed: server returned status 500") {
		t.Fatalf("err = %v, want 500 login failure", err)
	}
}

func TestRunLoginPasswordInvalidIncludesResetURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/collections/users/auth-methods":
			_, _ = w.Write([]byte(`{"password": {"enabled": true, "identityFields": ["email"]}}`))
		case "/api/collections/users/auth-with-password":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"status":400,"message":"Failed to authenticate."}`))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	stateRoot := newTestStateRoot(t)
	ctx := withStateRoot(context.Background(), stateRoot)

	err := runLoginWithPrompter(ctx, []string{srv.URL}, loginPrompter{
		interactive: true,
		promptInput: func(string) (string, error) {
			return "alice@example.com", nil
		},
		promptSecret: func(string) (string, error) {
			return "wrong-password", nil
		},
	})
	wantSubstr := srv.URL + "/register#reset"
	if err == nil || !strings.Contains(err.Error(), wantSubstr) {
		t.Fatalf("err = %v, want reset URL %q", err, wantSubstr)
	}
}

func newTestStateRoot(t *testing.T) *state.Root {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv(token.EnvServer, "")

	prevEmail := flagLoginEmail
	flagLoginEmail = ""
	t.Cleanup(func() {
		flagLoginEmail = prevEmail
	})

	stateRoot, err := state.EnsureRoot("")
	if err != nil {
		t.Fatalf("EnsureRoot: %v", err)
	}
	return stateRoot
}

func TestPrintWhoamiQuotaUnlimitedRefreshAndFallbacks(t *testing.T) {
	resetAt := time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)

	t.Run("quota_chinese_and_refresh_on_401", func(t *testing.T) {
		usageCalls := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/v1/account/usage":
				usageCalls++
				if r.Header.Get("Authorization") == "Bearer stale-token" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				if r.Header.Get("Authorization") != "Bearer fresh-token" {
					t.Errorf("unexpected Authorization: %s", r.Header.Get("Authorization"))
				}
				_, _ = fmt.Fprintf(w, `{
					"week_start": "2026-11-16",
					"reset_at": %q,
					"requests": 18,
					"used_tokens": 42180,
					"weekly_token_quota": 500000,
					"remaining_tokens": 457820
				}`, resetAt.Format(time.RFC3339))
			case "/api/collections/users/auth-refresh":
				_, _ = w.Write([]byte(`{"token":"fresh-token","record":{"id":"u1","email":"alice@example.com"},"meta":{"config":{"schema_version":1}}}`))
			default:
				t.Errorf("unexpected path: %s", r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		stateRoot := newTestStateRoot(t)
		ctx := withStateRoot(context.Background(), stateRoot)
		if err := preferences.NewStore(preferences.StoreOptions{State: stateRoot}).Save(preferences.Data{Language: "zh"}); err != nil {
			t.Fatalf("Save preferences: %v", err)
		}
		store := token.NewStore(token.StoreOptions{State: stateRoot})
		if err := store.Save(token.Credentials{ServerURL: srv.URL, Token: "stale-token"}); err != nil {
			t.Fatalf("Save token: %v", err)
		}

		var buf bytes.Buffer
		if err := printWhoami(ctx, &buf, time.Second); err != nil {
			t.Fatalf("printWhoami: %v", err)
		}
		out := buf.String()
		if usageCalls != 2 {
			t.Fatalf("usageCalls = %d, want 2", usageCalls)
		}
		for _, want := range []string{
			"邮箱:     alice@example.com",
			"状态:     已登录",
			"本周用量: 42,180 / 500,000 tokens（剩余 457,820，共 18 次调用）",
			"重置时间: " + resetAt.Local().Format("2006-01-02 15:04"),
		} {
			if !strings.Contains(out, want) {
				t.Fatalf("output missing %q:\n%s", want, out)
			}
		}
	})

	t.Run("unlimited_english", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{
				"week_start": "2026-11-16",
				"reset_at": "2026-11-23T00:00:00Z",
				"requests": 5,
				"used_tokens": 1200,
				"weekly_token_quota": 0
			}`))
		}))
		defer srv.Close()

		stateRoot := newTestStateRoot(t)
		ctx := withStateRoot(context.Background(), stateRoot)
		if err := preferences.NewStore(preferences.StoreOptions{State: stateRoot}).Save(preferences.Data{Language: "en"}); err != nil {
			t.Fatalf("Save preferences: %v", err)
		}
		store := token.NewStore(token.StoreOptions{State: stateRoot})
		if err := store.Save(token.Credentials{ServerURL: srv.URL, Token: "valid-token", Email: "bob@example.com"}); err != nil {
			t.Fatalf("Save token: %v", err)
		}

		var buf bytes.Buffer
		if err := printWhoami(ctx, &buf, time.Second); err != nil {
			t.Fatalf("printWhoami: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "Weekly usage: 1,200 tokens (unlimited, 5 requests)") {
			t.Fatalf("unexpected output:\n%s", out)
		}
		if strings.Contains(out, "Resets at:") {
			t.Fatalf("unlimited output should not show reset time:\n%s", out)
		}
	})

	t.Run("unsupported_older_server_404", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		stateRoot := newTestStateRoot(t)
		ctx := withStateRoot(context.Background(), stateRoot)
		if err := preferences.NewStore(preferences.StoreOptions{State: stateRoot}).Save(preferences.Data{Language: "en"}); err != nil {
			t.Fatalf("Save preferences: %v", err)
		}
		store := token.NewStore(token.StoreOptions{State: stateRoot})
		if err := store.Save(token.Credentials{ServerURL: srv.URL, Token: "valid-token", Email: "bob@example.com"}); err != nil {
			t.Fatalf("Save token: %v", err)
		}

		var buf bytes.Buffer
		if err := printWhoami(ctx, &buf, time.Second); err != nil {
			t.Fatalf("printWhoami: %v", err)
		}
		if !strings.Contains(buf.String(), "Weekly usage: unsupported by this server version") {
			t.Fatalf("unexpected output:\n%s", buf.String())
		}
	})

	t.Run("revoked_token_returns_login_expired", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer srv.Close()

		stateRoot := newTestStateRoot(t)
		ctx := withStateRoot(context.Background(), stateRoot)
		if err := preferences.NewStore(preferences.StoreOptions{State: stateRoot}).Save(preferences.Data{Language: "en"}); err != nil {
			t.Fatalf("Save preferences: %v", err)
		}
		store := token.NewStore(token.StoreOptions{State: stateRoot})
		if err := store.Save(token.Credentials{ServerURL: srv.URL, Token: "revoked-token", Email: "bob@example.com"}); err != nil {
			t.Fatalf("Save token: %v", err)
		}

		var buf bytes.Buffer
		err := printWhoami(ctx, &buf, time.Second)
		if err == nil || err.Error() != "login expired; run `aist login`" {
			t.Fatalf("err = %v, want login expired", err)
		}
		if !strings.Contains(buf.String(), "login expired") {
			t.Fatalf("unexpected output:\n%s", buf.String())
		}
	})
}
