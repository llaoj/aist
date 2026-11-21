package cmd

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/llaoj/aist/internal/app"
	"github.com/llaoj/aist/internal/interactive"
	"github.com/llaoj/aist/internal/session"
	"github.com/llaoj/aist/internal/ui"
	"github.com/spf13/cobra"
)

var sessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "Resume a saved session",
	Long:  "List recent sessions and continue the selected conversation.",
	RunE: func(cmd *cobra.Command, args []string) error {
		application := app.FromContext(cmd.Context())
		if application == nil {
			return fmt.Errorf("application is not initialized")
		}
		translator := application.Translator()

		summaries, err := application.ListSessions(cmd.Context())
		if err != nil {
			return fmt.Errorf(translator.T("sessions.list_failed"), err)
		}
		if len(summaries) == 0 {
			fmt.Println(translator.T("sessions.empty"))
			return nil
		}

		options := make([]string, 0, len(summaries))
		for _, summary := range summaries {
			options = append(options, formatSessionListEntry(summary))
		}
		index, err := ui.PromptSelect(translator.T("sessions.title"), translator.T("sessions.key_hint"), options)
		if err != nil {
			if errors.Is(err, ui.ErrInterrupted) {
				return nil
			}
			return err
		}

		selected := summaries[index]
		chatSession, err := application.NewSessionWithID(selected.ID, interactive.ModeInteractive)
		if err != nil {
			return err
		}
		title := selected.Title
		if title == "" {
			title = "(untitled)"
		}
		fmt.Println(translator.T("sessions.restored", title, selected.MessageCount))
		fmt.Println()
		if err := chatSession.Resume(); err != nil {
			color.Red(translator.T("error.general", interactive.RenderErrorMessage(translator, err)))
			return nil
		}
		printGoodbye(translator)
		return nil
	},
}

func formatSessionListEntry(summary session.SessionInfo) string {
	title := strings.TrimSpace(summary.Title)
	if title == "" {
		title = "(untitled)"
	}
	updated := summary.UpdatedAt.Local()
	if updated.IsZero() {
		updated = time.Now()
	}
	return fmt.Sprintf("%s  %s  (%d messages)", updated.Format("2006-01-02 15:04"), title, summary.MessageCount)
}

func init() {
	rootCmd.AddCommand(sessionsCmd)
}
