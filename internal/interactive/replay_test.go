package interactive

import (
	"context"
	"strings"
	"testing"

	"github.com/llaoj/aist/internal/commandguard"
	"github.com/llaoj/aist/internal/fileedit"
	"github.com/llaoj/aist/internal/i18n"
	"github.com/llaoj/aist/internal/session"
	"github.com/llaoj/aist/internal/tools"
)

type replayedShellCall struct {
	stepLabel string
	cmd       tools.ShellCommand
}

type replayedFinish struct {
	exitCode int
	errText  string
}

type historyRecorder struct {
	header          string
	users           []string
	assistants      []string
	shells          []replayedShellCall
	outputHeaders   []string
	outputs         []string
	finishes        []replayedFinish
	fileChanges     []*fileedit.Change
	tools           []string
	notices         []string
	blacklists      []string
	turnCompletions int
}

func (r *historyRecorder) RenderWelcome(string)          {}
func (r *historyRecorder) RenderUserMessage(text string) { r.users = append(r.users, text) }
func (r *historyRecorder) RenderAssistantMessage(content string) {
	if content != "" {
		r.assistants = append(r.assistants, content)
	}
}
func (r *historyRecorder) RenderShellCommand(stepLabel string, cmd tools.ShellCommand) {
	r.shells = append(r.shells, replayedShellCall{stepLabel: stepLabel, cmd: cmd})
}
func (r *historyRecorder) RenderFileChange(change *fileedit.Change) {
	r.fileChanges = append(r.fileChanges, change)
}
func (r *historyRecorder) RenderExecutionOutput(text string) {
	r.outputs = append(r.outputs, text)
}
func (r *historyRecorder) RenderExecutionOutputHeader(target string) {
	r.outputHeaders = append(r.outputHeaders, target)
}
func (r *historyRecorder) RenderExecutionFinished(exitCode int, err error) {
	errText := ""
	if err != nil {
		errText = err.Error()
	}
	r.finishes = append(r.finishes, replayedFinish{exitCode: exitCode, errText: errText})
}
func (r *historyRecorder) RenderNotice(_ NoticeLevel, message string) {
	r.notices = append(r.notices, message)
}
func (r *historyRecorder) RenderInterruptedNotice() {}
func (r *historyRecorder) RenderBlacklistNotice(pattern string) {
	r.blacklists = append(r.blacklists, pattern)
}
func (r *historyRecorder) RenderTurnComplete()             { r.turnCompletions++ }
func (r *historyRecorder) RenderHistoryHeader(text string) { r.header = text }
func (r *historyRecorder) RenderToolSummary(text string)   { r.tools = append(r.tools, text) }
func (r *historyRecorder) Sync()                           {}

