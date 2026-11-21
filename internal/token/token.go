// Package token stores the PocketBase auth token issued by the AIST login flow.
//
// The token is the only credential the client holds. Real provider API keys
// never reach the client; model requests are proxied by the server.
package token

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/llaoj/aist/internal/state"
)

// EnvServer is the environment variable that can override the server URL.
const EnvServer = "AIST_SERVER"

const (
	tokenFileMode os.FileMode = 0o600
	stateDirMode  os.FileMode = 0o700
)

var errStateRootRequired = errors.New("token store state root is not configured")

// Credentials is the persisted login result.
type Credentials struct {
	ServerURL string    `json:"server_url"`
	Token     string    `json:"token,omitempty"`
	Email     string    `json:"email,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

// Store reads and writes the token file under the state root.
type Store struct {
	path     string
	lookup   func(string) (string, bool)
	readFile func(string) ([]byte, error)
	write    func(string, []byte, os.FileMode) error
}

// StoreOptions configures a Store.
type StoreOptions struct {
	// State holds the prepared state layout.
	State *state.Root
	// LookupEnv overrides environment lookup in tests.
	LookupEnv func(string) (string, bool)
	// ReadFile overrides file reading in tests.
	ReadFile func(string) ([]byte, error)
	// WriteFile overrides file writing in tests.
	WriteFile func(string, []byte, os.FileMode) error
}

// NewStore creates a token store.
func NewStore(opts StoreOptions) *Store {
	var path string
	if opts.State != nil {
		path = opts.State.Paths.TokenFile
	}
	lookup := opts.LookupEnv
	if lookup == nil {
		lookup = os.LookupEnv
	}
	readFile := opts.ReadFile
	if readFile == nil {
		readFile = os.ReadFile
	}
	write := opts.WriteFile
	if write == nil {
		write = os.WriteFile
	}
	return &Store{
		path:     path,
		lookup:   lookup,
		readFile: readFile,
		write:    write,
	}
}

// Path returns the token file path.
func (s *Store) Path() string {
	return s.path
}

// Load returns the effective credentials. A stored file is used when present;
// the AIST_SERVER environment variable overrides the server URL.
func (s *Store) Load() (Credentials, error) {
	creds, err := s.loadFile()
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return Credentials{}, err
		}
		creds = Credentials{}
	}

	if v, ok := s.lookup(EnvServer); ok {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			creds.ServerURL = trimmed
		}
	}
	return creds, nil
}

func (s *Store) loadFile() (Credentials, error) {
	if strings.TrimSpace(s.path) == "" {
		return Credentials{}, errStateRootRequired
	}
	data, err := s.readFile(s.path)
	if err != nil {
		return Credentials{}, err
	}
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return Credentials{}, fmt.Errorf("failed to parse token file %s: %w", s.path, err)
	}
	return creds, nil
}

// Save writes the credentials to the token file with 0600 permissions.
func (s *Store) Save(creds Credentials) error {
	if strings.TrimSpace(s.path) == "" {
		return errStateRootRequired
	}
	if strings.TrimSpace(creds.Token) == "" {
		return errors.New("auth token is required")
	}
	return s.writeCredentials(creds)
}

// SaveServerURL records the server a login is being attempted against, so a
// cancelled or failed login still defaults to it next time. It leaves an
// existing login untouched.
func (s *Store) SaveServerURL(serverURL string) error {
	serverURL = strings.TrimSpace(serverURL)
	if serverURL == "" {
		return nil
	}
	current, err := s.loadFile()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if strings.TrimSpace(current.Token) != "" {
		return nil
	}
	return s.writeCredentials(Credentials{ServerURL: serverURL})
}

func (s *Store) writeCredentials(creds Credentials) error {
	if strings.TrimSpace(s.path) == "" {
		return errStateRootRequired
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, stateDirMode); err != nil {
		return fmt.Errorf("failed to create state directory %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode token: %w", err)
	}
	data = append(data, '\n')
	if err := s.write(s.path, data, tokenFileMode); err != nil {
		return fmt.Errorf("failed to write token file %s: %w", s.path, err)
	}
	if err := os.Chmod(s.path, tokenFileMode); err != nil {
		return fmt.Errorf("failed to set permissions on %s: %w", s.path, err)
	}
	return nil
}

// Clear removes the stored token file. A missing file is not an error.
func (s *Store) Clear() error {
	if strings.TrimSpace(s.path) == "" {
		return errStateRootRequired
	}
	err := os.Remove(s.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
