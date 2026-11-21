package e2e

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/llaoj/aist/internal/executor"
	"github.com/llaoj/aist/internal/interactive"
	"github.com/llaoj/aist/internal/llm"
	"github.com/llaoj/aist/internal/tools"
)

type scriptedModel struct {
	responses []*llm.CompletionResponse
	requests  []llm.CompletionRequest
	index     int
}

func (m *scriptedModel) Name() string { return "scripted" }

func (m *scriptedModel) Call(_ context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	m.requests = append(m.requests, req)
	if m.index >= len(m.responses) {
		return &llm.CompletionResponse{Model: m.Name(), Content: "done"}, nil
	}
	response := m.responses[m.index]
	m.index++
	return response, nil
}

type staticPrompts struct{}

func (staticPrompts) Interactive() string  { return "interactive" }
func (staticPrompts) PipeAnalysis() string { return "pipe" }
func (staticPrompts) Compaction() string   { return "summary" }
func (staticPrompts) FollowUpLimit() string {
	return "<system-reminder>MAXIMUM STEPS REACHED</system-reminder>"
}

type interruptedInput struct{}

func (interruptedInput) ReadInput() (string, error) { return "", interactive.ErrInterrupted }

type fakeConfirmer struct {
	requests  []interactive.ConfirmationRequest
	decisions []interactive.Decision
}

func (f *fakeConfirmer) ConfirmCommand(_ context.Context, req interactive.ConfirmationRequest) (interactive.Decision, error) {
	f.requests = append(f.requests, req)
	if len(f.decisions) == 0 {
		return interactive.DecisionApprove, nil
	}
	decision := f.decisions[0]
	f.decisions = f.decisions[1:]
	return decision, nil
}

type runnerAdapter struct {
	runner executor.CommandRunner
}

func (a runnerAdapter) Execute(ctx context.Context, req interactive.ShellCommandRequest) (interactive.ShellCommandResult, error) {
	result, err := a.runner.Execute(ctx, executor.CommandRequest{Target: req.Target, Command: req.Command})
	return interactive.ShellCommandResult{
		Target:         result.Target,
		Command:        result.Command,
		Output:         result.Output,
		OutputStreamed: result.OutputStreamed,
		ExitCode:       result.ExitCode,
		Duration:       result.Duration,
	}, err
}

type harness struct {
	session *interactive.Session
	model   *scriptedModel
	confirm *fakeConfirmer
}

func newHarness(t *testing.T, sshBinary string, responses []*llm.CompletionResponse, decisions []interactive.Decision) *harness {
	t.Helper()
	model := &scriptedModel{responses: responses}
	confirmer := &fakeConfirmer{decisions: decisions}
	session := interactive.NewSession(interactive.SessionOptions{
		LLM:              model,
		Mode:             interactive.ModeInteractive,
		PromptSet:        staticPrompts{},
		InputReader:      interruptedInput{},
		CommandConfirmer: confirmer,
		ShellCommandExecutor: runnerAdapter{runner: executor.NewCommandRouter(
			executor.NewLocal(),
			executor.NewSSH(executor.SSHOptions{Binary: sshBinary, DisablePTY: true, Timeout: 5 * time.Second}),
		)},
	})
	return &harness{session: session, model: model, confirm: confirmer}
}

