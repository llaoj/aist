package interactive

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/llaoj/aist/internal/fileedit"
	"github.com/llaoj/aist/internal/llm"
	sessionpkg "github.com/llaoj/aist/internal/session"
	"github.com/llaoj/aist/internal/target"
)

type fakeFileEditor struct {
	change     *fileedit.Change
	prepareErr error
	prepareReq fileedit.EditRequest
	applied    bool
}

func (f *fakeFileEditor) Prepare(_ context.Context, req fileedit.EditRequest) (*fileedit.Change, error) {
	f.prepareReq = req
	return f.change, f.prepareErr
}

func (f *fakeFileEditor) Apply(context.Context, *fileedit.Change) error {
	f.applied = true
	return nil
}

type fakeFilePrompter struct {
	confirm  bool
	decision Decision
	err      error
	calls    int
}

func (f *fakeFilePrompter) ConfirmFileEdit(context.Context, *fileedit.Change) (Decision, error) {
	f.calls++
	if f.decision != DecisionUnknown {
		return f.decision, f.err
	}
	if f.confirm {
		return DecisionApprove, f.err
	}
	return DecisionSkip, f.err
}

func newFileEditSession(editor FileEditor, prompter FileEditConfirmer) *Session {
	return NewSession(SessionOptions{
		FileEditor:        editor,
		FileEditConfirmer: prompter,
	})
}

func fileEditCall() llm.ToolCall {
	return llm.ToolCall{
		ID:   "edit-1",
		Type: "function",
		Function: llm.FunctionCall{
			Name: "file_edit",
			Arguments: `{
				"path":"/tmp/ssh_config",
				"old_content":"Host old\n",
				"new_content":"Host new\n",
				"reason":"rename host"
			}`,
		},
	}
}

func TestFileEditToolAppliesAfterConfirmation(t *testing.T) {
	editor := &fakeFileEditor{change: &fileedit.Change{
		Path:   "/tmp/ssh_config",
		Diff:   "--- /tmp/ssh_config\n+++ /tmp/ssh_config\n-Host old\n+Host new\n",
		Reason: "rename host",
	}}
	prompter := &fakeFilePrompter{confirm: true}
	session := newFileEditSession(editor, prompter)

	if _, err := session.handleFileEdit(context.Background(), fileEditCall()); err != nil {
		t.Fatalf("handleFileEdit: %v", err)
	}
	if editor.prepareReq.Path != "/tmp/ssh_config" || editor.prepareReq.OldContent != "Host old\n" {
		t.Fatalf("prepare request = %+v", editor.prepareReq)
	}
	if prompter.calls != 1 || !editor.applied {
		t.Fatalf("prompt calls=%d applied=%v", prompter.calls, editor.applied)
	}
	result := findToolResult(t, ToLLMMessages(session.sessionManager.DisplayMessages()), "edit-1")
	if !strings.Contains(result, "updated successfully") {
		t.Fatalf("result = %q", result)
	}
}

func TestFileEditToolSkippedDoesNotWrite(t *testing.T) {
	editor := &fakeFileEditor{change: &fileedit.Change{Path: "/tmp/ssh_config", Diff: "diff"}}
	prompter := &fakeFilePrompter{confirm: false}
	session := newFileEditSession(editor, prompter)

	if _, err := session.handleFileEdit(context.Background(), fileEditCall()); err != nil {
		t.Fatalf("handleFileEdit: %v", err)
	}
	if editor.applied {
		t.Fatal("skipped edit must not be applied")
	}
	result := findToolResult(t, ToLLMMessages(session.sessionManager.DisplayMessages()), "edit-1")
	if !strings.Contains(result, "No changes were written") {
		t.Fatalf("result = %q", result)
	}
	if status := lastToolStatus(t, session, "edit-1"); status != sessionpkg.ToolOutcomeSkipped {
		t.Fatalf("status = %q, want %q", status, sessionpkg.ToolOutcomeSkipped)
	}
}

func TestFileEditToolPrepareFailureDoesNotPrompt(t *testing.T) {
	editor := &fakeFileEditor{prepareErr: errors.New("old_content was not found")}
	prompter := &fakeFilePrompter{confirm: true}
	session := newFileEditSession(editor, prompter)

	if _, err := session.handleFileEdit(context.Background(), fileEditCall()); err != nil {
		t.Fatalf("handleFileEdit: %v", err)
	}
	if prompter.calls != 0 || editor.applied {
		t.Fatalf("unexpected prompt/apply: calls=%d applied=%v", prompter.calls, editor.applied)
	}
	result := findToolResult(t, ToLLMMessages(session.sessionManager.DisplayMessages()), "edit-1")
	if !strings.Contains(result, "No changes were written") {
		t.Fatalf("result = %q", result)
	}
}

func TestFileEditFailureStillRequestsFollowUp(t *testing.T) {
	editor := &fakeFileEditor{prepareErr: errors.New("old_content was not found")}
	prompter := &fakeFilePrompter{confirm: true}
	model := &scriptedModel{responses: []*llm.CompletionResponse{
		{ToolCalls: []llm.ToolCall{fileEditCall()}},
		{Content: "The edit could not be applied."},
	}}
	session := NewSession(SessionOptions{
		LLM:               model,
		Mode:              ModeInteractive,
		PromptSet:         staticPrompts{},
		FileEditor:        editor,
		FileEditConfirmer: prompter,
	})

	if err := session.handleUserTurn("edit the file"); err != nil {
		t.Fatalf("handleUserTurn: %v", err)
	}
	if model.index != 2 {
		t.Fatalf("model calls = %d, want 2 after failed file edit", model.index)
	}
}

