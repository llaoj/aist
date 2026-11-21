package server

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/llaoj/aist/internal/llm"
	"github.com/llaoj/aist/internal/session"
	"github.com/llaoj/aist/internal/target"
	"github.com/llaoj/aist/internal/tools"
	"github.com/llaoj/aist/internal/util"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/security"
)

// toolAuditLogType distinguishes tool audit logs from PocketBase "request" logs.
const toolAuditLogType = "tool"

// auditFieldMaxBytes caps string fields by UTF-8 bytes (prefix kept). Fields
// without an entry, such as path or identity fields, are short by nature.
var auditFieldMaxBytes = map[string]int{
	"result":  6000,
	"command": 4000,
	"old":     4000,
	"new":     4000,
	"args":    2000,
	"reason":  500,
}

// auditMaxDataBytes keeps the serialized data under PocketBase's default 16KB
// cap, which otherwise drops whole keys in alphabetical order (type and tool
// sort late and would be lost first). JSON escaping of control characters can
// inflate values well beyond their raw size.
const auditMaxDataBytes = 14 << 10

// auditMarkerBytes reserves room for the truncation marker so every shrink
// step strictly reduces the size.
const auditMarkerBytes = 32

const auditMessageMaxBytes = 300

const requestIDLength = 15

// toolAuditor converts the arguments of one audited tool call into the log
// message and tool-specific attributes.
type toolAuditor func(argsJSON string) (message string, attrs []any, err error)

var auditedTools = map[string]toolAuditor{
	tools.ToolShellExecute: auditShellExecute,
	tools.ToolFileEdit:     auditFileEdit,
}

// toolAuditEntry pairs an audited tool call with the result returned to the model.
type toolAuditEntry struct {
	ToolName string
	Args     string
	Result   string
}

func newRequestID() string {
	return security.RandomString(requestIDLength)
}

// toolAuditActor identifies who sent the request that carried the tool results.
// UserIP stays empty when the PocketBase Logs.LogIP setting is off.
type toolAuditActor struct {
	AuthID    string
	Email     string
	UserIP    string
	RequestID string
}

// toolAuditLog is one log line ready for app.Logger().
type toolAuditLog struct {
	Message string
	Attrs   []any
}

// recordToolAudit writes one log per new audited tool result in the chat
// body and tags the request activity log with the same requestId. It never
// fails the proxied request.
func (s *API) recordToolAudit(e *core.RequestEvent, user *core.Record, body []byte) {
	requestID := newRequestID()
	e.Set(apis.RequestEventKeyLogMeta, map[string]any{"requestId": requestID})

	actor := toolAuditActor{AuthID: user.Id, Email: user.Email(), RequestID: requestID}
	if e.App.Settings().Logs.LogIP {
		actor.UserIP = e.RealIP()
	}
	logs, err := buildToolAuditLogs(body, actor)
	if err != nil {
		e.App.Logger().Debug("tool audit skipped: invalid chat body", "error", err.Error())
		return
	}
	for _, log := range logs {
		e.App.Logger().Info(log.Message, log.Attrs...)
	}
}

func buildToolAuditLogs(body []byte, actor toolAuditActor) ([]toolAuditLog, error) {
	entries, err := pendingToolAudits(body)
	if err != nil {
		return nil, err
	}
	logs := make([]toolAuditLog, 0, len(entries))
	for _, entry := range entries {
		message, attrs, err := auditedTools[entry.ToolName](entry.Args)
		if err != nil {
			message = entry.ToolName + ": invalid arguments"
			attrs = []any{"args", entry.Args}
		}
		attrs = append(attrs,
			"type", toolAuditLogType,
			"tool", entry.ToolName,
			"authId", actor.AuthID,
			"email", actor.Email,
			"requestId", actor.RequestID,
			"result", entry.Result,
		)
		if actor.UserIP != "" {
			attrs = append(attrs, "userIP", actor.UserIP)
		}
		logs = append(logs, toolAuditLog{
			Message: util.TruncatePrefix(message, auditMessageMaxBytes),
			Attrs:   limitAuditAttrs(attrs),
		})
	}
	return logs, nil
}