func (h *harness) run(t *testing.T) {
	t.Helper()
	if err := h.session.Run("test question"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func newFakeSSH(t *testing.T, marker string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ssh")
	script := `#!/bin/sh
while [ $# -gt 2 ]; do
  case "$1" in
    -o) shift 2 ;;
    -*) shift ;;
    *) break ;;
  esac
done
target="$1"
command="$2"
if [ -n "$AIST_FAKE_SSH_MARKER" ]; then
  printf '%s\n' "$target" >> "$AIST_FAKE_SSH_MARKER"
fi
printf 'target=%s\ncommand=%s\n' "$target" "$command"
case "$target" in
  down)
    echo "ssh: connect to host $target port 22: Connection refused" >&2
    exit 255
    ;;
  fail)
    echo "boom" >&2
    exit 3
    ;;
  *)
    echo "remote-ok"
    ;;
esac
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("AIST_FAKE_SSH_MARKER", marker)
	return path
}

func shellCall(id string, step int, target, command, commandType string) llm.ToolCall {
	return llm.ToolCall{
		ID:   id,
		Type: "function",
		Function: llm.FunctionCall{
			Name: tools.ToolShellExecute,
			Arguments: `{"step_index":` + itoa(step) + `,"reason":"check",` +
				`"command":"` + command + `","command_type":"` + commandType + `","target":"` + target + `"}`,
		},
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

func latestToolResult(t *testing.T, model *scriptedModel, callID string) string {
	t.Helper()
	if len(model.requests) == 0 {
		t.Fatal("model was not called")
	}
	messages := model.requests[len(model.requests)-1].Messages
	for _, message := range messages {
		if message.Role == "tool" && message.ToolCallID == callID {
			return llm.DerefPtr(message.Content)
		}
	}
	t.Fatalf("tool result %q not found", callID)
	return ""
}

func TestM5TargetRouting(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "calls")
	ssh := newFakeSSH(t, marker)
	h := newHarness(t, ssh, []*llm.CompletionResponse{
		{ToolCalls: []llm.ToolCall{shellCall("t1", 1, "web1", "df -h", "query")}},
		{Content: "done"},
	}, nil)
	h.run(t)

	result := latestToolResult(t, h.model, "t1")
	for _, want := range []string{"Target: web1", "target=web1", "remote-ok"} {
		if !strings.Contains(result, want) {
			t.Fatalf("tool result missing %q: %s", want, result)
		}
	}
	if len(h.confirm.requests) != 1 || len(h.confirm.requests[0].Targets) != 1 || h.confirm.requests[0].Targets[0] != "web1" {
		t.Fatalf("confirmations = %+v", h.confirm.requests)
	}
	calls, err := os.ReadFile(marker)
	if err != nil || !strings.Contains(string(calls), "web1") {
		t.Fatalf("fake ssh calls = %q err=%v", calls, err)
	}
}

func TestM5UnreachableTargetFailsAtExecution(t *testing.T) {
	ssh := newFakeSSH(t, filepath.Join(t.TempDir(), "calls"))
	h := newHarness(t, ssh, []*llm.CompletionResponse{
		{ToolCalls: []llm.ToolCall{shellCall("t2", 1, "down", "true", "query")}},
		{Content: "done"},
	}, nil)
	h.run(t)

	result := latestToolResult(t, h.model, "t2")
	for _, want := range []string{"Status: failed", "Error: ssh target unreachable", "SSH access is not available", "ssh down"} {
		if !strings.Contains(result, want) {
			t.Fatalf("tool result missing %q: %s", want, result)
		}
	}
}

func TestM5InvalidTargetIsNotExecuted(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "calls")
	ssh := newFakeSSH(t, marker)
	h := newHarness(t, ssh, []*llm.CompletionResponse{
		{ToolCalls: []llm.ToolCall{shellCall("t3", 1, "-oProxyCommand=evil", "df -h", "query")}},
		{Content: "done"},
	}, nil)
	h.run(t)

	result := latestToolResult(t, h.model, "t3")
	if !strings.Contains(result, "Invalid target") {
		t.Fatalf("tool result = %s", result)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid target reached executor, marker err=%v", err)
	}
	if len(h.confirm.requests) != 0 {
		t.Fatalf("invalid target was confirmed: %+v", h.confirm.requests)
	}
}

func TestM5QueryAndModifyConfirmations(t *testing.T) {
	ssh := newFakeSSH(t, filepath.Join(t.TempDir(), "calls"))
	h := newHarness(t, ssh, []*llm.CompletionResponse{
		{ToolCalls: []llm.ToolCall{
			shellCall("q1", 1, "web1", "df -h", "query"),
			shellCall("q2", 2, "web2", "free -m", "query"),
			shellCall("m1", 3, "web1", "systemctl restart nginx", "modify"),
		}},
		{Content: "done"},
	}, nil)
	h.run(t)

	if len(h.confirm.requests) != 3 {
		t.Fatalf("confirmations = %+v", h.confirm.requests)
	}
	want := []struct {
		cmdType tools.CommandType
		target  string
	}{
		{cmdType: tools.CommandTypeQuery, target: "web1"},
		{cmdType: tools.CommandTypeQuery, target: "web2"},
		{cmdType: tools.CommandTypeModify, target: "web1"},
	}
	for i, item := range want {
		req := h.confirm.requests[i]
		if req.Type != item.cmdType || req.UniqueTargets != 1 || len(req.Targets) != 1 || req.Targets[0] != item.target {
			t.Fatalf("confirmation %d = %+v", i, req)
		}
	}
	for _, id := range []string{"q1", "q2", "m1"} {
		if result := latestToolResult(t, h.model, id); !strings.Contains(result, "Target: ") {
			t.Fatalf("result %s missing target: %s", id, result)
		}
	}
}
