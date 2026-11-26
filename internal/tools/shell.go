package tools

import (
	"encoding/json"
	"strings"
)

// shellExecuteToolDef is the canonical description of the shell_execute tool.
// Callers convert ShellExecuteDefinition into their wire-specific format.
var shellExecuteToolDef = map[string]any{
	"type": "function",
	"function": map[string]any{
		"name": ToolShellExecute,
		"description": `Execute a shell command step to help answer the user's operations question.
## Command Classification Criteria
When calling shell_execute, set command_type based on actual behavior, wrong classification leads to user misoperation:

"query" - Query commands (read-only):
Judgment criteria: Whether command only reads information, does NOT modify system in any way
- If after execution, system state remains unchanged → "query"
- If command only views, reads, displays information → "query"
- If command can be safely executed repeatedly with no side effects → "query"
Typical examples: ls, cat, df, free, ps, grep, find, stat, uname, systemctl status, docker ps, kubectl get

"modify" - State-changing commands:
Judgment criteria: Whether command changes system state or performs write operations
- If after execution, system state changes → "modify"
- If command creates, deletes, modifies, installs, uninstalls, starts, stops → "modify"
- If command has irreversible operations or side effects → "modify"
- If command affects managed infrastructure (Kubernetes, Docker, cloud APIs) → "modify"
Typical examples: install, remove, rm, mv, cp, mkdir, chmod, chown, kill, start, stop, restart, enable, disable

Judgment examples:
- systemctl status nginx → only views status, system unchanged → "query"
- sudo systemctl status nginx → sudo used only to view status, system unchanged → "query"
- systemctl restart nginx → restarts service, system changed → "modify"
- sudo rm /usr/local/bin/foo → deletes system file, system changed → "modify"
- cat /etc/hosts → only reads file, system unchanged → "query"
- sysctl -w net.ipv4.ip_forward=1 → changes kernel setting, system changed → "modify"
- docker ps → views containers, system unchanged → "query"
- docker rm xxx → deletes container, system changed → "modify"
- brew install xxx → installs software, system changed → "modify"
- curl -X GET http://api/metrics → only fetches data, no system change → "query"
- curl -X POST http://api/restart → triggers service restart, system changed → "modify"

## Targets
- Omit target to run on the local machine.
- Set target (host, IP, user@host, or SSH alias) when the user or Memory names that host, or when a command you actually ran discovered it (for example kubectl get nodes).
- Always run remote commands by setting target and placing the plain remote command in command. Do NOT omit target and wrap commands in manual "ssh <host> '...'" or "timeout ... ssh ..." locally (even if the user says "ssh to <host>"), except when multi-hop chaining cannot be expressed via target. The SSH runner already sets ConnectTimeout=10.
- Never invent a target and do not scrape known_hosts, ssh config, hosts files or inventories to guess one.
- SSH aliases and user@host targets are resolved by the system ssh client.

## Core Rules
- Interactive PTY execution: commands run in a real PTY attached to the user's terminal (locally and over ssh -t). Prompt-based interactive commands (such as sudo, ssh/MFA, passwd, package managers, and [y/N] confirmations) are supported.
- Full-screen TUI programs are prohibited and blocked at runtime: NEVER run full-screen editors (vi, vim, nvim, nano, emacs, visudo, crontab -e), full-screen pagers/monitors (less, more, htop, btop, watch, file managers, or top without -b or -l <samples>), or interactive tmux/screen sessions. Use file_edit to create or edit files (supports remote target and sudo=true).
- Non-paging / bounded inspection: pass non-paging or bounded flags where appropriate (for example --no-pager, top -b -n 1 on Linux / top -l 1 on macOS, ping -c 4, tail -n 100) so diagnostics finish automatically.
- System differences: consider OS differences (Linux/macOS), use compatible commands, avoid OS-specific syntax`,
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"step_index": map[string]any{
					"type":        "integer",
					"description": "Step sequence number, starting from 1",
				},
				"reason": map[string]any{
					"type":        "string",
					"description": "One-sentence explanation of what this command does and why it is needed",
				},
				"command": map[string]any{
					"type":        "string",
					"description": "The raw executable shell command only. **Do NOT include commentary, natural-language explanations, step numbers, or markdown fences.**",
				},
				"command_type": map[string]any{
					"type":        "string",
					"enum":        []string{"query", "modify"},
					"description": "query=read-only/no side effects, modify=changes system state, must follow classification rules above",
				},
				"target": map[string]any{
					"type":        "string",
					"description": "Optional SSH host, IP, user@host, or alias from the user, Memory, or a command you actually ran. Omit for the local machine. Use this instead of wrapping commands in manual 'ssh <host> ...'. Never invent a target.",
				},
			},
			"required": []string{"step_index", "reason", "command", "command_type"},
		},
	},
}

