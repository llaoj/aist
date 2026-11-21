package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/llaoj/aist/internal/config"
	"github.com/llaoj/aist/internal/preferences"
	"github.com/llaoj/aist/internal/state"
)

func TestLangCommandShowAndSwitch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "zh_CN.UTF-8")

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	defer func() {
		rootCmd.SetOut(nil)
		rootCmd.SetArgs(nil)
	}()

	rootCmd.SetArgs([]string{"lang"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute lang: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != config.LanguageChinese {
		t.Fatalf("lang output = %q, want zh", got)
	}

	out.Reset()
	rootCmd.SetArgs([]string{"lang", "en"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute lang en: %v", err)
	}
	if !strings.Contains(out.String(), "(en)") {
		t.Fatalf("lang en output = %q", out.String())
	}

	out.Reset()
	rootCmd.SetArgs([]string{"lang"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute lang after switch: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != config.LanguageEnglish {
		t.Fatalf("lang output after switch = %q, want en", got)
	}

	rootCmd.SetArgs([]string{"lang", "fr"})
	if err := rootCmd.Execute(); err == nil || !strings.Contains(err.Error(), "choose zh or en") {
		t.Fatalf("Execute lang fr err = %v", err)
	}

	prefsPath := filepath.Join(home, state.DefaultRootName, "preferences.json")
	info, err := os.Stat(prefsPath)
	if err != nil {
		t.Fatalf("Stat %s: %v", prefsPath, err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 0600", info.Mode().Perm())
	}
	stateRoot := state.NewFromPaths(state.DefaultForHome(home))
	stored, err := preferences.NewStore(preferences.StoreOptions{State: stateRoot}).LoadOrInit()
	if err != nil {
		t.Fatalf("LoadOrInit: %v", err)
	}
	if stored.Language != config.LanguageEnglish {
		t.Fatalf("stored language = %q, want en", stored.Language)
	}

	// Corrupted preferences.json can be repaired directly by `aist lang zh`.
	if err := os.WriteFile(prefsPath, []byte("{corrupted"), 0o600); err != nil {
		t.Fatalf("WriteFile corrupted: %v", err)
	}
	out.Reset()
	rootCmd.SetArgs([]string{"lang", "zh"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute lang zh over corrupted file: %v", err)
	}
	repaired, err := preferences.NewStore(preferences.StoreOptions{State: stateRoot}).LoadOrInit()
	if err != nil {
		t.Fatalf("LoadOrInit after repair: %v", err)
	}
	if repaired.Language != config.LanguageChinese {
		t.Fatalf("repaired language = %q, want zh", repaired.Language)
	}
}
