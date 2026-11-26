package server

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestClassifyUpstreamErrorProviderPayloads(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		status     int
		retryAfter string
		body       string
		cause      error
		wantKind   upstreamErrorKind
		wantRetry  time.Duration
		wantSubstr string
	}{
		{
			name:       "deepseek_402_insufficient_balance",
			status:     http.StatusPaymentRequired,
			body:       `{"error":{"message":"Insufficient Balance","type":"unknown_error","param":null,"code":"invalid_request_error"}}`,
			wantKind:   upstreamErrorKindQuota,
			wantSubstr: "Insufficient Balance",
		},
		{
			name:       "deepseek_429_rate_limit",
			status:     http.StatusTooManyRequests,
			retryAfter: "15",
			body:       `{"error":{"message":"Rate limit reached for requests","type":"requests","param":null,"code":"rate_limit_exceeded"}}`,
			wantKind:   upstreamErrorKindRateLimit,
			wantRetry:  15 * time.Second,
			wantSubstr: "Rate limit reached",
		},
		{
			name:       "siliconflow_402_top_level_code",
			status:     http.StatusPaymentRequired,
			body:       `{"code":402,"message":"Your account balance is insufficient, please top up first.","data":null}`,
			wantKind:   upstreamErrorKindQuota,
			wantSubstr: "balance is insufficient",
		},
		{
			name:       "siliconflow_403_auth",
			status:     http.StatusForbidden,
			body:       `{"code":30001,"message":"Account balance insufficient"}`,
			wantKind:   upstreamErrorKindAuth,
			wantSubstr: "Account balance insufficient",
		},
		{
			name:       "siliconflow_429_tpm_limit",
			status:     http.StatusTooManyRequests,
			body:       `{"code":429,"message":"Request was rejected due to rate limiting. Details: TPM limit reached.","data":null}`,
			wantKind:   upstreamErrorKindRateLimit,
			wantSubstr: "TPM limit reached",
		},
		{
			name:       "zhipu_429_rate_limit",
			status:     http.StatusTooManyRequests,
			body:       `{"error":{"code":"1302","message":"您的账户已达到速率限制，请您控制请求频率。"}}`,
			wantKind:   upstreamErrorKindRateLimit,
			wantSubstr: "速率限制",
		},
		{
			name:       "modelscope_429_request_limit",
			status:     http.StatusTooManyRequests,
			body:       `{"errors":{"message":"Request limit exceeded.","request_id":"649b2bbc-c541"}}`,
			wantKind:   upstreamErrorKindRateLimit,
			wantSubstr: "Request limit exceeded",
		},
		{
			name:       "retry_after_http_date",
			status:     http.StatusTooManyRequests,
			retryAfter: now.Add(45 * time.Second).UTC().Format(http.TimeFormat),
			body:       `{"error":{"message":"Too many requests"}}`,
			wantKind:   upstreamErrorKindRateLimit,
			wantRetry:  45 * time.Second,
			wantSubstr: "Too many requests",
		},
		{
			name:       "auth_401",
			status:     http.StatusUnauthorized,
			body:       `{"error":{"message":"Authentication Fails, Your api key is invalid","type":"authentication_error"}}`,
			wantKind:   upstreamErrorKindAuth,
			wantSubstr: "Authentication Fails",
		},
		{
			name:       "auth_403_permission",
			status:     http.StatusForbidden,
			body:       `{"error":{"message":"Model access denied","code":"permission_denied"}}`,
			wantKind:   upstreamErrorKindAuth,
			wantSubstr: "Model access denied",
		},
		{
			name:       "bad_request_400_context_length",
			status:     http.StatusBadRequest,
			body:       `{"error":{"message":"This model's maximum context length is 65536 tokens.","type":"invalid_request_error","code":"context_length_exceeded"}}`,
			wantKind:   upstreamErrorKindBadRequest,
			wantSubstr: "maximum context length",
		},
		{
			name:       "bad_request_400_errors_array",
			status:     http.StatusBadRequest,
			body:       `{"errors":[{"message":"invalid tool_call_id","code":"invalid_param"}]}`,
			wantKind:   upstreamErrorKindBadRequest,
			wantSubstr: "invalid tool_call_id",
		},
		{
			name:       "bad_request_400_root_array_gemini_style",
			status:     http.StatusBadRequest,
			body:       `[{"error":{"code":400,"message":"API key not valid.","status":"INVALID_ARGUMENT"}}]`,
			wantKind:   upstreamErrorKindBadRequest,
			wantSubstr: "API key not valid.",
		},
		{
			name:       "bad_request_400_detail_and_msg_variants",
			status:     http.StatusUnprocessableEntity,
			body:       `{"detail":[{"msg":"field required"}]}`,
			wantKind:   upstreamErrorKindBadRequest,
			wantSubstr: "field required",
		},
		{
			name:       "transient_502_non_json",
			status:     http.StatusBadGateway,
			body:       `<html>502 Bad Gateway</html>`,
			wantKind:   upstreamErrorKindTransient,
			wantSubstr: "HTTP 502",
		},
		{
			name:       "transport_error_status_0",
			status:     0,
			wantKind:   upstreamErrorKindTransient,
			wantSubstr: "transport error",
		},
		{
			name:       "timeout_error_status_0",
			status:     0,
			cause:      context.DeadlineExceeded,
			wantKind:   upstreamErrorKindTransient,
			wantSubstr: "timed out",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var header http.Header
			if tt.retryAfter != "" {
				header = http.Header{"Retry-After": []string{tt.retryAfter}}
			}
			got := classifyUpstreamError(tt.status, header, []byte(tt.body), tt.cause, now)
			if got.Kind != tt.wantKind {
				t.Fatalf("Kind = %v, want %v (summary=%q)", got.Kind, tt.wantKind, got.Summary)
			}
			if got.RetryAfter != tt.wantRetry {
				t.Fatalf("RetryAfter = %v, want %v", got.RetryAfter, tt.wantRetry)
			}
			if !strings.Contains(got.Summary, tt.wantSubstr) {
				t.Fatalf("Summary = %q, want substring %q", got.Summary, tt.wantSubstr)
			}
		})
	}
}

func TestTruncateBytesPreservesUTF8(t *testing.T) {
	long := strings.Repeat("中", 300)
	got := truncateBytes(long, maxUpstreamErrorSummaryBytes)
	if len(got) > maxUpstreamErrorSummaryBytes {
		t.Fatalf("len = %d, want <= %d", len(got), maxUpstreamErrorSummaryBytes)
	}
	if len(got)%3 != 0 {
		t.Fatalf("cut mid-UTF8 rune: len=%d", len(got))
	}
}
