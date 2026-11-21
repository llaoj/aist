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
		got := New(language).T("interactive.followup_limit_hint", 25)
		if strings.Contains(got, "%!") {
			t.Fatalf("%s hint has unused format args: %q", language, got)
		}
		if !strings.Contains(got, "25") {
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
