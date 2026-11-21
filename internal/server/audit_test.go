package server

import (
	"encoding/json"
	"strings"
	"testing"
)

type auditTestCall struct {
	id, name, args string
}

func assistantWithCalls(calls ...auditTestCall) map[string]any {
	toolCalls := make([]map[string]any, 0, len(calls))
	for _, call := range calls {
		toolCalls = append(toolCalls, map[string]any{
			"id":       call.id,
			"type":     "function",
			"function": map[string]any{"name": call.name, "arguments": call.args},
		})
	}
	return map[string]any{"role": "assistant", "tool_calls": toolCalls}
}

func toolMessage(callID, content string) map[string]any {
	return map[string]any{"role": "tool", "tool_call_id": callID, "content": content}
}

func chatBody(t *testing.T, messages ...map[string]any) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{"model": "auto", "messages": messages})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return body
}

func attrMap(t *testing.T, attrs []any) map[string]any {
	t.Helper()
	if len(attrs)%2 != 0 {
		t.Fatalf("odd attrs: %v", attrs)
	}
	out := make(map[string]any, len(attrs)/2)
	for i := 0; i < len(attrs); i += 2 {
		out[attrs[i].(string)] = attrs[i+1]
	}
	return out
}

var testActor = toolAuditActor{AuthID: "u1", Email: "dave@example.com", UserIP: "10.0.0.1", RequestID: "req1"}

const shellArgs = `{"step_index":1,"reason":"restart web","command":"systemctl restart nginx","command_type":"modify","target":"web1"}`

