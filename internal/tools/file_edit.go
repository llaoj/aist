package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/llaoj/aist/internal/target"
)

var fileEditToolDef = map[string]any{
	"type": "function",
	"function": map[string]any{
		"name": ToolFileEdit,
		"description": `Make a targeted edit to a file on the local machine or a remote SSH target.
Rules:
- old_content must be an exact snippet that occurs exactly once in the existing file.
  Include enough surrounding context to make it unique.
- new_content replaces old_content. For a new file, leave old_content empty and put
  the complete file body in new_content.
- Omit target for the local machine, or set target to a verified SSH host, IP or alias from the user, Memory, or a command you actually ran. Never invent a target.
- Set sudo=true when reading or writing the file requires root privileges (for example root-owned files under /etc).
- The UI shows a diff and asks the user before writing, backs up the original file locally, and preserves permissions/ownership. Never claim a file was changed before the user confirms.
- Do not read or edit private keys, passwords, tokens or other secrets.
- Scope: creating or editing regular text files (single match per call). For deleting/moving files, chmod/chown, or multi-file batch operations, use shell_execute instead.`,
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Absolute path or ~/ path, for example /etc/nginx/nginx.conf or ~/.ssh/config.",
				},
				"old_content": map[string]any{
					"type":        "string",
					"description": "Exact existing text to replace. Empty only when creating a new file.",
				},
				"new_content": map[string]any{
					"type":        "string",
					"description": "Replacement text, or the full body when creating a new file.",
				},
				"reason": map[string]any{
					"type":        "string",
					"description": "One sentence explaining why this change is needed.",
				},
				"target": map[string]any{
					"type":        "string",
					"description": "Optional SSH host, IP or alias from the user, Memory, or a command you actually ran. Omit for the local machine. Never invent a target.",
				},
				"sudo": map[string]any{
					"type":        "boolean",
					"description": "Optional boolean. Set true when reading or writing the file requires root/sudo privileges.",
				},
			},
			"required": []string{"path", "old_content", "new_content", "reason"},
		},
	},
}

// FileEditArgs is the decoded file_edit call.
type FileEditArgs struct {
	Path       string `json:"path"`
	OldContent string `json:"old_content"`
	NewContent string `json:"new_content"`
	Reason     string `json:"reason"`
	Target     string `json:"target"`
	UseSudo    bool   `json:"sudo"`
}

// FileEditDefinition returns the file_edit tool definition.
func FileEditDefinition() Definition {
	return definitionFromMap(fileEditToolDef)
}

// ParseFileEditArgs decodes and validates file_edit arguments. Content is kept
// byte-for-byte; only metadata fields are trimmed.
func ParseFileEditArgs(raw string) (FileEditArgs, error) {
	var args FileEditArgs
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return FileEditArgs{}, err
	}
	args.Path = strings.TrimSpace(args.Path)
	args.Reason = strings.TrimSpace(args.Reason)
	args.Target = strings.TrimSpace(args.Target)
	if args.Path == "" {
		return FileEditArgs{}, errors.New("file_edit path is required")
	}
	if _, err := target.New(args.Target); err != nil {
		return FileEditArgs{}, fmt.Errorf("file_edit target is invalid: %w", err)
	}
	if strings.TrimSpace(args.NewContent) == "" {
		return FileEditArgs{}, errors.New("file_edit new_content is required")
	}
	return args, nil
}

// ToolName returns the canonical tool name.
func (a FileEditArgs) ToolName() string { return ToolFileEdit }

// FormatInvocation renders the target file path.
func (a FileEditArgs) FormatInvocation(t Translator) string {
	parsed, _ := target.New(a.Target)
	return t.T("tool.invoke.file_edit", parsed.RenderPath(a.Path, a.UseSudo))
}

// FormatResult renders the invocation plus a localized status.
func (a FileEditArgs) FormatResult(outcome ToolOutcome, t Translator) string {
	return formatResult(a.FormatInvocation(t), outcome, t)
}
