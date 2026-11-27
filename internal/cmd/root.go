package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/llaoj/aist/internal/app"
	"github.com/llaoj/aist/internal/auth"
	"github.com/llaoj/aist/internal/i18n"
	"github.com/llaoj/aist/internal/interactive"
	"github.com/llaoj/aist/internal/preferences"
	"github.com/llaoj/aist/internal/state"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var rootCmd = &cobra.Command{
	Use:   "aist [question]",
	Short: "AI Shell Assistant",
	Long: `AI Shell Assistant (aist)

Examples:
  aist "your question"     # Ask question and exit
  cmd | aist "question"    # Analyze piped data with context
  aist lang zh             # Switch display and reply language (zh or en)`,
	FParseErrWhitelist: cobra.FParseErrWhitelist{
		UnknownFlags: true,
	},
	DisableFlagParsing: false,
	SilenceUsage:       true,
	SilenceErrors:      true,
	Args:               cobra.ArbitraryArgs,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if skipsStateInit(cmd) {
			return nil
		}
		if !cmd.HasParent() && len(args) > 0 && args[0] == "memory" {
			return errors.New("the `aist memory` command is no longer supported; Memory is maintained through conversation")
		}
		stateRoot, err := state.EnsureRoot("")
		if err != nil {
			return err
		}
		if !(cmd == langCmd && len(args) > 0) {
			if _, err := preferences.NewStore(preferences.StoreOptions{State: stateRoot}).LoadOrInit(); err != nil {
				return err
			}
		}
		ctx := withStateRoot(cmd.Context(), stateRoot)
		cmd.SetContext(ctx)

		if skipsAppInit(cmd) {
			return nil
		}
		application, err := app.New(ctx, app.Options{State: stateRoot})
		if err != nil {
			if errors.Is(err, auth.ErrNotLoggedIn) {
				return errors.New("not logged in; run `aist login`")
			}
			if errors.Is(err, auth.ErrLoginRequired) {
				return errors.New("login expired; run `aist login`")
			}
			return err
		}
		cmd.SetContext(app.WithContext(ctx, application))
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		var initialQuestion string
		if len(args) > 0 {
			initialQuestion = args[0]
		}

		fileInfo, _ := os.Stdin.Stat()
		if (fileInfo.Mode() & os.ModeCharDevice) == 0 {
			runPipeMode(cmd, initialQuestion)
		} else {
			runInteractiveMode(cmd, initialQuestion)
		}
	},
}

type stateContextKey struct{}

func withStateRoot(ctx context.Context, root *state.Root) context.Context {
	return context.WithValue(ctx, stateContextKey{}, root)
}

func stateRootFromContext(ctx context.Context) *state.Root {
	root, _ := ctx.Value(stateContextKey{}).(*state.Root)
	return root
}

const (
	skipStateInitAnnotation = "aist/skip-state-init"
	skipAppInitAnnotation   = "aist/skip-app-init"
)

func markSkipStateInit(cmd *cobra.Command) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[skipStateInitAnnotation] = "true"
}

func skipsStateInit(cmd *cobra.Command) bool {
	if cmd.Annotations[skipStateInitAnnotation] == "true" {
		return true
	}
	path := cmd.CommandPath()
	if path == "aist help" || path == "aist completion" || strings.HasPrefix(path, "aist completion ") {
		return true
	}
	return strings.HasPrefix(path, "aist __complete")
}

func markSkipAppInit(cmd *cobra.Command) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[skipAppInitAnnotation] = "true"
}

func skipsAppInit(cmd *cobra.Command) bool {
	return skipsStateInit(cmd) || cmd.Annotations[skipAppInitAnnotation] == "true"
}

func Execute() error {
	return rootCmd.Execute()
}

func runInteractiveMode(cmd *cobra.Command, initialQuestion string) {
	runInteractiveSession(cmd, initialQuestion, nil)
}

func runInteractiveSession(cmd *cobra.Command, initialQuestion string, pipeReader io.Reader) {
	application := app.FromContext(cmd.Context())
	if application == nil {
		return
	}
	translator := application.Translator()

	session, err := application.NewSession(interactive.ModeInteractive)
	if err != nil {
		color.Red("%s\n", err)
		return
	}
	if pipeReader != nil {
		session.SetPipeReader(pipeReader)
	}

	err = session.Run(initialQuestion)
	if err != nil {
		color.Red(translator.T("error.general", interactive.RenderErrorMessage(translator, err)))
	} else {
		// Run returns nil only on ctrl+c (the interactive loop is otherwise infinite).
		// BubbleTea already restored the terminal and left a blank line, so print goodbye directly.
		printGoodbye(translator)
	}
}

func printGoodbye(translator *i18n.I18n) {
	color.Green(translator.T("interactive.goodbye"))
}

func runPipeMode(cmd *cobra.Command, initialQuestion string) {
	application := app.FromContext(cmd.Context())
	if application == nil {
		return
	}
	translator := application.Translator()

	pipeData, err := io.ReadAll(io.LimitReader(os.Stdin, interactive.MaxPipeDataBytes))
	if err != nil {
		color.Red(translator.T("error.general", interactive.RenderErrorMessage(translator, err)))
		return
	}

	if tty := openControllingTTY(); tty != nil {
		_ = os.Stdin.Close()
		os.Stdin = tty
		defer tty.Close()
		runInteractiveSession(cmd, initialQuestion, bytes.NewReader(pipeData))
		return
	}

	if strings.TrimSpace(initialQuestion) == "" {
		color.Red(translator.T("error.general", interactive.RenderErrorMessage(translator, interactive.ErrPipeQuestionRequired)))
		return
	}

	session, err := application.NewSession(interactive.ModePipe)
	if err != nil {
		color.Red("%s\n", err)
		return
	}
	session.SetPipeReader(bytes.NewReader(pipeData))

	err = session.RunPipeAnalysis(initialQuestion)
	if err != nil && !errors.Is(err, interactive.ErrInterrupted) {
		color.Red(translator.T("error.general", interactive.RenderErrorMessage(translator, err)))
	}
}

func openControllingTTY() *os.File {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return nil
	}
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil
	}
	if !term.IsTerminal(int(tty.Fd())) {
		_ = tty.Close()
		return nil
	}
	return tty
}
