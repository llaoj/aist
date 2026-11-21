package interactive

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/llaoj/aist/internal/i18n"
	"github.com/llaoj/aist/internal/llm"
)

func TestRenderErrorMessageLocalized(t *testing.T) {
	retryAt := time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)
	localExpected := retryAt.Local().Format(localRetryTimeLayout)

	tests := []struct {
		name      string
		err       error
		wantZhSub string
		wantEnSub string
	}{
		{
			name: "quota_exceeded_with_retry_at",
			err: &llm.StatusError{
				StatusCode: 429,
				ErrorCode:  llm.ErrorCodeQuotaExceeded,
				RetryAt:    retryAt,
			},
			wantZhSub: localExpected,
			wantEnSub: localExpected,
		},
		{
			name: "no_available_model_with_retry_at",
			err: &llm.StatusError{
				StatusCode: 503,
				ErrorCode:  llm.ErrorCodeNoAvailableModel,
				RetryAt:    retryAt,
			},
			wantZhSub: localExpected,
			wantEnSub: localExpected,
		},
		{
			name: "no_available_model_without_retry_at",
			err: &llm.StatusError{
				StatusCode: 403,
				ErrorCode:  llm.ErrorCodeNoAvailableModel,
			},
			wantZhSub: "当前没有可用的模型",
			wantEnSub: "No model is currently available",
		},
		{
			name: "upstream_unavailable",
			err: &llm.StatusError{
				StatusCode: 502,
				ErrorCode:  llm.ErrorCodeUpstreamUnavailable,
			},
			wantZhSub: "上游模型服务暂时不可用",
			wantEnSub: "temporarily unavailable",
		},
		{
			name: "invalid_request_context_length",
			err: &llm.StatusError{
				StatusCode: 400,
				ErrorCode:  llm.ErrorCodeInvalidRequest,
				Message:    "context length exceeded",
			},
			wantZhSub: "上下文长度",
			wantEnSub: "context window",
		},
		{
			name:      "authentication_expired",
			err:       fmt.Errorf("%w: invalid access token", llm.ErrAuthentication),
			wantZhSub: "aist login",
			wantEnSub: "aist login",
		},
		{
			name:      "generic_error_passthrough",
			err:       errors.New("disk full"),
			wantZhSub: "disk full",
			wantEnSub: "disk full",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			zh := RenderErrorMessage(i18n.New("zh"), tt.err)
			if strings.Contains(zh, "%!") || !strings.Contains(zh, tt.wantZhSub) {
				t.Fatalf("zh = %q, want substring %q", zh, tt.wantZhSub)
			}
			en := RenderErrorMessage(i18n.New("en"), tt.err)
			if strings.Contains(en, "%!") || !strings.Contains(en, tt.wantEnSub) {
				t.Fatalf("en = %q, want substring %q", en, tt.wantEnSub)
			}
		})
	}
}
