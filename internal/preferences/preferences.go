// Package preferences stores client-local user preferences under ~/.aist.
package preferences

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/llaoj/aist/internal/config"
	"github.com/llaoj/aist/internal/state"
	"github.com/llaoj/aist/internal/storage"
)

const (
	preferencesFileMode os.FileMode = 0o600
	stateDirMode        os.FileMode = 0o700
)

var ErrUnsupportedLanguage = errors.New("unsupported language")

var errStateRootRequired = errors.New("preferences store state root is not configured")

type Data struct {
	Language string `json:"language"`
}

type Store struct {
	path         string
	lookup       func(string) (string, bool)
	systemLocale func() string
	readFile     func(string) ([]byte, error)
	writeFile    func(string, []byte, storage.WriteOptions) error
}

type StoreOptions struct {
	State        *state.Root
	LookupEnv    func(string) (string, bool)
	SystemLocale func() string
	ReadFile     func(string) ([]byte, error)
	WriteFile    func(string, []byte, storage.WriteOptions) error
}

func NewStore(opts StoreOptions) *Store {
	var path string
	if opts.State != nil {
		path = opts.State.Paths.PreferencesFile
	}
	lookup := opts.LookupEnv
	if lookup == nil {
		lookup = os.LookupEnv
	}
	systemLocale := opts.SystemLocale
	if systemLocale == nil {
		systemLocale = defaultSystemLocale
	}
	readFile := opts.ReadFile
	if readFile == nil {
		readFile = os.ReadFile
	}
	writeFile := opts.WriteFile
	if writeFile == nil {
		writeFile = storage.WriteFile
	}
	return &Store{
		path:         path,
		lookup:       lookup,
		systemLocale: systemLocale,
		readFile:     readFile,
		writeFile:    writeFile,
	}
}

func (s *Store) Path() string {
	return s.path
}

// LoadOrInit returns stored preferences, or infers and persists defaults when
// preferences.json does not exist yet or has no valid language configured.
func (s *Store) LoadOrInit() (Data, error) {
	if strings.TrimSpace(s.path) == "" {
		return Data{}, errStateRootRequired
	}
	raw, err := s.readFile(s.path)
	if err == nil && len(bytes.TrimSpace(raw)) > 0 {
		var stored Data
		if err := json.Unmarshal(raw, &stored); err != nil {
			return Data{}, fmt.Errorf("failed to parse preferences file %s: %w", s.path, err)
		}
		language := strings.ToLower(strings.TrimSpace(stored.Language))
		if config.IsSupportedLanguage(language) {
			return Data{Language: language}, nil
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Data{}, err
	}

	inferred := Data{Language: s.inferLanguage()}
	if err := s.Save(inferred); err != nil {
		return Data{}, err
	}
	return inferred, nil
}

func (s *Store) Save(data Data) error {
	if strings.TrimSpace(s.path) == "" {
		return errStateRootRequired
	}
	language := strings.ToLower(strings.TrimSpace(data.Language))
	if !config.IsSupportedLanguage(language) {
		return fmt.Errorf("%w: %s", ErrUnsupportedLanguage, data.Language)
	}
	encoded, err := json.MarshalIndent(Data{Language: language}, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode preferences: %w", err)
	}
	encoded = append(encoded, '\n')
	if err := s.writeFile(s.path, encoded, storage.WriteOptions{
		FileMode: preferencesFileMode,
		DirMode:  stateDirMode,
	}); err != nil {
		return fmt.Errorf("failed to write preferences file %s: %w", s.path, err)
	}
	return nil
}

func (s *Store) inferLanguage() string {
	if envLocale := s.firstLocaleEnv(); envLocale != "" {
		if isChineseLocale(envLocale) {
			return config.LanguageChinese
		}
		return config.LanguageEnglish
	}
	if isChineseLocale(strings.ToLower(strings.TrimSpace(s.systemLocale()))) {
		return config.LanguageChinese
	}
	return config.LanguageEnglish
}

func (s *Store) firstLocaleEnv() string {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		val, _ := s.lookup(key)
		locale := strings.ToLower(strings.TrimSpace(val))
		if locale != "" && !isEncodingOnlyLocale(locale) {
			return locale
		}
	}
	return ""
}

// isEncodingOnlyLocale matches terminals such as iTerm2 on macOS that export
// LANG=UTF-8 without a language tag.
func isEncodingOnlyLocale(locale string) bool {
	return locale == "utf-8" || locale == "utf8"
}

func isChineseLocale(locale string) bool {
	return locale == "zh" ||
		strings.HasPrefix(locale, "zh_") ||
		strings.HasPrefix(locale, "zh-") ||
		strings.HasPrefix(locale, "zh.")
}

func defaultSystemLocale() string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "/usr/bin/defaults", "read", "-g", "AppleLanguages").Output(); err == nil {
		if primary := parseDarwinPrimaryLanguage(string(out)); primary != "" {
			return primary
		}
	}
	if out, err := exec.CommandContext(ctx, "/usr/bin/defaults", "read", "-g", "AppleLocale").Output(); err == nil {
		return strings.TrimSpace(string(out))
	}
	return ""
}

func parseDarwinPrimaryLanguage(raw string) string {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimPrefix(trimmed, "(")
	trimmed = strings.TrimSuffix(trimmed, ")")
	for _, part := range strings.Split(trimmed, ",") {
		candidate := strings.Trim(strings.TrimSpace(part), `"'`)
		if candidate != "" {
			return candidate
		}
	}
	return ""
}
