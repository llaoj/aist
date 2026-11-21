package server

import (
	"bytes"
	"encoding/json"
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

func classifyUpstreamError(status int, header http.Header, body []byte, now time.Time) classifiedUpstreamError {
	if status == 0 {
		return classifiedUpstreamError{
			Kind:    upstreamErrorKindTransient,
			Summary: "upstream transport error",
		}
	}

	code, errType, message := parseUpstreamErrorBody(body)
	summary := truncateBytes(util.FirstNonEmpty(message, code, errType, fmt.Sprintf("HTTP %d", status)), maxUpstreamErrorSummaryBytes)

	switch {
	case status == http.StatusPaymentRequired:
		return classifiedUpstreamError{Kind: upstreamErrorKindQuota, Summary: summary}

	case status == http.StatusTooManyRequests:
		if isQuotaExhausted(code, errType, message) {
			return classifiedUpstreamError{Kind: upstreamErrorKindQuota, Summary: summary}
		}
		return classifiedUpstreamError{
			Kind:       upstreamErrorKindRateLimit,
			RetryAfter: parseRetryAfter(header, now),
			Summary:    summary,
		}

	case status == http.StatusUnauthorized:
		return classifiedUpstreamError{Kind: upstreamErrorKindAuth, Summary: summary}

	case status == http.StatusForbidden:
		if isQuotaExhausted(code, errType, message) {
			return classifiedUpstreamError{Kind: upstreamErrorKindQuota, Summary: summary}
		}
		return classifiedUpstreamError{Kind: upstreamErrorKindAuth, Summary: summary}

	case status == http.StatusBadRequest,
		status == http.StatusRequestEntityTooLarge,
		status == http.StatusUnprocessableEntity:
		return classifiedUpstreamError{Kind: upstreamErrorKindBadRequest, Summary: summary}

	default:
		return classifiedUpstreamError{Kind: upstreamErrorKindTransient, Summary: summary}
	}
}

func parseUpstreamErrorBody(body []byte) (code, errType, message string) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return "", "", ""
	}

	var payload struct {
		Error   json.RawMessage `json:"error"`
		Errors  json.RawMessage `json:"errors"`
		Code    json.RawMessage `json:"code"`
		Type    string          `json:"type"`
		Message string          `json:"message"`
	}
	if err := json.Unmarshal(trimmed, &payload); err != nil {
		return "", "", ""
	}

	code = decodeCodeValue(payload.Code)
	errType = strings.TrimSpace(payload.Type)
	message = strings.TrimSpace(payload.Message)

	if nestedCode, nestedType, nestedMsg := parseErrorNode(payload.Error); nestedCode != "" || nestedType != "" || nestedMsg != "" {
		code = util.FirstNonEmpty(nestedCode, code)
		errType = util.FirstNonEmpty(nestedType, errType)
		message = util.FirstNonEmpty(nestedMsg, message)
	}
	if nestedCode, nestedType, nestedMsg := parseErrorNode(payload.Errors); nestedCode != "" || nestedType != "" || nestedMsg != "" {
		code = util.FirstNonEmpty(nestedCode, code)
		errType = util.FirstNonEmpty(nestedType, errType)
		message = util.FirstNonEmpty(nestedMsg, message)
	}
	return code, errType, message
}

func parseErrorNode(raw json.RawMessage) (code, errType, message string) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "", "", ""
	}
	if trimmed[0] == '"' {
		var text string
		if err := json.Unmarshal(trimmed, &text); err == nil {
			return "", "", strings.TrimSpace(text)
		}
		return "", "", ""
	}
	var node struct {
		Code    json.RawMessage `json:"code"`
		Type    string          `json:"type"`
		Message string          `json:"message"`
	}
	if err := json.Unmarshal(trimmed, &node); err != nil {
		return "", "", ""
	}
	return decodeCodeValue(node.Code), strings.TrimSpace(node.Type), strings.TrimSpace(node.Message)
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

func isQuotaExhausted(code, errType, message string) bool {
	lowerCode := strings.ToLower(code)
	lowerType := strings.ToLower(errType)

	for _, marker := range []string{
		"insufficient_quota",
		"insufficient_balance",
		"billing_hard_limit_reached",
		"accountquotaexceeded",
		"freetierquotaexceeded",
		"setlimitexceeded",
		"accountoverdueerror",
		"quota_exceeded",
	} {
		if strings.Contains(lowerCode, marker) || strings.Contains(lowerType, marker) {
			return true
		}
	}

	switch lowerCode {
	case "1113", "30001", "30011", "quotaexceeded":
		return true
	}

	lowerMsg := strings.ToLower(message)
	for _, phrase := range []string{
		"insufficient balance",
		"balance is insufficient",
		"exceeded your current quota",
		"exceeded the monthly usage quota",
		"daily quota",
		"today's quota",
		"free quota",
		"余额不足",
		"账户已欠费",
		"无可用资源包",
		"免费额度",
	} {
		if strings.Contains(lowerMsg, phrase) {
			return true
		}
	}
	return false
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
