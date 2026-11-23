package interactive

import (
	"errors"

	"github.com/llaoj/aist/internal/llm"
	"github.com/llaoj/aist/internal/tools"
	"github.com/llaoj/aist/internal/util"
)

// RenderErrorMessage converts an execution or LLM proxy error into a localized
// user-facing description without an "Error:" prefix.
func RenderErrorMessage(translator tools.Translator, err error) string {
	if err == nil {
		return ""
	}
	if translator == nil {
		return err.Error()
	}

	if errors.Is(err, llm.ErrAuthentication) {
		return translator.T("error.auth_expired")
	}
	if errors.Is(err, ErrPipeQuestionRequired) {
		return translator.T("pipe.question_required")
	}

	var status *llm.StatusError
	if !errors.As(err, &status) {
		return err.Error()
	}

	switch status.ErrorCode {
	case llm.ErrorCodeQuotaExceeded:
		if !status.RetryAt.IsZero() {
			return translator.T("error.quota_exceeded_retry", util.FormatLocalMinute(status.RetryAt))
		}
		return translator.T("error.quota_exceeded")

	case llm.ErrorCodeNoAvailableModel:
		if !status.RetryAt.IsZero() {
			return translator.T("error.no_available_model_retry", util.FormatLocalMinute(status.RetryAt))
		}
		return translator.T("error.no_available_model")

	case llm.ErrorCodeUpstreamUnavailable:
		return translator.T("error.upstream_unavailable")

	case llm.ErrorCodeInvalidRequest:
		switch status.Message {
		case "context length exceeded":
			return translator.T("error.invalid_request_context")
		case "content blocked by upstream safety policy":
			return translator.T("error.invalid_request_safety")
		default:
			return translator.T("error.invalid_request")
		}
	}

	return err.Error()
}
