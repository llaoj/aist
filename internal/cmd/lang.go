package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/llaoj/aist/internal/config"
	"github.com/llaoj/aist/internal/i18n"
	"github.com/llaoj/aist/internal/preferences"
	"github.com/spf13/cobra"
)

var langCmd = &cobra.Command{
	Use:   "lang [zh|en]",
	Short: "Show or switch the client display and reply language",
	Long: `Show or switch the local language preference stored in ~/.aist/preferences.json.

Examples:
  aist lang      # show current language (zh or en)
  aist lang zh   # switch to Simplified Chinese
  aist lang en   # switch to English`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLang(cmd, args)
	},
}

func init() {
	markSkipAppInit(langCmd)
	rootCmd.AddCommand(langCmd)
}

func newPreferencesStore(ctx context.Context) (*preferences.Store, error) {
	stateRoot := stateRootFromContext(ctx)
	if stateRoot == nil {
		return nil, errors.New("state root is not initialized")
	}
	return preferences.NewStore(preferences.StoreOptions{State: stateRoot}), nil
}

func runLang(cmd *cobra.Command, args []string) error {
	store, err := newPreferencesStore(cmd.Context())
	if err != nil {
		return err
	}
	if len(args) == 0 {
		prefs, err := store.LoadOrInit()
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), prefs.Language)
		return nil
	}

	target := strings.ToLower(strings.TrimSpace(args[0]))
	if !config.IsSupportedLanguage(target) {
		return fmt.Errorf("unsupported language %q; choose zh or en", args[0])
	}
	if err := store.Save(preferences.Data{Language: target}); err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), i18n.New(target).T("lang.switched"))
	return nil
}