func TestFileEditEndToEnd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ssh_config")
	if err := os.WriteFile(path, []byte("Host web1\n  HostName 10.0.0.1\n"), 0o640); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatalf("Chmod: %v", err)
	}

	model := &scriptedModel{responses: []*llm.CompletionResponse{
		{ToolCalls: []llm.ToolCall{{
			ID:   "edit-e2e",
			Type: "function",
			Function: llm.FunctionCall{
				Name:      "file_edit",
				Arguments: `{"path":"` + path + `","old_content":"  HostName 10.0.0.1\n","new_content":"  HostName 10.0.0.2\n","reason":"move host"}`,
			},
		}}},
		{Content: "Updated the host."},
	}}
	session := NewSession(SessionOptions{
		LLM:               model,
		Mode:              ModeInteractive,
		PromptSet:         staticPrompts{},
		FileEditor:        fileedit.NewEditor(fileedit.EditorOptions{HomeDir: dir}),
		FileEditConfirmer: &fakeFilePrompter{confirm: true},
	})

	if err := session.handleUserTurn("change web1 to 10.0.0.2"); err != nil {
		t.Fatalf("handleUserTurn: %v", err)
	}
	if model.index != 2 {
		t.Fatalf("model calls = %d, want 2 after successful file edit without text", model.index)
	}

	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(updated), "10.0.0.2") || strings.Contains(string(updated), "10.0.0.1") {
		t.Fatalf("content = %q", updated)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
	backups := 0
	_ = filepath.WalkDir(filepath.Join(dir, ".aist", "fileedit", "backups"), func(_ string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && strings.HasSuffix(entry.Name(), ".bak") {
			backups++
		}
		return nil
	})
	if backups != 1 {
		t.Fatalf("backups = %d", backups)
	}
	if result := findToolResult(t, ToLLMMessages(session.sessionManager.DisplayMessages()), "edit-e2e"); !strings.Contains(result, "updated successfully") {
		t.Fatalf("tool result = %q", result)
	}
}

func TestFileEditPropagatesRemoteTargetAndSudo(t *testing.T) {
	remote, err := target.New("web1")
	if err != nil {
		t.Fatalf("target.New: %v", err)
	}
	editor := &fakeFileEditor{change: &fileedit.Change{
		Target:  remote,
		Path:    "/etc/nginx/nginx.conf",
		Diff:    "--- /etc/nginx/nginx.conf\n+++ /etc/nginx/nginx.conf\n",
		Reason:  "tune workers",
		UseSudo: true,
	}}
	prompter := &fakeFilePrompter{confirm: true}
	session := newFileEditSession(editor, prompter)

	call := llm.ToolCall{
		ID:   "edit-remote-sudo",
		Type: "function",
		Function: llm.FunctionCall{
			Name: "file_edit",
			Arguments: `{
				"target":"web1",
				"path":"/etc/nginx/nginx.conf",
				"old_content":"worker_processes 1;\n",
				"new_content":"worker_processes auto;\n",
				"reason":"tune workers",
				"sudo":true
			}`,
		},
	}
	if _, err := session.handleFileEdit(context.Background(), call); err != nil {
		t.Fatalf("handleFileEdit: %v", err)
	}
	if editor.prepareReq.Target != "web1" || !editor.prepareReq.UseSudo {
		t.Fatalf("prepareReq = %+v", editor.prepareReq)
	}
	result := findToolResult(t, ToLLMMessages(session.sessionManager.DisplayMessages()), "edit-remote-sudo")
	for _, want := range []string{"Status: success", "Target: web1", "Path: /etc/nginx/nginx.conf", "Sudo: true"} {
		if !strings.Contains(result, want) {
			t.Fatalf("result = %q, want %q", result, want)
		}
	}
}

func TestFileEditRejectRemainingEndsTurnForUserClarification(t *testing.T) {
	editor := &fakeFileEditor{change: &fileedit.Change{Path: "/tmp/ssh_config", Diff: "diff"}}
	prompter := &fakeFilePrompter{decision: DecisionRejectRemaining}
	model := &strictValidatingModel{
		t: t,
		responses: []*llm.CompletionResponse{
			{ToolCalls: []llm.ToolCall{
				fileEditCall(),
				shellCall("after-edit", 1, "local", "echo should-not-run", "query"),
			}},
			{Content: "已按补充说明调整。"},
		},
	}
	executor := &fakeShellExecutor{}
	recorder := &turnRecorder{}
	session := NewSession(SessionOptions{
		LLM:                  model,
		Mode:                 ModeInteractive,
		PromptSet:            staticPrompts{},
		FileEditor:           editor,
		FileEditConfirmer:    prompter,
		CommandConfirmer:     &fakeConfirmer{},
		ShellCommandExecutor: executor,
		Renderer:             recorder,
	})

	if err := session.handleUserTurn("修改配置"); err != nil {
		t.Fatalf("turn 1: %v", err)
	}
	if !recorder.hasNotice("add more details") {
		t.Fatalf("reject hint not rendered: %v", recorder.notices)
	}
	if recorder.turnCompletions != 0 {
		t.Fatalf("turn completions = %d, want 0", recorder.turnCompletions)
	}
	if model.index != 1 || len(executor.requests) != 0 {
		t.Fatalf("model calls=%d execs=%d, want 1 and 0", model.index, len(executor.requests))
	}
	if err := session.handleUserTurn("改用 /etc/ssh/ssh_config"); err != nil {
		t.Fatalf("turn 2: %v", err)
	}
	if model.index != 2 {
		t.Fatalf("model calls = %d, want 2", model.index)
	}
}
