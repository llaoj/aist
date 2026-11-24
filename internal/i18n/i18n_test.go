package i18n

import (
	"strings"
	"testing"
)

func TestFileEditConfirmFormatsLocation(t *testing.T) {
	for _, language := range []string{"en", "zh"} {
		got := New(language).T("file_edit.confirm", "[web1] /etc/nginx/nginx.conf (sudo)")
		if strings.Contains(got, "%!") {
			t.Fatalf("%s confirm has unused format args: %q", language, got)
		}
		if !strings.Contains(got, "[web1] /etc/nginx/nginx.conf (sudo)") {
			t.Fatalf("%s confirm = %q", language, got)
		}
	}
}

func TestFollowUpLimitHintFormatsRoundCount(t *testing.T) {
	for _, language := range []string{"en", "zh"} {
		got := New(language).T("interactive.followup_limit_hint", 15)
		if strings.Contains(got, "%!") {
			t.Fatalf("%s hint has unused format args: %q", language, got)
		}
		if !strings.Contains(got, "15") {
			t.Fatalf("%s hint = %q, want round count", language, got)
		}
	}
}

func TestExecuteSuccessExitCodeFormatsCode(t *testing.T) {
	for _, language := range []string{"en", "zh"} {
		got := New(language).T("executor.execute_success_exit_code", 1)
		if strings.Contains(got, "%!") {
			t.Fatalf("%s message has unused format args: %q", language, got)
		}
		if !strings.Contains(got, "1") {
			t.Fatalf("%s message = %q, want exit code", language, got)
		}
	}
}

func TestMessagesKeyParity(t *testing.T) {
	for key := range EnglishMessages {
		if _, exists := ChineseMessages[key]; !exists {
			t.Errorf("missing key %q in ChineseMessages", key)
		}
	}
	for key := range ChineseMessages {
		if _, exists := EnglishMessages[key]; !exists {
			t.Errorf("missing key %q in EnglishMessages", key)
		}
	}
}

func TestWhoamiUsageMessagesFormatCleanly(t *testing.T) {
	for _, language := range []string{"en", "zh"} {
		tr := New(language)
		quotaMsg := tr.T("whoami.usage_quota", "42,180", "500,000", "457,820", 18)
		if strings.Contains(quotaMsg, "%!") {
			t.Fatalf("%s whoami.usage_quota has format mismatch: %q", language, quotaMsg)
		}
		unlimitedMsg := tr.T("whoami.usage_unlimited", "42,180", 18)
		if strings.Contains(unlimitedMsg, "%!") {
			t.Fatalf("%s whoami.usage_unlimited has format mismatch: %q", language, unlimitedMsg)
		}
	}
}
