package interactive

import (
	"context"
	"strings"
	"testing"

	"github.com/llaoj/aist/internal/fileedit"
	"github.com/llaoj/aist/internal/i18n"
	"github.com/llaoj/aist/internal/session"
	"github.com/llaoj/aist/internal/tools"
)

type historyRecorder struct {
	header     string
	users      []string
	assistants []string
	tools      []string
	notices    []string
}

func (r *historyRecorder) RenderWelcome(string)          {}
func (r *historyRecorder) RenderUserMessage(text string) { r.users = append(r.users, text) }
func (r *historyRecorder) RenderAssistantMessage(_ string, content string) {
	r.assistants = append(r.assistants, content)
}
func (r *historyRecorder) RenderShellCommand(string, tools.ShellCommand) {}
func (r *historyRecorder) RenderFileChange(*fileedit.Change)             {}
func (r *historyRecorder) RenderExecutionOutput(string)                  {}
func (r *historyRecorder) RenderExecutionOutputHeader(string)            {}
func (r *historyRecorder) RenderExecutionFinished(int, error)            {}
func (r *historyRecorder) RenderNotice(_ NoticeLevel, message string) {
	r.notices = append(r.notices, message)
}
func (r *historyRecorder) RenderInterruptedNotice()        {}
func (r *historyRecorder) RenderBlacklistNotice(string)    {}
func (r *historyRecorder) RenderTurnComplete()             {}
func (r *historyRecorder) RenderHistoryHeader(text string) { r.header = text }
func (r *historyRecorder) RenderToolSummary(text string)   { r.tools = append(r.tools, text) }
func (r *historyRecorder) Sync()                           {}

func newReplayService(t *testing.T) *session.Manager {
	t.Helper()
	service := session.NewManager(session.ManagerOptions{
		Settings: session.Settings{CompactionEnabled: false},
	})
	ctx := context.Background()
	if _, err := service.Create(ctx, "TOP SECRET SYSTEM PROMPT"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := service.AppendUser(ctx, "check web1 disk"); err != nil {
		t.Fatalf("AppendUser: %v", err)
	}
	call := session.ToolCall{
		ID:   "call-1",
		Type: "function",
		Function: session.FunctionCall{
			Name:      tools.ToolShellExecute,
			Arguments: `{"command":"df -h","target":"web1"}`,
		},
	}
	if _, err := service.AppendAssistant(ctx, "", []session.ToolCall{call}, nil, "p/m"); err != nil {
		t.Fatalf("AppendAssistant: %v", err)
	}
	if _, err := service.AppendToolResult(ctx, "call-1", "FULL TOOL OUTPUT THAT MUST NOT REPLAY", session.ToolOutcomeSuccess); err != nil {
		t.Fatalf("AppendToolResult: %v", err)
	}
	if _, err := service.AppendUser(ctx, "what did you find?"); err != nil {
		t.Fatalf("AppendUser: %v", err)
	}
	if _, err := service.AppendAssistant(ctx, "Disk usage is normal.", nil, nil, "p/m"); err != nil {
		t.Fatalf("AppendAssistant: %v", err)
	}

	oldCall := session.ToolCall{
		ID:   "call-old",
		Type: "function",
		Function: session.FunctionCall{
			Name:      tools.ToolFileEdit,
			Arguments: `{"path":"/etc/nginx/nginx.conf","old_content":"a","new_content":"b","reason":"test"}`,
		},
	}
	if _, err := service.AppendAssistant(ctx, "", []session.ToolCall{oldCall}, nil, "p/m"); err != nil {
		t.Fatalf("AppendAssistant old: %v", err)
	}
	if _, err := service.AppendToolResult(ctx, "call-old", "legacy output", ""); err != nil {
		t.Fatalf("AppendToolResult old: %v", err)
	}
	return service
}

func TestReplayHistoryUsesToolOwnedLocalizedText(t *testing.T) {
	service := newReplayService(t)
	recorder := &historyRecorder{}
	interactive := NewSession(SessionOptions{
		SessionManager: service,
		Translator:     i18n.New("zh"),
		Renderer:       recorder,
		Mode:           ModeInteractive,
		PromptSet:      staticPrompts{},
	})
	interactive.ReplayHistory()

	if recorder.header != "历史消息" {
		t.Fatalf("header = %q", recorder.header)
	}
	if len(recorder.users) != 2 || recorder.users[0] != "check web1 disk" {
		t.Fatalf("users = %+v", recorder.users)
	}
	foundReply := false
	for _, assistant := range recorder.assistants {
		if assistant == "Disk usage is normal." {
			foundReply = true
		}
	}
	if !foundReply {
		t.Fatalf("assistants = %+v", recorder.assistants)
	}
	if len(recorder.tools) != 2 {
		t.Fatalf("tools = %+v", recorder.tools)
	}
	if recorder.tools[0] != "[web1] $ df -h · 成功" {
		t.Fatalf("new tool line = %q", recorder.tools[0])
	}
	if recorder.tools[1] != "编辑文件 /etc/nginx/nginx.conf" {
		t.Fatalf("old tool line = %q", recorder.tools[1])
	}
	for _, text := range append(append([]string{}, recorder.users...), recorder.assistants...) {
		if strings.Contains(text, "TOP SECRET SYSTEM PROMPT") || strings.Contains(text, "FULL TOOL OUTPUT") {
			t.Fatalf("hidden content leaked: %q", text)
		}
	}
	for _, text := range recorder.tools {
		if strings.Contains(text, "FULL TOOL OUTPUT") {
			t.Fatalf("tool content leaked: %q", text)
		}
	}
}

func TestReplayHistoryFollowsCurrentLanguage(t *testing.T) {
	service := newReplayService(t)
	recorder := &historyRecorder{}
	interactive := NewSession(SessionOptions{
		SessionManager: service,
		Translator:     i18n.New("en"),
		Renderer:       recorder,
		Mode:           ModeInteractive,
		PromptSet:      staticPrompts{},
	})
	interactive.ReplayHistory()

	if recorder.header != "History messages" {
		t.Fatalf("header = %q", recorder.header)
	}
	if recorder.tools[0] != "[web1] $ df -h · success" {
		t.Fatalf("tool line = %q", recorder.tools[0])
	}
}
