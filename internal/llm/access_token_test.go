package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeAccessTokenSource struct {
	token         string
	refreshToken  string
	refreshCalled int
	refreshErr    error
}

func (f *fakeAccessTokenSource) AccessToken(context.Context) (string, error) {
	return f.token, nil
}

func (f *fakeAccessTokenSource) RefreshAccessToken(context.Context) (string, error) {
	f.refreshCalled++
	if f.refreshErr != nil {
		return "", f.refreshErr
	}
	f.token = f.refreshToken
	return f.token, nil
}

func TestModelRefreshesOnUnauthorized(t *testing.T) {
	source := &fakeAccessTokenSource{token: "old", refreshToken: "new"}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			if got := r.Header.Get("Authorization"); got != "Bearer old" {
				t.Errorf("first Authorization = %q", got)
			}
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer new" {
			t.Errorf("second Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer srv.Close()

	model, err := NewOpenAICompatibleModel(ProviderSpec{
		ID:                "p/m",
		BaseURL:           srv.URL,
		AccessTokenSource: source,
		ModelName:         "m",
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleModel: %v", err)
	}
	resp, err := model.Call(context.Background(), CompletionRequest{})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Content != "ok" {
		t.Fatalf("content = %q", resp.Content)
	}
	if source.refreshCalled != 1 {
		t.Fatalf("refresh calls = %d", source.refreshCalled)
	}
}

func TestModelDoesNotRefreshOnForbidden(t *testing.T) {
	source := &fakeAccessTokenSource{token: "token", refreshToken: "new"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	model, err := NewOpenAICompatibleModel(ProviderSpec{
		ID:                "p/m",
		BaseURL:           srv.URL,
		AccessTokenSource: source,
		ModelName:         "m",
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleModel: %v", err)
	}
	if _, err := model.Call(context.Background(), CompletionRequest{}); err == nil {
		t.Fatal("expected forbidden error")
	}
	if source.refreshCalled != 0 {
		t.Fatalf("refresh calls = %d, want 0", source.refreshCalled)
	}
}
