package interactive

import (
	"github.com/llaoj/aist/internal/fileedit"
	"github.com/llaoj/aist/internal/tools"
)

// noopRenderer is used by headless sessions and tests.
type noopRenderer struct{}

func (noopRenderer) RenderWelcome(string)                          {}
func (noopRenderer) RenderUserMessage(string)                      {}
func (noopRenderer) RenderAssistantMessage(string, string)         {}
func (noopRenderer) RenderShellCommand(string, tools.ShellCommand) {}
func (noopRenderer) RenderFileChange(*fileedit.Change)             {}
func (noopRenderer) RenderExecutionOutput(string)                  {}
func (noopRenderer) RenderExecutionOutputHeader(string)            {}
func (noopRenderer) RenderExecutionFinished(int, error)            {}
func (noopRenderer) RenderNotice(NoticeLevel, string)              {}
func (noopRenderer) RenderInterruptedNotice()                      {}
func (noopRenderer) RenderBlacklistNotice(string)                  {}
func (noopRenderer) RenderTurnComplete()                           {}
func (noopRenderer) RenderHistoryHeader(string)                    {}
func (noopRenderer) RenderToolSummary(string)                      {}
func (noopRenderer) Sync()                                         {}
