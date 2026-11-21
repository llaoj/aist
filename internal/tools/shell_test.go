package tools

import (
	"strings"
	"testing"
)

func TestParseShellCommandReadsTarget(t *testing.T) {
	cmd, err := ParseShellCommand(RawToolCall{
		ID:       "call-1",
		ToolName: ToolShellExecute,
		ArgumentsJSON: `{
			"step_index": 1,
			"reason": "check disk",
			"command": "df -h",
			"command_type": "query",
			"target": " web1 "
		}`,
	})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if cmd.Target != "web1" || cmd.CommandLine != "df -h" || cmd.Type != CommandTypeQuery {
		t.Fatalf("command = %+v", cmd)
	}
}

func TestShellExecuteDefinitionIncludesTarget(t *testing.T) {
	def := ShellExecuteDefinition()
	schema := string(def.InputSchema)
	if !strings.Contains(schema, `"target"`) || !strings.Contains(schema, "Never invent a target") {
		t.Fatalf("schema = %s", schema)
	}
	if !strings.Contains(def.Description, "## Targets") {
		t.Fatalf("description missing target rules: %s", def.Description)
	}
	if !strings.Contains(def.Description, "kubectl get nodes") {
		t.Fatalf("description missing discovered-target rule: %s", def.Description)
	}
	if !strings.Contains(def.Description, "Interactive PTY execution") || !strings.Contains(def.Description, "sudo") {
		t.Fatalf("description missing interactive/sudo support: %s", def.Description)
	}
	if !strings.Contains(def.Description, "Full-screen TUI programs are prohibited") || !strings.Contains(def.Description, "sudo=true") {
		t.Fatalf("description missing TUI block / file_edit sudo guidance: %s", def.Description)
	}
	if strings.Contains(def.Description, "sudo vim") {
		t.Fatalf("description should not suggest vim for file edits: %s", def.Description)
	}
}

func TestParseShellCommandUsesUnknownForMissingType(t *testing.T) {
	cmd, err := ParseShellCommand(RawToolCall{
		ID:            "call-2",
		ToolName:      ToolShellExecute,
		ArgumentsJSON: `{"step_index":1,"reason":"check","command":"uptime"}`,
	})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if cmd.Type != CommandTypeUnknown {
		t.Fatalf("type = %v, want CommandTypeUnknown", cmd.Type)
	}
}

func TestParseShellCommandRejectsInvalidType(t *testing.T) {
	_, err := ParseShellCommand(RawToolCall{
		ID:            "call-3",
		ToolName:      ToolShellExecute,
		ArgumentsJSON: `{"step_index":1,"reason":"check","command":"uptime","command_type":"bogus"}`,
	})
	if err == nil {
		t.Fatal("expected invalid command_type error")
	}
}
