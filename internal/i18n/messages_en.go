package i18n

// EnglishMessages contains all UI messages in English
var EnglishMessages = map[string]string{
	// Interactive mode messages
	"interactive.welcome":               "Welcome to AI Shell assistanT (AIST)",
	"interactive.exit_hint":             "Tip: Esc cancels a request, Ctrl+C exits",
	"interactive.input_prompt":          "User:",
	"interactive.input_key_hint":        "Enter to send · Ctrl+C to exit",
	"interactive.goodbye":               "Goodbye!",
	"interactive.thinking":              "Thinking",
	"interactive.execution_output":      "Execution output",
	"interactive.user_label":            "User",
	"interactive.turn_complete":         "✓ Turn complete, please continue with questions",
	"interactive.reject_remaining_hint": "Stopped the remaining actions. Please add more details to continue.",
	"interactive.followup_limit_hint":   `Reached the consecutive unattended tool call limit (%d rounds) and paused; enter "continue" to proceed or add more details.`,
	"interactive.think_content":         "Thinking Content:",
	"interactive.esc_interrupt":         "✗ Canceled. Continue typing.",
	"interactive.thinking_key_hint":     "Esc to cancel · Ctrl+C to exit",
	"interactive.file_change":           "File edit",
	"interactive.file_change_reason":    "Reason",
	"interactive.file_change_target":    "Target",

	// Executor messages
	"executor.query_command":             "Query command:",
	"executor.modify_command":            "Modify command:",
	"executor.execute_prompt":            "Execute this command?",
	"executor.execute_prompt_target":     "Execute this command on [%s]?",
	"executor.confirm.run":               "Run",
	"executor.confirm.skip":              "Skip this command",
	"executor.confirm.reject_remaining":  "Reject all and let me add details",
	"executor.modify_warning":            "Warning: This command will modify server state, are you sure?",
	"executor.modify_warning_target":     "Warning: This command will modify state on [%s], are you sure?",
	"executor.executing":                 "Executing",
	"executor.execute_success":           "✓ Execution successful",
	"executor.execute_success_exit_code": "✓ Execution successful (exit code: %d)",
	"executor.execute_failed":            "✗ Execution failed: %v",
	"executor.command_timeout":           "command timed out after %s and was terminated",
	"executor.command_stuck_exit":        "command did not exit within %s after its output ended and was terminated",
	"executor.ssh_unreachable":           "SSH connection failed; verify access with `ssh %s`",
	"executor.command_canceled":          "command canceled",
	"executor.command_deadline_exceeded": "command timed out",

	// Blacklist messages
	"executor.blacklisted":         "✗ Command rejected: This command matches blacklist rule '%s', execution forbidden",
	"executor.blacklist_hint":      "To execute this command, please contact the administrator for permission or modify the blacklist configuration",
	"executor.tui_editor_blocked":  "✗ Blocked full-screen editor '%s': use the file_edit tool instead (supports remote target and sudo)",
	"executor.tui_monitor_blocked": "✗ Blocked full-screen TUI program '%s': use non-paging or batch flags instead (e.g. --no-pager, top -b -n 1 / top -l 1)",

	// Error messages
	"error.no_models":                "✗ Error: No models configured",
	"error.hint_no_models":           `No models are available for your account; run "aist login" or contact your administrator`,
	"error.general":                  "✗ Error: %v",
	"error.auth_expired":             `Authentication expired; run "aist login" to sign in again`,
	"error.quota_exceeded":           "Weekly token quota exceeded; try again next week or contact your administrator",
	"error.quota_exceeded_retry":     "Weekly token quota exceeded; available again at %s",
	"error.no_available_model":       "No model is currently available; try again later or contact your administrator",
	"error.no_available_model_retry": "No model is currently available (upstream quota or rate limit); retry after %s",
	"error.upstream_unavailable":     "Upstream model service is temporarily unavailable; please try again later",
	"error.invalid_request":          "Invalid request; check your input or start a new session",
	"error.invalid_request_detail":   "Request rejected: %s",

	// Login messages
	"login.quick_start_title":       "Quick start:",
	"login.quick_start_interactive": "  aist                  # enter interactive mode",
	"login.quick_start_question":    `  aist "your question"  # ask a single question`,
	"login.quick_start_lang":        "  aist lang zh          # switch to Chinese",

	// Whoami messages
	"whoami.email":             "Email:        %s",
	"whoami.server":            "Server:       %s",
	"whoami.status_logged_in":  "Status:       logged in",
	"whoami.status_expired":    `Status:       login expired (run "aist login" to sign in again)`,
	"whoami.usage_quota":       "Weekly usage: %s / %s tokens (%s remaining, %d requests)",
	"whoami.usage_unlimited":   "Weekly usage: %s tokens (unlimited, %d requests)",
	"whoami.reset_at":          "Resets at:    %s",
	"whoami.usage_unavailable": "Weekly usage: unavailable (offline or server unreachable)",
	"whoami.usage_unsupported": "Weekly usage: unsupported by this server version",

	// Language preference messages
	"lang.switched": "Language switched to English (en)",

	// Version messages
	"version.app_name":   "AI Shell assistanT (AIST)",
	"version.version":    "Version: %s",
	"version.commit":     "Commit: %s",
	"version.build_date": "Build Date: %s",

	// Confirmation prompt messages
	"confirm.yes":              "Yes",
	"confirm.no":               "No",
	"confirm.reject_remaining": "Reject all and let me add details",
	"confirm.key_hint":         "↑↓ select · Enter confirm · Ctrl+C exit session",

	// Model status messages
	"llm.status_title":   "Current Model",
	"llm.status_default": "(Default)",

	// Memory messages
	"memory.saved_created":  "✓ Remembered: %s",
	"memory.saved_updated":  "✓ Updated Memory: %s",
	"memory.saved_replaced": "✓ Replaced Memory: %s",

	// Session messages
	"sessions.title":          "Select a session",
	"sessions.key_hint":       "↑↓ select · Enter resume · Ctrl+C quit",
	"sessions.empty":          "No saved sessions yet",
	"sessions.continue_empty": "No saved sessions yet; starting a new session",
	"sessions.restored":       "Restored session: %s (%d messages)",
	"sessions.history_header": "History messages",
	"sessions.compacted":      "Context compacted",
	"sessions.list_failed":    "Failed to read sessions: %v",

	// Pipe mode messages
	"pipe.input_loaded":        "Loaded piped input (%d lines)",
	"pipe.input_loaded_prompt": "Loaded piped input (%d lines). Enter your question.",
	"pipe.question_required":   `a question is required in non-interactive pipe mode, e.g. cmd | aist "your question"`,

	// Tool display messages
	"tool.invoke.shell_execute": "[%s] $ %s",
	"tool.invoke.memory_search": "Memory search %q",
	"tool.invoke.memory_save":   "Memory save %q (%s)",
	"tool.invoke.file_edit":     "File edit %s",
	"tool.invoke.unknown":       "Unknown tool %q",
	"tool.target.local":         "local",
	"tool.mode.append":          "append",
	"tool.mode.replace":         "replace",
	"tool.status.success":       "success",
	"tool.status.failed":        "failed",
	"tool.status.skipped":       "skipped",
	"tool.status.rejected":      "rejected",
	"tool.history.prefix":       "tool",
	"tool.result.line":          "%s · %s",

	// File edit messages
	"file_edit.confirm":        "Apply this change to %s?",
	"file_edit.modify_warning": "Warning: This will modify %s, are you sure?",
	"file_edit.updated":        "✓ Updated file: %s",
}
