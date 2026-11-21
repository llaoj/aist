// Package auth manages the persisted login and refreshes the auth token.
package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/llaoj/aist/internal/remote"
	"github.com/llaoj/aist/internal/token"
	"github.com/llaoj/aist/internal/util"
)

// ErrNotLoggedIn indicates that no auth token is stored.
var ErrNotLoggedIn = errors.New("not logged in")

// ErrLoginRequired indicates that the stored auth token is no longer valid.
var ErrLoginRequired = errors.New("login required")

const defaultRefreshSkew = time.Minute

// ManagerOptions configures a Manager.
type ManagerOptions struct {
	Store       *token.Store
	Client      *remote.Client
	Now         func() time.Time
	RefreshSkew time.Duration
}

// Manager loads the login state and refreshes the auth token on demand.
type Manager struct {
	mu     sync.Mutex
	store  *token.Store
	client *remote.Client
	now    func() time.Time
	skew   time.Duration

	credentials token.Credentials
	loaded      bool
}

// NewManager creates a Manager.
func NewManager(opts ManagerOptions) *Manager {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	skew := opts.RefreshSkew
	if skew <= 0 {
		skew = defaultRefreshSkew
	}
	return &Manager{
		store:  opts.Store,
		client: opts.Client,
		now:    now,
		skew:   skew,
	}
}

// Startup forces a refresh and returns the new auth token plus the account
// configuration.
func (m *Manager) Startup(ctx context.Context) (*remote.AuthResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.loadLocked(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(m.credentials.Token) == "" {
		return nil, ErrNotLoggedIn
	}
	return m.refreshLocked(ctx)
}

// AccessToken returns a usable auth token, refreshing it when necessary.
func (m *Manager) AccessToken(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.loadLocked(); err != nil {
		return "", err
	}
	if strings.TrimSpace(m.credentials.Token) == "" {
		return "", ErrNotLoggedIn
	}
	if !m.expiredLocked() {
		return m.credentials.Token, nil
	}
	if _, err := m.refreshLocked(ctx); err != nil {
		return "", err
	}
	return m.credentials.Token, nil
}

// RefreshAccessToken forces a refresh, for example after a proxy 401.
func (m *Manager) RefreshAccessToken(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.loadLocked(); err != nil {
		return "", err
	}
	if strings.TrimSpace(m.credentials.Token) == "" {
		return "", ErrNotLoggedIn
	}
	if _, err := m.refreshLocked(ctx); err != nil {
		return "", err
	}
	return m.credentials.Token, nil
}

func (m *Manager) loadLocked() error {
	if m.loaded {
		return nil
	}
	credentials, err := m.store.Load()
	if err != nil {
		return err
	}
	m.credentials = credentials
	m.loaded = true
	return nil
}

func (m *Manager) expiredLocked() bool {
	if m.credentials.ExpiresAt.IsZero() {
		return false
	}
	return !m.now().Add(m.skew).Before(m.credentials.ExpiresAt)
}

func (m *Manager) refreshLocked(ctx context.Context) (*remote.AuthResponse, error) {
	tokenValue := strings.TrimSpace(m.credentials.Token)
	if tokenValue == "" {
		return nil, ErrNotLoggedIn
	}

	resp, err := m.client.Refresh(ctx, tokenValue)
	if err != nil {
		if remote.IsUnauthorized(err) {
			return nil, ErrLoginRequired
		}
		return nil, fmt.Errorf("failed to refresh login: %w", err)
	}

	credentials := token.Credentials{
		ServerURL: util.FirstNonEmpty(m.credentials.ServerURL),
		Token:     resp.Token,
		Email:     util.FirstNonEmpty(resp.Record.Email, m.credentials.Email),
		ExpiresAt: remote.TokenExpiry(resp.Token),
	}
	if err := m.store.Save(credentials); err != nil {
		return nil, err
	}
	m.credentials = credentials
	return resp, nil
}
