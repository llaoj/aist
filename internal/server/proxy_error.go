package server

import (
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

const (
	proxyErrorCodeQuotaExceeded       = "quota_exceeded"
	proxyErrorCodeNoAvailableModel    = "no_available_model"
	proxyErrorCodeUpstreamUnavailable = "upstream_unavailable"
	proxyErrorCodeInvalidRequest      = "invalid_request"
)

type proxyErrorResponse struct {
	Error proxyErrorDetail `json:"error"`
}

type proxyErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	RetryAt string `json:"retry_at,omitempty"`
}

func respondProxyError(e *core.RequestEvent, status int, code, message string, retryAt time.Time) error {
	detail := proxyErrorDetail{
		Code:    code,
		Message: message,
	}
	if !retryAt.IsZero() {
		detail.RetryAt = retryAt.UTC().Format(time.RFC3339)
	}
	return e.JSON(status, proxyErrorResponse{Error: detail})
}

func earliestModelRetryAt(candidates []modelCandidate, now time.Time) time.Time {
	var earliest time.Time
	for _, c := range candidates {
		if c.UnavailableReason == modelUnavailableReasonAuth {
			continue
		}
		if c.UnavailableUntil.After(now) && (earliest.IsZero() || c.UnavailableUntil.Before(earliest)) {
			earliest = c.UnavailableUntil
		}
	}
	return earliest
}

func sanitizeBadRequestMessage(body []byte) string {
	code, errType, message := parseUpstreamErrorBody(body)
	combined := strings.ToLower(code + " " + errType + " " + message)

	for _, marker := range []string{
		"context_length_exceeded",
		"maximum context length",
		"context window",
		"prompt is too long",
		"too many tokens",
		"max_tokens",
		"上下文长度",
		"最大长度",
	} {
		if strings.Contains(combined, marker) {
			return "context length exceeded"
		}
	}

	for _, marker := range []string{
		"content_filter",
		"data_inspection_failed",
		"content_policy",
		"safety",
		"敏感",
		"安全审核",
	} {
		if strings.Contains(combined, marker) {
			return "content blocked by upstream safety policy"
		}
	}

	return "upstream rejected the request"
}
