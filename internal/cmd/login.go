package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/llaoj/aist/internal/i18n"
	"github.com/llaoj/aist/internal/remote"
	"github.com/llaoj/aist/internal/token"
	"github.com/llaoj/aist/internal/ui"
	"github.com/llaoj/aist/internal/util"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	flagLoginEmail string
)

var loginCmd = &cobra.Command{
	Use:   "login [server]",
	Short: "Log in with your email and store the issued token",
	Long: `Log in to an AIST server with your email and password or a one-time password.

The server holds provider and model configuration; the client only stores the
issued token. Pass the server address as the positional argument, or override
it with AIST_SERVER.

Examples:
  aist login --email alice@example.com
  aist login --email alice@example.com https://aistrun.cc
  aist login --email alice@example.com 127.0.0.1:8090
  aist login 127.0.0.1:8090 --email alice@example.com
  aist login 127.0.0.1:8090   # prompts for email and password or verification code`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLogin(cmd.Context(), args)
	},
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove the stored login",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLogout(cmd.Context())
	},
}

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Show the current login",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWhoami(cmd.Context())
	},
}

func init() {
	markSkipAppInit(loginCmd)
	markSkipAppInit(logoutCmd)
	markSkipAppInit(whoamiCmd)
	loginCmd.Flags().StringVarP(&flagLoginEmail, "email", "e", "", "email to log in with")
	rootCmd.AddCommand(loginCmd, logoutCmd, whoamiCmd)
}

func newTokenStore(ctx context.Context) (*token.Store, error) {
	stateRoot := stateRootFromContext(ctx)
	if stateRoot == nil {
		return nil, errors.New("state root is not initialized")
	}
	return token.NewStore(token.StoreOptions{State: stateRoot}), nil
}

type loginPrompter struct {
	interactive  bool
	promptInput  func(prompt string) (string, error)
	promptSecret func(prompt string) (string, error)
}

func runLogin(ctx context.Context, args []string) error {
	return runLoginWithPrompter(ctx, args, loginPrompter{
		interactive:  term.IsTerminal(int(os.Stdin.Fd())),
		promptInput:  ui.PromptInput,
		promptSecret: ui.PromptSecret,
	})
}

func runLoginWithPrompter(ctx context.Context, args []string, prompter loginPrompter) error {
	store, err := newTokenStore(ctx)
	if err != nil {
		return err
	}
	current, err := store.Load()
	if err != nil {
		return err
	}

	serverArg := ""
	if len(args) > 0 {
		serverArg = args[0]
	}
	serverURL := normalizeServerURL(util.FirstNonEmpty(serverArg, current.ServerURL, remote.DefaultServerURL))
	// Best effort: remembering the server only improves the next login's
	// default; a write failure here surfaces again when the token is saved.
	_ = store.SaveServerURL(serverURL)
	email := strings.TrimSpace(flagLoginEmail)

	if email == "" {
		if !prompter.interactive {
			return errors.New("email is required: pass it with --email or run interactively")
		}
		entered, err := prompter.promptInput("Email: ")
		if err != nil {
			return err
		}
		email = strings.TrimSpace(entered)
	}
	if email == "" {
		return errors.New("email is required")
	}

	client := remote.NewClient(remote.ClientOptions{
		ServerURL: serverURL,
		Timeout:   15 * time.Second,
	})

	resp, err := authenticateWithServer(ctx, client, prompter, email)
	if err != nil {
		return err
	}

	creds := token.Credentials{
		ServerURL: serverURL,
		Token:     resp.Token,
		Email:     util.FirstNonEmpty(resp.Record.Email, email),
		ExpiresAt: remote.TokenExpiry(resp.Token),
	}

	if err := store.Save(creds); err != nil {
		return err
	}
	fmt.Printf("Logged in to %s as %s\n", creds.ServerURL, creds.Email)
	fmt.Printf("Credentials stored at %s\n", store.Path())

	prefStore, err := newPreferencesStore(ctx)
	if err != nil {
		return err
	}
	prefs, err := prefStore.LoadOrInit()
	if err != nil {
		return err
	}
	translator := i18n.New(prefs.Language)
	fmt.Println()
	fmt.Println(translator.T("login.quick_start_title"))
	fmt.Println(translator.T("login.quick_start_interactive"))
	fmt.Println(translator.T("login.quick_start_question"))
	fmt.Println(translator.T("login.quick_start_lang"))
	return nil
}