// pendingToolAudits returns the audited tool results that follow the last
// assistant message. Earlier results were already audited when they first
// appeared, because every request resends the whole history.
func pendingToolAudits(body []byte) ([]toolAuditEntry, error) {
	var payload struct {
		Messages []llm.ChatMessage `json:"messages"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	lastAssistant := -1
	for i := len(payload.Messages) - 1; i >= 0; i-- {
		if payload.Messages[i].Role == string(session.RoleAssistant) {
			lastAssistant = i
			break
		}
	}
	if lastAssistant < 0 {
		return nil, nil
	}

	calls := make(map[string]llm.ToolCall, len(payload.Messages[lastAssistant].ToolCalls))
	for _, call := range payload.Messages[lastAssistant].ToolCalls {
		calls[call.ID] = call
	}

	var entries []toolAuditEntry
	for _, message := range payload.Messages[lastAssistant+1:] {
		if message.Role != string(session.RoleTool) {
			continue
		}
		call, found := calls[message.ToolCallID]
		if !found {
			continue
		}
		if _, audited := auditedTools[call.Function.Name]; !audited {
			continue
		}
		entries = append(entries, toolAuditEntry{
			ToolName: call.Function.Name,
			Args:     call.Function.Arguments,
			Result:   llm.DerefPtr(message.Content),
		})
	}
	return entries, nil
}

func auditShellExecute(argsJSON string) (string, []any, error) {
	cmd, err := tools.ParseShellCommand(tools.RawToolCall{ToolName: tools.ToolShellExecute, ArgumentsJSON: argsJSON})
	if err != nil {
		return "", nil, err
	}
	targetLabel := auditTargetLabel(cmd.Target)
	message := tools.ToolShellExecute + " @" + targetLabel + ": " + singleLine(cmd.CommandLine)
	return message, []any{
		"target", targetLabel,
		"command", cmd.CommandLine,
		"commandType", cmd.Type.String(),
		"reason", cmd.Reason,
	}, nil
}

func auditFileEdit(argsJSON string) (string, []any, error) {
	args, err := tools.ParseFileEditArgs(argsJSON)
	if err != nil {
		return "", nil, err
	}
	targetLabel := auditTargetLabel(args.Target)
	message := tools.ToolFileEdit + " @" + targetLabel + ": " + singleLine(args.Path)
	return message, []any{
		"target", targetLabel,
		"path", args.Path,
		"sudo", args.UseSudo,
		"reason", args.Reason,
		"old", args.OldContent,
		"new", args.NewContent,
	}, nil
}

func auditTargetLabel(raw string) string {
	return util.FirstNonEmpty(raw, target.LocalName)
}

func singleLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

// limitAuditAttrs applies auditFieldMaxBytes and then, while the serialized
// data exceeds auditMaxDataBytes, shortens whichever limited field encodes
// largest, so the field that inflates (for example via escaped control
// characters) pays first. Values are always cut from the original text so each
// carries a single marker.
func limitAuditAttrs(attrs []any) []any {
	type limitedValue struct {
		index  int
		raw    string
		budget int
	}
	out := slices.Clone(attrs)
	values := make(map[string]*limitedValue)
	for i := 0; i+1 < len(out); i += 2 {
		key, _ := out[i].(string)
		raw, isString := out[i+1].(string)
		budget, limited := auditFieldMaxBytes[key]
		if !isString || !limited {
			continue
		}
		values[key] = &limitedValue{index: i + 1, raw: raw, budget: min(budget, len(raw))}
		out[i+1] = util.TruncatePrefix(raw, budget)
	}
	for {
		excess := auditDataSize(out) - auditMaxDataBytes
		if excess <= 0 {
			return out
		}
		var largest *limitedValue
		largestSize := 0
		for _, value := range values {
			if size := encodedSize(out[value.index]); value.budget > 0 && size > largestSize {
				largest, largestSize = value, size
			}
		}
		if largest == nil {
			return out
		}
		// Scale the encoded excess back to raw bytes by this field's escaping
		// ratio so heavily escaped values are not cut far more than needed.
		rawCut := excess*largest.budget/largestSize + auditMarkerBytes
		largest.budget = max(0, largest.budget-rawCut)
		out[largest.index] = util.TruncatePrefix(largest.raw, largest.budget)
	}
}

func encodedSize(value any) int {
	encoded, err := json.Marshal(value)
	if err != nil {
		return 0
	}
	return len(encoded)
}

// auditDataSize estimates the serialized data size. encoding/json escapes
// HTML characters, so it never underestimates PocketBase's encoding.
func auditDataSize(attrs []any) int {
	data := make(map[string]any, len(attrs)/2)
	for i := 0; i+1 < len(attrs); i += 2 {
		key, _ := attrs[i].(string)
		data[key] = attrs[i+1]
	}
	return encodedSize(data)
}
