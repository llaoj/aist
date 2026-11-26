package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/llaoj/aist/internal/util"
)

type upstreamErrorKind int

const (
	upstreamErrorKindUnknown upstreamErrorKind = iota
	upstreamErrorKindQuota
	upstreamErrorKindRateLimit
	upstreamErrorKindAuth
	upstreamErrorKindTransient
	upstreamErrorKindBadRequest
)

const maxUpstreamErrorSummaryBytes = 512

type classifiedUpstreamError struct {
	Kind       upstreamErrorKind
	RetryAfter time.Duration
	Summary    string
}

func classifyUpstreamError(status int, header http.Header, body []byte, cause error, now time.Time) classifiedUpstreamError {
	if status == 0 {
		summary := "upstream transport error"
		if errors.Is(cause, context.DeadlineExceeded) {
			summary = "upstream request timed out"
		}
		return classifiedUpstreamError{
			Kind:    upstreamErrorKindTransient,
			Summary: summary,
		}
	}

	summary := summarizeUpstreamError(status, body)

	switch status {
	case http.StatusPaymentRequired:
		return classifiedUpstreamError{Kind: upstreamErrorKindQuota, Summary: summary}

	case http.StatusTooManyRequests:
		return classifiedUpstreamError{
			Kind:       upstreamErrorKindRateLimit,
			RetryAfter: parseRetryAfter(header, now),
			Summary:    summary,
		}

	case http.StatusUnauthorized, http.StatusForbidden:
		return classifiedUpstreamError{Kind: upstreamErrorKindAuth, Summary: summary}

	case http.StatusBadRequest,
		http.StatusRequestEntityTooLarge,
		http.StatusUnprocessableEntity:
		return classifiedUpstreamError{Kind: upstreamErrorKindBadRequest, Summary: summary}

	default:
		return classifiedUpstreamError{Kind: upstreamErrorKindTransient, Summary: summary}
	}
}

func summarizeUpstreamError(status int, body []byte) string {
	code, errType, message := parseUpstreamErrorBody(body)
	return truncateBytes(util.FirstNonEmpty(message, code, errType, fmt.Sprintf("HTTP %d", status)), maxUpstreamErrorSummaryBytes)
}

func parseUpstreamErrorBody(body []byte) (code, errType, message string) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return "", "", ""
	}
	if trimmed[0] == '[' {
		return parseErrorArray(trimmed)
	}

	var payload struct {
		Error   json.RawMessage `json:"error"`
		Errors  json.RawMessage `json:"errors"`
		Detail  json.RawMessage `json:"detail"`
		Code    json.RawMessage `json:"code"`
		Type    string          `json:"type"`
		Message string          `json:"message"`
		Msg     string          `json:"msg"`
	}
	if err := json.Unmarshal(trimmed, &payload); err != nil {
		return "", "", ""
	}

	code = decodeCodeValue(payload.Code)
	errType = strings.TrimSpace(payload.Type)
	message = util.FirstNonEmpty(strings.TrimSpace(payload.Message), strings.TrimSpace(payload.Msg))

	for _, raw := range []json.RawMessage{payload.Error, payload.Errors, payload.Detail} {
		if nestedCode, nestedType, nestedMsg := parseErrorNode(raw); nestedCode != "" || nestedType != "" || nestedMsg != "" {
			code = util.FirstNonEmpty(nestedCode, code)
			errType = util.FirstNonEmpty(nestedType, errType)
			message = util.FirstNonEmpty(nestedMsg, message)
		}
	}
	return code, errType, message
}

func parseErrorArray(raw []byte) (code, errType, message string) {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return "", "", ""
	}
	for _, item := range items {
		c, t, m := parseErrorNode(item)
		if c != "" || t != "" || m != "" {
			return c, t, m
		}
	}
	return "", "", ""
}

func parseErrorNode(raw json.RawMessage) (code, errType, message string) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "", "", ""
	}
	switch trimmed[0] {
	case '"':
		var text string
		if err := json.Unmarshal(trimmed, &text); err == nil {
			return "", "", strings.TrimSpace(text)
		}
		return "", "", ""
	case '[':
		return parseErrorArray(trimmed)
	case '{':
		return parseUpstreamErrorBody(trimmed)
	default:
		return "", "", ""
	}
}

func decodeCodeValue(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ""
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err == nil {
			return strings.TrimSpace(s)
		}
	}
	return string(trimmed)
}

func parseRetryAfter(header http.Header, now time.Time) time.Duration {
	if header == nil {
		return 0
	}
	raw := strings.TrimSpace(header.Get("Retry-After"))
	if raw == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(raw); err == nil {
		if seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
		return 0
	}
	if until, err := http.ParseTime(raw); err == nil && until.After(now) {
		return until.Sub(now)
	}
	return 0
}

func truncateBytes(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	for cut > 0 && (s[cut]&0xC0) == 0x80 {
		cut--
	}
	return s[:cut]
}