func authenticateWithServer(ctx context.Context, client *remote.Client, prompter loginPrompter, email string) (*remote.AuthResponse, error) {
	methods, err := client.AuthMethods(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load login methods: %w", err)
	}

	switch {
	case methods.Password.Enabled:
		return runPasswordLogin(ctx, client, prompter, methods, email)
	case methods.OTP.Enabled:
		resp, err := runOTPLogin(ctx, client, prompter, email, "")
		if err != nil {
			return nil, fmt.Errorf("login failed: %w", err)
		}
		return resp, nil
	default:
		return nil, errors.New("server does not allow password or OTP login")
	}
}

func runPasswordLogin(ctx context.Context, client *remote.Client, prompter loginPrompter, methods *remote.AuthMethods, email string) (*remote.AuthResponse, error) {
	password, err := prompter.promptSecret("Password: ")
	if err != nil {
		return nil, err
	}
	if password == "" {
		return nil, errors.New("password is required")
	}

	resp, err := client.AuthWithPassword(ctx, email, password)
	if err == nil {
		return resp, nil
	}
	if mfaID, requiresMFA := remote.MFARequired(err); requiresMFA {
		if !methods.OTP.Enabled {
			return nil, errors.New("login requires OTP, but OTP is not enabled on the server")
		}
		otpResp, otpErr := runOTPLogin(ctx, client, prompter, email, mfaID)
		if otpErr != nil {
			return nil, fmt.Errorf("login failed: %w", otpErr)
		}
		return otpResp, nil
	}
	if remote.IsBadRequest(err) || remote.IsUnauthorized(err) {
		return nil, fmt.Errorf("login failed: invalid email or password (forgot password? %s/register#reset)", client.ServerURL())
	}
	return nil, fmt.Errorf("login failed: %w", err)
}

func runOTPLogin(ctx context.Context, client *remote.Client, prompter loginPrompter, email, mfaID string) (*remote.AuthResponse, error) {
	otpID, err := client.RequestOTP(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("failed to request verification code: %w", err)
	}
	fmt.Printf("Verification code sent to %s\n", email)

	code, err := prompter.promptInput("Verification code: ")
	if err != nil {
		return nil, err
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, errors.New("verification code is required")
	}

	resp, err := client.AuthWithOTP(ctx, otpID, code, mfaID)
	if err != nil {
		if remote.IsBadRequest(err) {
			return nil, errors.New("invalid or expired verification code")
		}
		return nil, err
	}
	return resp, nil
}

func normalizeServerURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		scheme := "https"
		if isLocalServerHost(raw) {
			scheme = "http"
		}
		raw = scheme + "://" + raw
	}
	return strings.TrimRight(raw, "/")
}

func isLocalServerHost(raw string) bool {
	host := raw
	if i := strings.IndexByte(host, '/'); i >= 0 {
		host = host[:i]
	}
	if strings.HasPrefix(host, "[") {
		if end := strings.IndexByte(host, ']'); end >= 0 {
			host = host[1:end]
		}
	} else if strings.Count(host, ":") == 1 {
		if i := strings.LastIndexByte(host, ':'); i >= 0 {
			host = host[:i]
		}
	}
	host = strings.ToLower(host)
	return host == "localhost" || host == "::1" || strings.HasPrefix(host, "127.")
}

func runLogout(ctx context.Context) error {
	store, err := newTokenStore(ctx)
	if err != nil {
		return err
	}
	if err := store.Clear(); err != nil {
		return err
	}
	fmt.Println("Logged out.")
	return nil
}

func runWhoami(ctx context.Context) error {
	store, err := newTokenStore(ctx)
	if err != nil {
		return err
	}
	creds, err := store.Load()
	if err != nil {
		return err
	}
	if strings.TrimSpace(creds.Token) == "" {
		return errors.New("not logged in; run `aist login`")
	}
	server := creds.ServerURL
	if server == "" {
		server = remote.DefaultServerURL
	}
	if creds.Email != "" {
		fmt.Printf("Email:    %s\n", creds.Email)
	}
	fmt.Printf("Server:   %s\n", server)
	fmt.Println("Status:   logged in")
	return nil
}
