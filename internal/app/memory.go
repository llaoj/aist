package app

import (
	"context"
	"strings"

	"github.com/fatih/color"
	"github.com/llaoj/aist/internal/i18n"
	"github.com/llaoj/aist/internal/memory"
	"github.com/llaoj/aist/internal/ui"
)

// memoryNotifier renders short save notifications.
type memoryNotifier struct {
	translator *i18n.I18n
}

func (n memoryNotifier) MemoryChanged(ctx context.Context, result memory.ChangeResult) {
	if err := ctx.Err(); err != nil {
		return
	}

	key := "memory.saved_updated"
	switch result.Action {
	case memory.ChangeActionReplaced:
		key = "memory.saved_replaced"
	case memory.ChangeActionCreated:
		key = "memory.saved_created"
	}

	message := n.translator.T(key, result.Entry.Name)
	if summary := strings.TrimSpace(result.Entry.Summary); summary != "" {
		message += " — " + summary
	}
	color.Green(message)
	ui.PrintRegionEnd()
}
