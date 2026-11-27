package i18n

// ChineseMessages contains all UI messages in Chinese
var ChineseMessages = map[string]string{
	// Interactive mode messages
	"interactive.welcome":               "欢迎使用 AI Shell assistanT (AIST)",
	"interactive.exit_hint":             "提示: Esc 取消请求，Ctrl+C 退出",
	"interactive.input_prompt":          "用户:",
	"interactive.input_key_hint":        "Enter 发送 · Ctrl+C 退出",
	"interactive.goodbye":               "再见！",
	"interactive.thinking":              "思考中",
	"interactive.execution_output":      "执行输出",
	"interactive.user_label":            "用户",
	"interactive.turn_complete":         "✓ 本轮已完成",
	"interactive.reject_remaining_hint": "已停止本轮后续操作，请补充更多说明后继续。",
	"interactive.followup_limit_hint":   "已达到连续自动工具调用上限（%d 轮），已暂停自动执行；输入“继续”可接着处理，或补充更多说明。",
	"interactive.think_content":         "思考内容:",
	"interactive.esc_interrupt":         "✗ 已取消，可继续输入",
	"interactive.thinking_key_hint":     "Esc 取消请求 · Ctrl+C 退出",
	"interactive.file_change":           "文件修改",
	"interactive.file_change_reason":    "原因",
	"interactive.file_change_target":    "目标",

	// Executor messages
	"executor.query_command":             "查询命令:",
	"executor.modify_command":            "修改命令:",
	"executor.execute_prompt":            "是否执行此命令?",
	"executor.execute_prompt_target":     "是否在 [%s] 上执行此命令?",
	"executor.confirm.run":               "执行",
	"executor.confirm.skip":              "跳过此命令",
	"executor.confirm.reject_remaining":  "拒绝全部，由我补充说明",
	"executor.modify_warning":            "警告: 该命令将修改服务器状态，是否确定执行?",
	"executor.modify_warning_target":     "警告: 该命令将修改 [%s] 上的状态，是否确定执行?",
	"executor.executing":                 "执行中",
	"executor.execute_success":           "✓ 执行成功",
	"executor.execute_success_exit_code": "✓ 执行成功 (退出码: %d)",
	"executor.execute_failed":            "✗ 执行失败: %v",
	"executor.command_timeout":           "命令执行超时 (%s)，已终止执行",
	"executor.command_stuck_exit":        "命令输出结束后 %s 内未退出，已终止执行",
	"executor.ssh_unreachable":           "SSH 连接失败，请使用 `ssh %s` 检查连接配置",
	"executor.command_canceled":          "命令已取消",
	"executor.command_deadline_exceeded": "命令执行超时",

	// Blacklist messages
	"executor.blacklisted":         "✗ 命令被拒绝: 该命令匹配黑名单规则 '%s'，禁止执行",
	"executor.blacklist_hint":      "如需执行此命令，请联系管理员申请权限或修改黑名单配置",
	"executor.tui_editor_blocked":  "✗ 已拦截全屏编辑器命令 '%s'：编辑文件请使用 file_edit 工具（支持远端 target 与 sudo）",
	"executor.tui_monitor_blocked": "✗ 已拦截全屏交互程序 '%s'：请改用非分页或单次采样参数（如 --no-pager、top -b -n 1 / top -l 1）",

	// Error messages
	"error.no_models":                "✗ 错误: 未配置任何模型",
	"error.hint_no_models":           "当前账号没有可用模型；请重新运行 aist login 或联系管理员",
	"error.general":                  "✗ 错误: %v",
	"error.auth_expired":             "登录状态已失效，请运行 aist login 重新登录",
	"error.quota_exceeded":           "本周 Token 额度已用完，请下周再试或联系管理员",
	"error.quota_exceeded_retry":     "本周 Token 额度已用完，将于 %s 恢复可用",
	"error.no_available_model":       "当前没有可用的模型，请稍后再试或联系管理员",
	"error.no_available_model_retry": "当前没有可用的模型（上游额度耗尽或限流），预计 %s 后恢复",
	"error.upstream_unavailable":     "上游模型服务暂时不可用，请稍后再试",
	"error.invalid_request":          "请求无效，请检查输入或开启新会话",
	"error.invalid_request_detail":   "上游拒绝了请求: %s",

	// Login messages
	"login.quick_start_title":       "快速开始:",
	"login.quick_start_interactive": "  aist                  # 进入交互模式",
	"login.quick_start_question":    `  aist "你的问题"       # 直接提问一次`,
	"login.quick_start_lang":        "  aist lang en          # 切换为英文",

	// Whoami messages
	"whoami.email":             "邮箱:     %s",
	"whoami.server":            "服务端:   %s",
	"whoami.status_logged_in":  "状态:     已登录",
	"whoami.status_expired":    "状态:     登录已失效（请运行 aist login 重新登录）",
	"whoami.usage_quota":       "本周用量: %s / %s tokens（剩余 %s，共 %d 次调用）",
	"whoami.usage_unlimited":   "本周用量: %s tokens（不限额度，共 %d 次调用）",
	"whoami.reset_at":          "重置时间: %s",
	"whoami.usage_unavailable": "本周用量: 无法获取实时用量（离线或服务端暂不可达）",
	"whoami.usage_unsupported": "本周用量: 当前服务端版本暂不支持",

	// Language preference messages
	"lang.switched": "已切换为简体中文 (zh)",

	// Version messages
	"version.app_name":   "AI Shell assistanT (AIST)",
	"version.version":    "版本: %s",
	"version.commit":     "提交: %s",
	"version.build_date": "构建日期: %s",

	// Confirmation prompt messages
	"confirm.yes":              "是",
	"confirm.no":               "否",
	"confirm.reject_remaining": "拒绝全部，由我补充说明",
	"confirm.key_hint":         "↑↓ 选择 · Enter 确认 · Ctrl+C 退出会话",

	// Model status messages
	"llm.status_title":   "当前模型",
	"llm.status_default": "(默认)",

	// Memory messages
	"memory.saved_created":  "✓ 已记住: %s",
	"memory.saved_updated":  "✓ 已更新记忆: %s",
	"memory.saved_replaced": "✓ 已覆盖记忆: %s",

	// Session messages
	"sessions.title":          "选择会话",
	"sessions.key_hint":       "↑↓ 选择 · Enter 恢复 · Ctrl+C 退出",
	"sessions.empty":          "暂无已保存的会话",
	"sessions.restored":       "已恢复会话: %s (%d 条消息)",
	"sessions.history_header": "历史消息",
	"sessions.compacted":      "上下文已压缩",
	"sessions.list_failed":    "读取会话失败: %v",

	// Pipe mode messages
	"pipe.input_loaded":        "已载入管道输入（%d 行）",
	"pipe.input_loaded_prompt": "已载入管道输入（%d 行），请输入你的问题。",
	"pipe.question_required":   `非交互管道模式下请提供问题，例如: cmd | aist "你的问题"`,

	// Tool display messages
	"tool.invoke.shell_execute": "[%s] $ %s",
	"tool.invoke.memory_search": "搜索记忆「%s」",
	"tool.invoke.memory_save":   "保存记忆「%s」（%s）",
	"tool.invoke.file_edit":     "编辑文件 %s",
	"tool.invoke.unknown":       "未知工具「%s」",
	"tool.target.local":         "本机",
	"tool.mode.append":          "追加",
	"tool.mode.replace":         "覆盖",
	"tool.status.success":       "成功",
	"tool.status.failed":        "失败",
	"tool.status.skipped":       "已跳过",
	"tool.status.rejected":      "已拒绝",
	"tool.history.prefix":       "工具",
	"tool.result.line":          "%s · %s",

	// File edit messages
	"file_edit.confirm":        "是否应用对 %s 的修改？",
	"file_edit.modify_warning": "警告: 该操作将修改 %s，是否确定执行？",
	"file_edit.updated":        "✓ 已更新文件: %s",
}
