package tools

import "github.com/llaoj/aist/internal/session"

// Translator is the minimal i18n surface needed to render tool text.
type Translator interface {
	T(key string, args ...any) string
}

// ToolOutcome is the persisted, language-neutral tool outcome.
type ToolOutcome = session.ToolOutcome

const (
	ToolOutcomeSuccess  = session.ToolOutcomeSuccess
	ToolOutcomeFailed   = session.ToolOutcomeFailed
	ToolOutcomeSkipped  = session.ToolOutcomeSkipped
	ToolOutcomeRejected = session.ToolOutcomeRejected
)

// ToolDisplay renders one tool call for humans.
//
// Implementations must use the supplied Translator so tool names, invocation
// text and result status follow the active language. Nothing here is stored as
// localized text: the session persists only the structured status.
type ToolDisplay interface {
	ToolName() string
	FormatInvocation(t Translator) string
	FormatResult(outcome ToolOutcome, t Translator) string
}

// DecodeInvocation turns a stored tool call back into its display object.
func DecodeInvocation(toolName, arguments string) (ToolDisplay, error) {
	switch toolName {
	case ToolShellExecute:
		cmd, err := ParseShellCommand(RawToolCall{
			ToolName:      toolName,
			ArgumentsJSON: arguments,
		})
		if err != nil {
			return nil, err
		}
		return cmd, nil
	case ToolMemorySearch:
		args, err := ParseMemorySearchArgs(arguments)
		if err != nil {
			return nil, err
		}
		return args, nil
	case ToolMemorySave:
		args, err := ParseMemorySaveArgs(arguments)
		if err != nil {
			return nil, err
		}
		return args, nil
	case ToolFileEdit:
		args, err := ParseFileEditArgs(arguments)
		if err != nil {
			return nil, err
		}
		return args, nil
	default:
		return UnknownToolDisplay{Tool: toolName}, nil
	}
}

// UnknownToolDisplay renders a tool this binary does not recognize.
type UnknownToolDisplay struct {
	Tool string
}

// ToolName returns the raw stored tool name.
func (u UnknownToolDisplay) ToolName() string { return u.Tool }

// FormatInvocation describes an unknown tool without guessing its arguments.
func (u UnknownToolDisplay) FormatInvocation(t Translator) string {
	return t.T("tool.invoke.unknown", u.Tool)
}

// FormatResult adds the localized status to the invocation.
func (u UnknownToolDisplay) FormatResult(outcome ToolOutcome, t Translator) string {
	return formatResult(u.FormatInvocation(t), outcome, t)
}

func formatResult(invocation string, outcome ToolOutcome, t Translator) string {
	if outcome == "" {
		return invocation
	}
	return t.T("tool.result.line", invocation, t.T("tool.status."+string(outcome)))
}

// InvalidToolArgsError reports malformed arguments for a named tool.
type InvalidToolArgsError string

func (e InvalidToolArgsError) Error() string {
	return "invalid arguments for " + string(e)
}

func errInvalidToolArgs(tool string) error { return InvalidToolArgsError(tool) }
