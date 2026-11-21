package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/fatih/color"
	"github.com/llaoj/aist/internal/executor"
	"github.com/llaoj/aist/internal/i18n"
	"github.com/llaoj/aist/internal/interactive"
	"github.com/llaoj/aist/internal/memory"
	"github.com/llaoj/aist/internal/tools"
	"github.com/llaoj/aist/internal/ui"
)

func captureOutput(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	oldStdout := os.Stdout
	oldColorOutput := color.Output
	os.Stdout = w
	color.Output = w

	fn()

	_ = w.Close()
	os.Stdout = oldStdout
	color.Output = oldColorOutput

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("read captured output: %v", err)
	}
	_ = r.Close()
	return buf.String()
}

func TestTerminalRendererRegionsHaveSingleBlankLine(t *testing.T) {
	renderer := terminalRenderer{translator: i18n.New("en")}

	out := captureOutput(t, func() {
		renderer.RenderWelcome("dev")
		renderer.RenderUserMessage("hello")
		renderer.RenderAssistantMessage("", "line one\n\nline two\n\n\nline three")
		renderer.RenderShellCommand("1/1", tools.ShellCommand{
			Target:      "local",
			CommandLine: "uptime",
			Reason:      "check load",
			Type:        tools.CommandTypeQuery,
		})
		renderer.RenderExecutionOutputHeader("local")
		renderer.RenderExecutionOutput("output")
		renderer.RenderExecutionFinished(0, nil)
	})

	if strings.Contains(out, "\n\n\n") {
		t.Fatalf("regions produced more than one blank line:\n%q", out)
	}
	for _, want := range []string{
		"User:\nhello\n",
		"AI:\nline one\nline two\nline three\n",
		"Query command:\n1/1. check load\n[local] $ uptime\n",
		"Execution output:\noutput\n\n✓ Execution successful\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestMemoryNotifierEndsRegionInGreen(t *testing.T) {
	oldNoColor := color.NoColor
	color.NoColor = false
	defer func() { color.NoColor = oldNoColor }()

	notifier := memoryNotifier{translator: i18n.New("en")}
	out := captureOutput(t, func() {
		notifier.MemoryChanged(context.Background(), memory.ChangeResult{
			Entry:     memory.Entry{Name: "hosts", Summary: "aliases"},
			Operation: memory.ChangeOperationAppend,
		})
	})

	plain := ansi.Strip(out)
	if !strings.HasSuffix(plain, "\n\n") || strings.HasSuffix(plain, "\n\n\n") {
		t.Fatalf("memory notification must end with exactly one blank line: %q", out)
	}
	if !strings.Contains(plain, "✓ Updated Memory: hosts — aliases") {
		t.Fatalf("missing memory notification: %q", out)
	}
	if !strings.Contains(out, "\x1b[32m") {
		t.Fatalf("memory notification is not green: %q", out)
	}
}

func TestTerminalConfirmerOmitsCommandLineInPrompt(t *testing.T) {
	var prompts []string
	confirmer := terminalConfirmer{
		translator: i18n.New("zh"),
		promptFn: func(prompt string, _ *i18n.I18n) (ui.ConfirmDecision, error) {
			prompts = append(prompts, prompt)
			return ui.ConfirmApprove, nil
		},
	}

	decision, err := confirmer.ConfirmCommand(context.Background(), interactive.ConfirmationRequest{
		Type:       tools.CommandTypeModify,
		Targets:    []string{"local"},
		Invocation: "[local] $ ls -la ~/.atomcode",
	})
	if err != nil {
		t.Fatalf("ConfirmCommand: %v", err)
	}
	if decision != interactive.DecisionApprove {
		t.Fatalf("decision = %v, want %v", decision, interactive.DecisionApprove)
	}
	if len(prompts) != 2 {
		t.Fatalf("prompts = %q, want 2 prompts", prompts)
	}
	if prompts[0] != "是否执行此命令?" {
		t.Fatalf("first prompt = %q, want %q", prompts[0], "是否执行此命令?")
	}
	if prompts[1] != "警告: 该命令将修改服务器状态，是否确定执行?" {
		t.Fatalf("second prompt = %q, want %q", prompts[1], "警告: 该命令将修改服务器状态，是否确定执行?")
	}
}

func TestTerminalRendererExecutionFinishedLocalized(t *testing.T) {
	oldNoColor := color.NoColor
	color.NoColor = false
	defer func() { color.NoColor = oldNoColor }()

	cases := []struct {
		lang      string
		exitCode  int
		err       error
		wantText  string
		wantColor string
	}{
		{lang: "zh", exitCode: 0, wantText: "✓ 执行成功", wantColor: "\x1b[32m"},
		{lang: "zh", exitCode: 1, wantText: "✓ 执行成功 (退出码: 1)", wantColor: "\x1b[32m"},
		{lang: "zh", exitCode: 124, err: &executor.TimeoutError{Timeout: 30 * time.Minute}, wantText: "✗ 执行失败: 命令执行超时 (30m0s)，已终止执行", wantColor: "\x1b[31m"},
		{lang: "zh", exitCode: 137, err: &executor.StuckExitError{Grace: 5 * time.Second}, wantText: "✗ 执行失败: 命令输出结束后 5s 内未退出，已终止执行", wantColor: "\x1b[31m"},
		{lang: "zh", exitCode: 255, err: &executor.SSHUnreachableError{Host: "198.51.100.48"}, wantText: "✗ 执行失败: SSH 连接失败，请使用 `ssh 198.51.100.48` 检查连接配置", wantColor: "\x1b[31m"},
		{lang: "zh", exitCode: 0, err: context.Canceled, wantText: "✗ 执行失败: 命令已取消", wantColor: "\x1b[31m"},
		{lang: "en", exitCode: 0, wantText: "✓ Execution successful", wantColor: "\x1b[32m"},
		{lang: "en", exitCode: 1, wantText: "✓ Execution successful (exit code: 1)", wantColor: "\x1b[32m"},
		{lang: "en", exitCode: 124, err: &executor.TimeoutError{Timeout: 30 * time.Minute}, wantText: "✗ Execution failed: command timed out after 30m0s and was terminated", wantColor: "\x1b[31m"},
		{lang: "en", exitCode: 137, err: &executor.StuckExitError{Grace: 5 * time.Second}, wantText: "✗ Execution failed: command did not exit within 5s after its output ended and was terminated", wantColor: "\x1b[31m"},
		{lang: "en", exitCode: 255, err: &executor.SSHUnreachableError{Host: "198.51.100.48"}, wantText: "✗ Execution failed: SSH connection failed; verify access with `ssh 198.51.100.48`", wantColor: "\x1b[31m"},
		{lang: "en", exitCode: 0, err: errors.New("fork/exec failed"), wantText: "✗ Execution failed: fork/exec failed", wantColor: "\x1b[31m"},
	}

	for _, tc := range cases {
		renderer := terminalRenderer{translator: i18n.New(tc.lang)}
		out := captureOutput(t, func() {
			renderer.RenderExecutionFinished(tc.exitCode, tc.err)
		})
		plain := ansi.Strip(out)
		if strings.TrimSpace(plain) != tc.wantText {
			t.Fatalf("lang=%s code=%d err=%v: got %q, want %q", tc.lang, tc.exitCode, tc.err, strings.TrimSpace(plain), tc.wantText)
		}
		if !strings.Contains(out, tc.wantColor) {
			t.Fatalf("lang=%s code=%d err=%v: missing ANSI color %q in %q", tc.lang, tc.exitCode, tc.err, tc.wantColor, out)
		}
	}
}
