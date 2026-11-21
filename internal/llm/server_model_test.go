package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestServerModelRetriesTransientFailure(t *testing.T) {
	calls := 0
	var gotModel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body struct {
			Model string `json:"model"`
		}
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)
		gotModel = body.Model
		if calls < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer srv.Close()

	model, err := NewServerModel(ServerModelOptions{
		BaseURL:      srv.URL,
		MaxAttempts:  3,
		RetryBackoff: time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewServerModel: %v", err)
	}
	resp, err := model.Call(context.Background(), CompletionRequest{})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Content != "ok" {
		t.Fatalf("resp = %+v", resp)
	}
	if calls != 3 {
		t.Fatalf("calls = %d", calls)
	}
	if gotModel != "auto" {
		t.Fatalf("model = %q, want auto", gotModel)
	}
}

func TestServerModelDoesNotRetryBadRequest(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	model, err := NewServerModel(ServerModelOptions{
		BaseURL:      srv.URL,
		MaxAttempts:  3,
		RetryBackoff: time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewServerModel: %v", err)
	}
	if _, err := model.Call(context.Background(), CompletionRequest{}); err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestServerModelDoesNotRetryAuthenticationFailure(t *testing.T) {
	source := &fakeAccessTokenSource{token: "expired", refreshErr: errors.New("refresh failed")}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	model, err := NewServerModel(ServerModelOptions{
		BaseURL:           srv.URL,
		AccessTokenSource: source,
		MaxAttempts:       3,
		RetryBackoff:      time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewServerModel: %v", err)
	}
	_, err = model.Call(context.Background(), CompletionRequest{})
	if err == nil || !errors.Is(err, ErrAuthentication) {
		t.Fatalf("err = %v", err)
	}
	if calls != 1 || source.refreshCalled != 1 {
		t.Fatalf("calls = %d refresh = %d", calls, source.refreshCalled)
	}
}

func TestServerModelDoesNotRetryQuotaOrNoAvailableModel(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		code   string
	}{
		{name: "quota_exceeded_429", status: http.StatusTooManyRequests, code: ErrorCodeQuotaExceeded},
		{name: "no_available_model_503", status: http.StatusServiceUnavailable, code: ErrorCodeNoAvailableModel},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`{"error":{"code":"` + tc.code + `","message":"blocked","retry_at":"2026-11-23T00:00:00Z"}}`))
			}))
			defer srv.Close()

			model, err := NewServerModel(ServerModelOptions{
				BaseURL:      srv.URL,
				MaxAttempts:  3,
				RetryBackoff: time.Millisecond,
			})
			if err != nil {
				t.Fatalf("NewServerModel: %v", err)
			}
			_, err = model.Call(context.Background(), CompletionRequest{})
			var statusErr *StatusError
			if !errors.As(err, &statusErr) {
				t.Fatalf("err = %v, want *StatusError", err)
			}
			if statusErr.ErrorCode != tc.code || statusErr.RetryAt.IsZero() {
				t.Fatalf("statusErr = %+v", statusErr)
			}
			if calls != 1 {
				t.Fatalf("calls = %d, want 1", calls)
			}
		})
	}
}
