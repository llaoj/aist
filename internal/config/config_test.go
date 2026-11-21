package config

import (
	"strings"
	"testing"
)

func validSettings() *Settings {
	return &Settings{
		SchemaVersion:   1,
		ProxyBaseURL:    "https://api.example.com/v1/proxy/",
		ContextWindow:   64000,
		MaxOutputTokens: 4096,
	}
}

func TestIsSupportedLanguage(t *testing.T) {
	if !IsSupportedLanguage(LanguageEnglish) || !IsSupportedLanguage(LanguageChinese) {
		t.Fatal("expected en and zh to be supported")
	}
	if IsSupportedLanguage("fr") || IsSupportedLanguage("") {
		t.Fatal("expected unsupported language to return false")
	}
}

func TestNormalize(t *testing.T) {
	settings := &Settings{
		ProxyBaseURL: "https://api.example.com/v1/proxy/",
	}
	settings.Normalize()
	if settings.SchemaVersion != SchemaVersion {
		t.Fatalf("schema version = %d", settings.SchemaVersion)
	}
	if settings.Session.MaxSessions != DefaultMaxSessions {
		t.Fatalf("max sessions = %d", settings.Session.MaxSessions)
	}
	if settings.ContextWindow != DefaultContextWindow || settings.MaxOutputTokens != DefaultMaxOutputTokens {
		t.Fatalf("limits = %d/%d", settings.ContextWindow, settings.MaxOutputTokens)
	}
	if settings.ProxyBaseURL != "https://api.example.com/v1/proxy" {
		t.Fatalf("proxy base url = %q", settings.ProxyBaseURL)
	}
}

func TestValidate(t *testing.T) {
	settings := validSettings()
	settings.Normalize()
	if err := settings.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got := settings.EffectiveContextWindow(); got != 64000 {
		t.Fatalf("context window = %d", got)
	}
	if got := settings.EffectiveMaxOutputTokens(); got != 4096 {
		t.Fatalf("max output = %d", got)
	}
	if got := settings.EffectiveProxyBaseURL(); got != "https://api.example.com/v1/proxy" {
		t.Fatalf("proxy = %q", got)
	}
}

func TestValidateRejectsMissingProxy(t *testing.T) {
	settings := validSettings()
	settings.ProxyBaseURL = ""
	err := settings.Validate()
	if err == nil || !strings.Contains(err.Error(), "proxy_base_url") {
		t.Fatalf("Validate err = %v", err)
	}
}

func TestValidateRejectsUnsupportedSchema(t *testing.T) {
	settings := validSettings()
	settings.SchemaVersion = 2
	err := settings.Validate()
	if err == nil || !strings.Contains(err.Error(), "schema version") {
		t.Fatalf("Validate err = %v", err)
	}
}
