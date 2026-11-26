package server

import (
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