// ShellExecuteDefinition returns the shell_execute tool definition in a
// protocol-neutral form.
func ShellExecuteDefinition() Definition {
	return definitionFromMap(shellExecuteToolDef)
}

// CommandType classifies a shell command as query (read-only) or modify (state-changing).
type CommandType int

const (
	CommandTypeUnknown CommandType = iota
	CommandTypeQuery
	CommandTypeModify
)

// String returns the wire token used for the command type.
func (t CommandType) String() string {
	switch t {
	case CommandTypeQuery:
		return "query"
	case CommandTypeModify:
		return "modify"
	default:
		return "unknown"
	}
}

// ShellCommand represents a shell command step from an LLM tool call.
type ShellCommand struct {
	StepIndex   int
	Reason      string
	CommandLine string
	Target      string
	Type        CommandType
	ToolCallID  string
}

// RawToolCall is a minimal representation of an LLM tool call.
type RawToolCall struct {
	ID            string
	ToolName      string
	ArgumentsJSON string
}

// shellExecuteArgs is used to unmarshal tool call arguments.
type shellExecuteArgs struct {
	StepIndex   int    `json:"step_index"`
	Reason      string `json:"reason"`
	CommandLine string `json:"command"`
	CommandType string `json:"command_type"`
	Target      string `json:"target"`
}

// ParseShellCommand converts one LLM tool call into a ShellCommand.
func ParseShellCommand(tc RawToolCall) (ShellCommand, error) {
	if tc.ToolName != ToolShellExecute {
		return ShellCommand{}, errInvalidToolArgs(tc.ToolName)
	}
	var args shellExecuteArgs
	if err := json.Unmarshal([]byte(tc.ArgumentsJSON), &args); err != nil {
		return ShellCommand{}, err
	}
	cmdType, err := parseCommandType(args.CommandType)
	if err != nil {
		return ShellCommand{}, err
	}
	return ShellCommand{
		StepIndex:   args.StepIndex,
		Reason:      args.Reason,
		CommandLine: args.CommandLine,
		Target:      strings.TrimSpace(args.Target),
		Type:        cmdType,
		ToolCallID:  tc.ID,
	}, nil
}

func parseCommandType(value string) (CommandType, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return CommandTypeUnknown, nil
	case "query":
		return CommandTypeQuery, nil
	case "modify":
		return CommandTypeModify, nil
	default:
		return CommandTypeUnknown, errInvalidToolArgs(ToolShellExecute)
	}
}

// ToolName returns the canonical tool name.
func (c ShellCommand) ToolName() string { return ToolShellExecute }

// FormatInvocation renders the command, target and step for humans.
func (c ShellCommand) FormatInvocation(t Translator) string {
	target := c.Target
	if target == "" {
		target = t.T("tool.target.local")
	}
	return t.T("tool.invoke.shell_execute", target, c.CommandLine)
}

// FormatResult renders the invocation plus a localized status.
func (c ShellCommand) FormatResult(outcome ToolOutcome, t Translator) string {
	return formatResult(c.FormatInvocation(t), outcome, t)
}