func newReplayManager(t *testing.T) *session.Manager {
	t.Helper()
	manager := session.NewManager(session.ManagerOptions{
		Settings: session.Settings{CompactionEnabled: false},
	})
	ctx := context.Background()
	if _, err := manager.Create(ctx); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := manager.AppendUser(ctx, "check web1 disk and update nginx"); err != nil {
		t.Fatalf("AppendUser: %v", err)
	}

	shellCall := session.ToolCall{
		ID:   "call-shell",
		Type: "function",
		Function: session.FunctionCall{
			Name:      tools.ToolShellExecute,
			Arguments: `{"step_index":1,"reason":"check disk usage","command":"df -h","command_type":"query","target":"web1"}`,
		},
	}
	editCall := session.ToolCall{
		ID:   "call-edit",
		Type: "function",
		Function: session.FunctionCall{
			Name:      tools.ToolFileEdit,
			Arguments: `{"path":"/etc/nginx/nginx.conf","old_content":"worker_connections 768;","new_content":"worker_connections 4096;","reason":"raise connection limit","target":"web1","sudo":true}`,
		},
	}
	memSearchCall := session.ToolCall{
		ID:   "call-mem-search",
		Type: "function",
		Function: session.FunctionCall{
			Name:      tools.ToolMemorySearch,
			Arguments: `{"query":"web1"}`,
		},
	}
	memSaveCall := session.ToolCall{
		ID:   "call-mem-save",
		Type: "function",
		Function: session.FunctionCall{
			Name:      tools.ToolMemorySave,
			Arguments: `{"name":"prod-web","content":"- web1: nginx","summary":"prod nginx"}`,
		},
	}
	if _, err := manager.AppendAssistant(ctx, "", []session.ToolCall{shellCall, editCall, memSearchCall, memSaveCall}, nil, "p/m"); err != nil {
		t.Fatalf("AppendAssistant tools: %v", err)
	}

	shellResult := renderToolResult(
		tools.ToolOutcomeSuccess,
		[]toolResultField{
			resultField("Target", "web1"),
			resultField("Exit Code", "0"),
			resultField("Command", "df -h"),
		},
		"Filesystem      Size  Used Avail Use%\n/dev/vda1        40G   12G   26G  32%",
	)
	if _, err := manager.AppendToolResult(ctx, "call-shell", shellResult, session.ToolOutcomeSuccess); err != nil {
		t.Fatalf("AppendToolResult shell: %v", err)
	}

	editResult := renderToolResult(
		tools.ToolOutcomeSuccess,
		[]toolResultField{
			resultField("Target", "web1"),
			resultField("Path", "/etc/nginx/nginx.conf"),
			resultField("Sudo", "true"),
			resultField("Detail", "File updated successfully after user confirmation."),
		},
		"",
	)
	if _, err := manager.AppendToolResult(ctx, "call-edit", editResult, session.ToolOutcomeSuccess); err != nil {
		t.Fatalf("AppendToolResult edit: %v", err)
	}
	if _, err := manager.AppendToolResult(ctx, "call-mem-search", "INTERNAL MEMORY CONTENT", session.ToolOutcomeSuccess); err != nil {
		t.Fatalf("AppendToolResult mem search: %v", err)
	}
	if _, err := manager.AppendToolResult(ctx, "call-mem-save", `Memory "prod-web" created successfully (operation=append). Summary: prod nginx`, session.ToolOutcomeSuccess); err != nil {
		t.Fatalf("AppendToolResult mem save: %v", err)
	}
	if _, err := manager.AppendAssistant(ctx, "Disk usage is 32% and nginx limit is updated.", nil, nil, "p/m"); err != nil {
		t.Fatalf("AppendAssistant final: %v", err)
	}

	legacyEdit := session.ToolCall{
		ID:   "call-legacy-edit",
		Type: "function",
		Function: session.FunctionCall{
			Name:      tools.ToolFileEdit,
			Arguments: `{"path":"/etc/hosts","old_content":"127.0.0.1 localhost","new_content":"127.0.0.1 localhost web1","reason":"add host"}`,
		},
	}
	if _, err := manager.AppendUser(ctx, "update hosts"); err != nil {
		t.Fatalf("AppendUser 2: %v", err)
	}
	if _, err := manager.AppendAssistant(ctx, "", []session.ToolCall{legacyEdit}, nil, "p/m"); err != nil {
		t.Fatalf("AppendAssistant legacy: %v", err)
	}
	if _, err := manager.AppendToolResult(ctx, "call-legacy-edit", "Status: success\nTarget: local\nPath: /etc/hosts", session.ToolOutcomeSuccess); err != nil {
		t.Fatalf("AppendToolResult legacy: %v", err)
	}
	return manager
}