func TestBuildToolAuditLogsShellExecute(t *testing.T) {
	body := chatBody(t,
		map[string]any{"role": "user", "content": "restart nginx"},
		assistantWithCalls(auditTestCall{"c1", "shell_execute", shellArgs}),
		toolMessage("c1", "Status: success\nTarget: web1\nExit Code: 0\nCommand: systemctl restart nginx"),
	)

	logs, err := buildToolAuditLogs(body, testActor)
	if err != nil {
		t.Fatalf("buildToolAuditLogs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("logs = %+v", logs)
	}
	if want := "shell_execute @web1: systemctl restart nginx"; logs[0].Message != want {
		t.Fatalf("message = %q, want %q", logs[0].Message, want)
	}
	attrs := attrMap(t, logs[0].Attrs)
	want := map[string]any{
		"type":        "tool",
		"tool":        "shell_execute",
		"authId":      "u1",
		"email":       "dave@example.com",
		"userIP":      "10.0.0.1",
		"requestId":   "req1",
		"target":      "web1",
		"command":     "systemctl restart nginx",
		"commandType": "modify",
		"reason":      "restart web",
		"result":      "Status: success\nTarget: web1\nExit Code: 0\nCommand: systemctl restart nginx",
	}
	for key, value := range want {
		if attrs[key] != value {
			t.Fatalf("attr %s = %v, want %v", key, attrs[key], value)
		}
	}
}

func TestBuildToolAuditLogsFileEdit(t *testing.T) {
	args := `{"path":"/etc/hosts","old_content":"a","new_content":"b","reason":"fix host","sudo":true}`
	body := chatBody(t,
		assistantWithCalls(auditTestCall{"e1", "file_edit", args}),
		toolMessage("e1", "Status: success\nTarget: local\nPath: /etc/hosts"),
	)

	logs, err := buildToolAuditLogs(body, testActor)
	if err != nil || len(logs) != 1 {
		t.Fatalf("logs = %+v, err = %v", logs, err)
	}
	if want := "file_edit @local: /etc/hosts"; logs[0].Message != want {
		t.Fatalf("message = %q, want %q", logs[0].Message, want)
	}
	attrs := attrMap(t, logs[0].Attrs)
	if attrs["path"] != "/etc/hosts" || attrs["sudo"] != true || attrs["old"] != "a" || attrs["new"] != "b" || attrs["target"] != "local" {
		t.Fatalf("attrs = %+v", attrs)
	}
}

func TestBuildToolAuditLogsOnlyAuditsResultsAfterLastAssistant(t *testing.T) {
	body := chatBody(t,
		assistantWithCalls(auditTestCall{"old", "shell_execute", shellArgs}),
		toolMessage("old", "Status: success"),
		map[string]any{"role": "assistant", "content": "done"},
		map[string]any{"role": "user", "content": "again"},
		assistantWithCalls(
			auditTestCall{"new1", "shell_execute", shellArgs},
			auditTestCall{"mem", "memory_search", `{"query":"x"}`},
			auditTestCall{"new2", "shell_execute", shellArgs},
		),
		toolMessage("new1", "Status: success"),
		toolMessage("mem", "memory hit"),
		toolMessage("new2", "Status: rejected"),
		map[string]any{"role": "user", "content": "stop"},
	)

	logs, err := buildToolAuditLogs(body, testActor)
	if err != nil {
		t.Fatalf("buildToolAuditLogs: %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("logs = %+v, want 2 shell results", logs)
	}
	if got := attrMap(t, logs[1].Attrs)["result"]; got != "Status: rejected" {
		t.Fatalf("second result = %v", got)
	}
}

func TestBuildToolAuditLogsSkipsHistoryWithoutNewResults(t *testing.T) {
	body := chatBody(t,
		assistantWithCalls(auditTestCall{"c1", "shell_execute", shellArgs}),
		toolMessage("c1", "Status: success"),
		map[string]any{"role": "assistant", "content": "done"},
		map[string]any{"role": "user", "content": "thanks"},
	)
	logs, err := buildToolAuditLogs(body, testActor)
	if err != nil || len(logs) != 0 {
		t.Fatalf("logs = %+v, err = %v", logs, err)
	}
}

func TestBuildToolAuditLogsTruncatesLargeValues(t *testing.T) {
	result := "Status: success\nExit Code: 0\n\n[Output]\n" + strings.Repeat("x", auditFieldMaxBytes["result"]*2)
	body := chatBody(t,
		assistantWithCalls(auditTestCall{"c1", "shell_execute", shellArgs}),
		toolMessage("c1", result),
	)
	logs, err := buildToolAuditLogs(body, testActor)
	if err != nil || len(logs) != 1 {
		t.Fatalf("logs = %+v, err = %v", logs, err)
	}
	got := attrMap(t, logs[0].Attrs)["result"].(string)
	if !strings.HasPrefix(got, "Status: success\nExit Code: 0") || !strings.Contains(got, "…[truncated ") {
		t.Fatalf("result not prefix-truncated: %.80q", got)
	}
	if len(got) > auditFieldMaxBytes["result"]+40 {
		t.Fatalf("result length = %d", len(got))
	}
}

func TestBuildToolAuditLogsKeepsInvalidArguments(t *testing.T) {
	body := chatBody(t,
		assistantWithCalls(auditTestCall{"c1", "shell_execute", `{"command":`}),
		toolMessage("c1", "Status: failed\nError: invalid shell_execute arguments"),
	)
	logs, err := buildToolAuditLogs(body, testActor)
	if err != nil || len(logs) != 1 {
		t.Fatalf("logs = %+v, err = %v", logs, err)
	}
	attrs := attrMap(t, logs[0].Attrs)
	if logs[0].Message != "shell_execute: invalid arguments" || attrs["args"] != `{"command":` {
		t.Fatalf("log = %+v", logs[0])
	}
}

func TestBuildToolAuditLogsOmitsUserIPWhenDisabled(t *testing.T) {
	body := chatBody(t,
		assistantWithCalls(auditTestCall{"c1", "shell_execute", shellArgs}),
		toolMessage("c1", "Status: success"),
	)
	actor := testActor
	actor.UserIP = ""
	logs, err := buildToolAuditLogs(body, actor)
	if err != nil || len(logs) != 1 {
		t.Fatalf("logs = %+v, err = %v", logs, err)
	}
	if _, found := attrMap(t, logs[0].Attrs)["userIP"]; found {
		t.Fatalf("userIP should be omitted: %+v", logs[0].Attrs)
	}
}

func TestBuildToolAuditLogsKeepsLongCommandsUpToLimit(t *testing.T) {
	command := strings.Repeat("a", 3900) + "; rm -rf /data"
	args, _ := json.Marshal(map[string]any{"step_index": 1, "reason": "x", "command": command, "command_type": "modify"})
	body := chatBody(t,
		assistantWithCalls(auditTestCall{"c1", "shell_execute", string(args)}),
		toolMessage("c1", "Status: success"),
	)
	logs, err := buildToolAuditLogs(body, testActor)
	if err != nil || len(logs) != 1 {
		t.Fatalf("logs = %+v, err = %v", logs, err)
	}
	if got := attrMap(t, logs[0].Attrs)["command"]; got != command {
		t.Fatalf("command was truncated: %.40q", got)
	}
}

func TestBuildToolAuditLogsFitsEscapedDataUnderLimit(t *testing.T) {
	escaped := strings.Repeat("\x1b[31m<&>", 2000)
	command := strings.Repeat("\x1b", 4000)
	args, _ := json.Marshal(map[string]any{"step_index": 1, "reason": "x", "command": command, "command_type": "query"})
	body := chatBody(t,
		assistantWithCalls(auditTestCall{"c1", "shell_execute", string(args)}),
		toolMessage("c1", "Status: success\n\n[Output]\n"+escaped),
	)
	logs, err := buildToolAuditLogs(body, testActor)
	if err != nil || len(logs) != 1 {
		t.Fatalf("logs = %+v, err = %v", logs, err)
	}
	if size := auditDataSize(logs[0].Attrs); size > auditMaxDataBytes {
		t.Fatalf("data size = %d, want <= %d", size, auditMaxDataBytes)
	}
	attrs := attrMap(t, logs[0].Attrs)
	if attrs["type"] != "tool" || attrs["tool"] != "shell_execute" || attrs["userIP"] != "10.0.0.1" {
		t.Fatalf("identity fields lost: %+v", attrs)
	}
	if !strings.HasPrefix(attrs["result"].(string), "Status: success") {
		t.Fatalf("result lost its header: %.40q", attrs["result"])
	}
}

func TestBuildToolAuditLogsFileEditContentLimit(t *testing.T) {
	args, _ := json.Marshal(map[string]any{
		"path": "/etc/app.conf", "old_content": "", "reason": "create",
		"new_content": strings.Repeat("n", 5000),
	})
	body := chatBody(t,
		assistantWithCalls(auditTestCall{"e1", "file_edit", string(args)}),
		toolMessage("e1", "Status: success"),
	)
	logs, err := buildToolAuditLogs(body, testActor)
	if err != nil || len(logs) != 1 {
		t.Fatalf("logs = %+v, err = %v", logs, err)
	}
	got := attrMap(t, logs[0].Attrs)["new"].(string)
	if !strings.HasPrefix(got, strings.Repeat("n", 4000)+"…[truncated 1000 bytes]") {
		t.Fatalf("new = %.60q (len %d)", got[len(got)-40:], len(got))
	}
}
