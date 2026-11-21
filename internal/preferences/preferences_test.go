package preferences

import (
	"errors"
	"os"
	"testing"

	"github.com/llaoj/aist/internal/config"
	"github.com/llaoj/aist/internal/state"
)

func newTestStateRoot(t *testing.T) *state.Root {
	t.Helper()
	root, err := state.EnsureRoot(t.TempDir())
	if err != nil {
		t.Fatalf("EnsureRoot: %v", err)
	}
	return root
}

func envMap(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		val, exists := values[key]
		return val, exists
	}
}

func TestLoadOrInitInfersAndPersistsOnce(t *testing.T) {
	cases := []struct {
		name         string
		env          map[string]string
		systemLocale string
		want         string
	}{
		{
			name: "default english",
			want: config.LanguageEnglish,
		},
		{
			name: "lang chinese",
			env:  map[string]string{"LANG": "zh_CN.UTF-8"},
			want: config.LanguageChinese,
		},
		{
			name: "lc_all overrides lang",
			env:  map[string]string{"LC_ALL": "zh_TW.UTF-8", "LANG": "en_US.UTF-8"},
			want: config.LanguageChinese,
		},
		{
			name: "lc_messages overrides lang",
			env:  map[string]string{"LC_MESSAGES": "zh-Hans", "LANG": "en_US.UTF-8"},
			want: config.LanguageChinese,
		},
		{
			name:         "explicit c locale ignores darwin fallback",
			env:          map[string]string{"LC_ALL": "C.UTF-8"},
			systemLocale: "zh-Hans-CN",
			want:         config.LanguageEnglish,
		},
		{
			name:         "encoding only lang falls back to system locale",
			env:          map[string]string{"LANG": "UTF-8"},
			systemLocale: "zh-Hans-CN",
			want:         config.LanguageChinese,
		},
		{
			name:         "unset env falls back to system locale",
			systemLocale: "zh_CN",
			want:         config.LanguageChinese,
		},
		{
			name:         "encoding only lc_messages falls through to lang",
			env:          map[string]string{"LC_MESSAGES": "UTF-8", "LANG": "zh_CN.UTF-8"},
			systemLocale: "",
			want:         config.LanguageChinese,
		},
		{
			name: "non-zh prefix like zha is english",
			env:  map[string]string{"LANG": "zha_CN.UTF-8"},
			want: config.LanguageEnglish,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stateRoot := newTestStateRoot(t)
			store := NewStore(StoreOptions{
				State:        stateRoot,
				LookupEnv:    envMap(tc.env),
				SystemLocale: func() string { return tc.systemLocale },
			})

			got, err := store.LoadOrInit()
			if err != nil {
				t.Fatalf("LoadOrInit: %v", err)
			}
			if got.Language != tc.want {
				t.Fatalf("Language = %q, want %q", got.Language, tc.want)
			}

			info, err := os.Stat(store.Path())
			if err != nil {
				t.Fatalf("Stat preferences: %v", err)
			}
			if info.Mode().Perm() != 0o600 {
				t.Fatalf("mode = %o, want 0600", info.Mode().Perm())
			}

			// Changing environment after persistence must not alter stored preference.
			second := NewStore(StoreOptions{
				State:        stateRoot,
				LookupEnv:    envMap(map[string]string{"LC_ALL": "zh_CN.UTF-8"}),
				SystemLocale: func() string { return "zh-Hans-CN" },
			})
			reloaded, err := second.LoadOrInit()
			if err != nil {
				t.Fatalf("second LoadOrInit: %v", err)
			}
			if reloaded.Language != tc.want {
				t.Fatalf("reloaded Language = %q, want %q", reloaded.Language, tc.want)
			}
		})
	}
}

func TestSaveAndRejectUnsupportedLanguage(t *testing.T) {
	stateRoot := newTestStateRoot(t)
	store := NewStore(StoreOptions{
		State:        stateRoot,
		LookupEnv:    envMap(nil),
		SystemLocale: func() string { return "" },
	})

	if err := store.Save(Data{Language: "ZH"}); err != nil {
		t.Fatalf("Save ZH: %v", err)
	}
	got, err := store.LoadOrInit()
	if err != nil {
		t.Fatalf("LoadOrInit: %v", err)
	}
	if got.Language != config.LanguageChinese {
		t.Fatalf("Language = %q, want zh", got.Language)
	}

	if err := store.Save(Data{Language: "fr"}); !errors.Is(err, ErrUnsupportedLanguage) {
		t.Fatalf("Save fr err = %v, want ErrUnsupportedLanguage", err)
	}
}

func TestLoadOrInitRepairsEmptyLanguageField(t *testing.T) {
	for _, content := range []string{"", "  \n", "{}\n"} {
		stateRoot := newTestStateRoot(t)
		if err := os.WriteFile(stateRoot.Paths.PreferencesFile, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}

		store := NewStore(StoreOptions{
			State:     stateRoot,
			LookupEnv: envMap(map[string]string{"LANG": "zh_CN.UTF-8"}),
		})
		got, err := store.LoadOrInit()
		if err != nil {
			t.Fatalf("LoadOrInit(%q): %v", content, err)
		}
		if got.Language != config.LanguageChinese {
			t.Fatalf("Language = %q, want zh", got.Language)
		}
	}
}

func TestParseDarwinPrimaryLanguage(t *testing.T) {
	raw := "(\n    \"zh-Hans-CN\",\n    \"en-CN\"\n)\n"
	if got := parseDarwinPrimaryLanguage(raw); got != "zh-Hans-CN" {
		t.Fatalf("got %q, want zh-Hans-CN", got)
	}
	if got := parseDarwinPrimaryLanguage("(\n)\n"); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}