func TestReplayHistoryReproducesLiveTerminalView(t *testing.T) {
	manager := newReplayManager(t)
	recorder := &historyRecorder{}
	sess := NewSession(SessionOptions{
		SessionManager: manager,
		Translator:     i18n.New("zh"),
		Renderer:       recorder,
		CommandGuard:   commandguard.New(nil),
		Mode:           ModeInteractive,
		PromptSet:      staticPrompts{},
	})
	sess.ReplayHistory()

	if recorder.header != "历史消息" {
		t.Fatalf("header = %q, want 历史消息", recorder.header)
	}
	if len(recorder.users) != 2 || recorder.users[0] != "check web1 disk and update nginx" {
		t.Fatalf("users = %+v", recorder.users)
	}
	if len(recorder.shells) != 1 {
		t.Fatalf("shells = %+v, want 1", recorder.shells)
	}
	if recorder.shells[0].stepLabel != "1/1" || recorder.shells[0].cmd.Target != "web1" || recorder.shells[0].cmd.CommandLine != "df -h" {
		t.Fatalf("shell[0] = %+v", recorder.shells[0])
	}
	if len(recorder.outputs) != 1 || !strings.Contains(recorder.outputs[0], "/dev/vda1        40G") {
		t.Fatalf("outputs = %+v, want full df -h output", recorder.outputs)
	}
	if len(recorder.finishes) != 1 || recorder.finishes[0].exitCode != 0 || recorder.finishes[0].errText != "" {
		t.Fatalf("finishes = %+v", recorder.finishes)
	}
	if len(recorder.fileChanges) != 2 {
		t.Fatalf("fileChanges = %d, want 2", len(recorder.fileChanges))
	}
	if !strings.Contains(recorder.fileChanges[0].Diff, "-worker_connections 768;\n+worker_connections 4096;") {
		t.Fatalf("expected preview diff on first file edit, got:\n%s", recorder.fileChanges[0].Diff)
	}
	if !strings.Contains(recorder.fileChanges[1].Diff, "-127.0.0.1 localhost\n+127.0.0.1 localhost web1") {
		t.Fatalf("expected reconstructed preview diff on legacy file edit, got:\n%s", recorder.fileChanges[1].Diff)
	}
	if recorder.turnCompletions != 1 {
		t.Fatalf("turnCompletions = %d, want 1", recorder.turnCompletions)
	}

	joinedNotices := strings.Join(recorder.notices, "\n")
	if !strings.Contains(joinedNotices, "✓ Updated file: [web1] /etc/nginx/nginx.conf (sudo)") {
		t.Fatalf("missing file update notice in %+v", recorder.notices)
	}
	if !strings.Contains(joinedNotices, "✓ 已记住: prod-web — prod nginx") {
		t.Fatalf("missing memory save notice in %+v", recorder.notices)
	}
	for _, text := range append(append([]string{}, recorder.users...), recorder.assistants...) {
		if strings.Contains(text, "TOP SECRET SYSTEM PROMPT") || strings.Contains(text, "INTERNAL MEMORY CONTENT") {
			t.Fatalf("internal content leaked: %q", text)
		}
	}
}

func TestReplayHistoryFollowsCurrentLanguageAndGuardNotices(t *testing.T) {
	manager := session.NewManager(session.ManagerOptions{
		Settings: session.Settings{CompactionEnabled: false},
	})
	ctx := context.Background()
	_, _ = manager.Create(ctx)
	_, _ = manager.AppendUser(ctx, buildPipedUserMessage("try blocked command", "line1\nline2\n"))
	blockedCall := session.ToolCall{
		ID:   "call-blocked",
		Type: "function",
		Function: session.FunctionCall{
			Name:      tools.ToolShellExecute,
			Arguments: `{"step_index":1,"reason":"remove files","command":"rm -rf /tmp/test","command_type":"modify"}`,
		},
	}
	_, _ = manager.AppendAssistant(ctx, "", []session.ToolCall{blockedCall}, nil, "p/m")
	blockedResult := renderToolResult(
		tools.ToolOutcomeSkipped,
		[]toolResultField{
			resultField("Target", "local"),
			resultField("Detail", "Command is blacklisted and cannot be executed. Blacklist pattern: rm *"),
			resultField("Command", "rm -rf /tmp/test"),
		},
		"",
	)
	_, _ = manager.AppendToolResult(ctx, "call-blocked", blockedResult, session.ToolOutcomeSkipped)

	recorder := &historyRecorder{}
	sess := NewSession(SessionOptions{
		SessionManager: manager,
		Translator:     i18n.New("en"),
		Renderer:       recorder,
		CommandGuard:   commandguard.New([]string{"rm *"}),
		Mode:           ModeInteractive,
		PromptSet:      staticPrompts{},
	})
	sess.ReplayHistory()

	if recorder.header != "History messages" {
		t.Fatalf("header = %q, want History messages", recorder.header)
	}
	if len(recorder.users) != 1 || recorder.users[0] != "try blocked command" {
		t.Fatalf("users = %+v, want question without raw <piped_input>", recorder.users)
	}
	if !strings.Contains(strings.Join(recorder.notices, "\n"), "Loaded piped input (2 lines)") {
		t.Fatalf("notices = %+v, want Loaded piped input (2 lines)", recorder.notices)
	}
	if len(recorder.shells) != 1 || recorder.shells[0].cmd.CommandLine != "rm -rf /tmp/test" {
		t.Fatalf("shells = %+v", recorder.shells)
	}
	if len(recorder.blacklists) != 1 || recorder.blacklists[0] != "rm *" {
		t.Fatalf("blacklists = %+v, want [rm *]", recorder.blacklists)
	}
}
