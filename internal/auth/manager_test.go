package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/llaoj/aist/internal/remote"
	"github.com/llaoj/aist/internal/state"
	"github.com/llaoj/aist/internal/token"
)

func testStore(t *testing.T) *token.Store {
	t.Helper()
	stateRoot, err := state.EnsureRoot(t.TempDir())
	if err != nil {
		t.Fatalf("EnsureRoot: %v", err)
	}
	return token.NewStore(token.StoreOptions{
		State:     stateRoot,
		LookupEnv: func(string) (string, bool) { return "", false },
	})
}

func testManager(t *testing.T, serverURL string, skew time.Duration) (*Manager, *token.Store) {
	t.Helper()
	store := testStore(t)
	manager := NewManager(ManagerOptions{
		Store:       store,
		Client:      remote.NewClient(remote.ClientOptions{ServerURL: serverURL}),
		RefreshSkew: skew,
	})
	return manager, store
}

func TestStartupRequiresLogin(t *testing.T) {
	manager, _ := testManager(t, "https://example.com", 0)
	if _, err := manager.Startup(context.Background()); !errors.Is(err, ErrNotLoggedIn) {
		t.Fatalf("Startup err = %v, want ErrNotLoggedIn", err)
	}
}

func TestStartupRefreshesAndSaves(t *testing.T) {
	newToken := testToken(time.Now().Add(time.Hour).Unix())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `{
			"token": %q,
			"record": {"id": "user1", "email": "alice@example.com"},
			"meta": {"config": {"schema_version": 1}}
		}`, newToken)
	}))
	defer srv.Close()

	manager, store := testManager(t, srv.URL, 0)
	if err := store.Save(token.Credentials{
		ServerURL: srv.URL,
		Token:     "old-token",
		Email:     "alice@example.com",
		ExpiresAt: time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	resp, err := manager.Startup(context.Background())
	if err != nil {
		t.Fatalf("Startup: %v", err)
	}
	if resp.Token != newToken {
		t.Fatalf("token = %q", resp.Token)
	}
	saved, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if saved.Token != newToken || saved.Email != "alice@example.com" {
		t.Fatalf("saved = %+v", saved)
	}
	if saved.ExpiresAt.IsZero() {
		t.Fatal("saved expiry is zero")
	}
}

func TestAccessTokenUsesValidToken(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	manager, store := testManager(t, srv.URL, time.Minute)
	if err := store.Save(token.Credentials{
		ServerURL: srv.URL,
		Token:     "valid-token",
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	value, err := manager.AccessToken(context.Background())
	if err != nil {
		t.Fatalf("AccessToken: %v", err)
	}
	if value != "valid-token" {
		t.Fatalf("token = %q", value)
	}
	if calls != 0 {
		t.Fatalf("refresh calls = %d, want 0", calls)
	}
}

func TestAccessTokenRefreshesExpiredToken(t *testing.T) {
	refreshed := testToken(time.Now().Add(time.Hour).Unix())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `{
			"token": %q,
			"record": {"id": "user1", "email": "alice@example.com"},
			"meta": {"config": {"schema_version": 1}}
		}`, refreshed)
	}))
	defer srv.Close()

	manager, store := testManager(t, srv.URL, time.Minute)
	if err := store.Save(token.Credentials{
		ServerURL: srv.URL,
		Token:     "expired-token",
		ExpiresAt: time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	value, err := manager.AccessToken(context.Background())
	if err != nil {
		t.Fatalf("AccessToken: %v", err)
	}
	if value != refreshed {
		t.Fatalf("token = %q", value)
	}
}

func TestRefreshAccessTokenUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	manager, store := testManager(t, srv.URL, 0)
	if err := store.Save(token.Credentials{
		ServerURL: srv.URL,
		Token:     "expired-token",
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := manager.RefreshAccessToken(context.Background()); !errors.Is(err, ErrLoginRequired) {
		t.Fatalf("RefreshAccessToken err = %v, want ErrLoginRequired", err)
	}
}

func testToken(exp int64) string {
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d}`, exp)))
	return "header." + payload + ".signature"
}
