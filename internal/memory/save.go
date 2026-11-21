package memory

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ChangeOperation is the requested change type.
type ChangeOperation string

const (
	ChangeOperationUnknown ChangeOperation = ""
	ChangeOperationAppend  ChangeOperation = "append"
	ChangeOperationReplace ChangeOperation = "replace"
)

// MaxSaveContentBytes limits a single save payload to a concise summary.
const MaxSaveContentBytes = 8 * 1024

// ErrSensitiveContent means the payload looked like it contained secrets.
var ErrSensitiveContent = errors.New("memory content appears to contain secrets")

var (
	privateBlockPattern = regexp.MustCompile(`(?is)<private>.*?</private>`)
	secretPatterns      = []*regexp.Regexp{
		regexp.MustCompile(`(?i)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----`),
		regexp.MustCompile(`(?i)\b(password|passwd|pwd|token|secret|api[_-]?key|access[_-]?key|private[_-]?key)\b\s*[:=：]\s*\S+`),
		regexp.MustCompile(`(?i)(密码|口令|令牌)\s*[:=：]\s*\S+`),
		regexp.MustCompile(`(?i)\bauthorization\b\s*[:=]\s*(bearer\s+)?\S+`),
		regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
		regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}\b`),
	}
)

// ChangeRequest describes a Memory change.
type ChangeRequest struct {
	Name      string
	Content   string
	Summary   string
	Type      string
	Operation ChangeOperation
	Source    string
}

// ChangeAction describes the concrete effect of a Memory change.
type ChangeAction string

const (
	ChangeActionCreated  ChangeAction = "created"
	ChangeActionUpdated  ChangeAction = "updated"
	ChangeActionReplaced ChangeAction = "replaced"
)

// ChangeResult describes a successful Memory change.
type ChangeResult struct {
	Entry     Entry
	Operation ChangeOperation
	Action    ChangeAction
}

// ParseChangeOperation normalizes a save mode. Empty defaults to append.
func ParseChangeOperation(value string) (ChangeOperation, error) {
	switch ChangeOperation(strings.ToLower(strings.TrimSpace(value))) {
	case ChangeOperationUnknown, ChangeOperationAppend:
		return ChangeOperationAppend, nil
	case ChangeOperationReplace:
		return ChangeOperationReplace, nil
	default:
		return "", fmt.Errorf("unsupported memory save mode %q", value)
	}
}

// SanitizeContent removes private blocks and rejects likely secrets.
func SanitizeContent(content string) (string, error) {
	content = privateBlockPattern.ReplaceAllString(content, "")
	content = strings.TrimSpace(content)
	if content == "" {
		return "", errors.New("memory content is empty after removing <private> sections")
	}
	if len(content) > MaxSaveContentBytes {
		return "", fmt.Errorf("memory content is too large (%d bytes); save a concise summary, not raw output", len(content))
	}
	if err := checkSecrets(content); err != nil {
		return "", err
	}
	return content, nil
}

func sanitizeField(value string) (string, error) {
	value = privateBlockPattern.ReplaceAllString(value, "")
	value = strings.TrimSpace(value)
	if err := checkSecrets(value); err != nil {
		return "", err
	}
	return value, nil
}

func checkSecrets(text string) error {
	for _, pattern := range secretPatterns {
		if pattern.MatchString(text) {
			return ErrSensitiveContent
		}
	}
	return nil
}
