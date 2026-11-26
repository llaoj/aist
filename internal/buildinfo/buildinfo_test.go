package buildinfo

import (
	"fmt"
	"runtime"
	"testing"
)

func TestUserAgent(t *testing.T) {
	orig := Default
	t.Cleanup(func() { Default = orig })

	Set("v0.7.7", "abc1234", "2026-11-26")
	want := fmt.Sprintf("aist/v0.7.7 (%s; %s)", runtime.GOOS, runtime.GOARCH)
	if got := UserAgent(); got != want {
		t.Fatalf("UserAgent() = %q, want %q", got, want)
	}

	Set("  ", "", "")
	wantFallback := fmt.Sprintf("aist/unknown (%s; %s)", runtime.GOOS, runtime.GOARCH)
	if got := UserAgent(); got != wantFallback {
		t.Fatalf("UserAgent() fallback = %q, want %q", got, wantFallback)
	}
}
