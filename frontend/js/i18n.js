(function () {
  "use strict";

  var STORAGE_KEY = "aist-lang";

  var messages = {
    zh: {
      "meta.title": "AIST - 首个 ON-CALL 运维智能体",
      "meta.description":
        "AIST（AI Shell assistanT）是常驻服务器与终端控制台的 On-Call 运维智能体：自然语言诊断、确认后执行、连续分析，支持私有化部署，数据不出内网。",
      "nav.aria": "主导航",
      "nav.features": "特性",
      "nav.usage": "使用方式",
      "nav.quickstart": "快速开始",
      "nav.contact": "商务洽谈",
      "nav.security": "安全",
      "nav.docs": "文档",
      "nav.register": "注册",
      "docs.href": "https://docs.aistrun.cc/zh/",
      "nav.open": "打开菜单",
      "nav.close": "关闭菜单",
      "lang.switch": "切换语言",
      "hero.meta.left": "首个 ON-CALL 运维 AGENT",
      "hero.meta.right": "● 10MB · ZERO DEPENDENCY",
      "hero.eyebrow": "[ 01 ] ON-CALL AGENT · 常驻终端",
      "hero.title.line1": "用自然语言，",
      "hero.title.line2": "驱动每一次运维",
      "hero.lead":
        "常驻服务器与终端控制台的 On-Call 运维智能体。自然语言诊断、确认后执行、连续分析，数据不出内网。",
      "hero.cta.primary": "快速开始",
      "hero.cta.ghost": "查看安全设计",
      "hero.spec.binary": "单文件二进制",
      "hero.spec.deps": "运行时依赖",
      "hero.spec.value.hosted": "私有化",
      "hero.spec.hosted": "数据不出内网",
      "hero.spec.value.memory": "记忆",
      "hero.spec.memory": "越用越懂环境",
      "hero.platform":
        "// linux-x86_64 · linux-arm64 · linux-armv6/v7 · linux-i386 · linux-riscv64 · darwin-x86_64 · darwin-arm64",
      "hero.caption": "自然语言 → 诊断 → 确认 → 执行 → 连续分析",
      "features.title": "核心特性",
      "features.sub":
        "为服务器运维与云原生运维原生打造。有 Shell 的地方，就能拥有 AI。",
      "feature.1.title": "自然语言交互",
      "feature.1.text": "用人话提问，AI 给出专业诊断与最合适的 Shell 命令。",
      "feature.2.title": "连续对话分析",
      "feature.2.text": "自动传递上一条命令的输出，支持递归诊断，层层深入。",
      "feature.3.title": "操作审计日志",
      "feature.3.text":
        "谁在何时操作了哪台服务器、执行了什么命令、修改了哪些文件及结果，全程记录在审计日志中。",
      "feature.4.title": "管道模式",
      "feature.4.text": "直接分析日志输出，单次可处理约 13,000 行数据并生成报告。",
      "feature.5.title": "多模型支持",
      "feature.5.text": "兼容所有 OpenAI 接口，当前模型不可用时自动切换。",
      "feature.6.title": "安全可控",
      "feature.6.text": "查询命令与修改命令差异化展示，修改命令二次确认。",
      "feature.7.title": "命令黑名单",
      "feature.7.text": "提示层 + 执行拦截层双重防护，敏感命令无法执行。",
      "feature.8.title": "私有化部署",
      "feature.8.text": "部署到单位或部门自有服务器与内网，数据不出内网。",
      "feature.9.title": "专属模型配置",
      "feature.9.text": "接入公有云模型或自建模型服务，模型与额度按组织配置。",
      "memory.title": "环境记忆 · 越用越懂你的基础设施",
      "memory.text":
        "持续使用后，aist 会逐步记住你的环境信息。下一次排查，不必再交代服务器环境、集群拓扑与节点角色，它会带着对环境的理解直接进入分析。",
      "memory.tag.topology": "服务器与集群拓扑",
      "memory.tag.roles": "节点角色与职责",
      "memory.tag.ports": "常用服务与端口",
      "memory.tag.deploy": "部署方式与路径",
      "memory.tag.incidents": "历史故障与处置经验",
      "footprint.title": "极致轻量 · 秒级集成",
      "footprint.text":
        "编译为单个原生二进制，无需 Node.js、Java 等运行时环境。只要是有 Shell 的环境或设备，拷贝即用。",
      "footprint.aist.size": "约 10 MB",
      "footprint.codex.size": "约 80 MB",
      "footprint.claude.size": "约 250 MB",
      "why.title": "首个 On-Call 运维智能体",
      "why.sub":
        "不是浏览器里的聊天窗口，而是常驻服务器、就在终端控制台里的运维 Agent。",
      "why.a.title": "从查文档到直接诊断",
      "why.a.text":
        "不再需要在搜索引擎、文档和终端之间来回切换，AI 直接理解现场并给出可执行的排查路径。",
      "why.b.title": "从人工串联到连续推理",
      "why.b.text":
        "每一步命令的输出都会进入下一轮判断，像一位经验丰富的工程师一样层层深入，而不是一问一答。",
      "why.c.title": "从浏览器到终端",
      "why.c.text":
        "不需要打开网页或额外的软件平台，Agent 常驻服务器与终端控制台，在真实运维现场就地待命。",
      "why.note":
        "这是一种全新的运维交互模式：把自然语言、现场执行与环境记忆连成闭环，让 AI 就在你的控制台里随时待命。",
      "quickstart.title": "快速开始",
      "quickstart.sub": "一键安装 · 下载即用 · 仅约 10 MB",
      "quickstart.note.lead":
        "脚本会自动识别 OS / 架构并完成安装或升级；首次运行会引导登录，没有账号可先",
      "quickstart.register": "注册账号",
      "quickstart.note.mid": "，如",
      "quickstart.forgot": "忘记密码",
      "quickstart.note.tail": "可在线重置。",
      "quickstart.platform":
        "// 支持 linux-x86_64 · linux-arm64 · linux-armv6/v7 · linux-i386 · linux-riscv64 · darwin-x86_64 · darwin-arm64",
      "copy.label": "复制",
      "copy.done": "已复制",
      "usage.title": "使用方式",
      "usage.sub": "三种模式覆盖日常运维场景。",
      "usage.tabs.aria": "使用方式",
      "usage.tab.interactive": "交互式",
      "usage.tab.single": "单次问答",
      "usage.tab.pipe": "管道分析",
      "usage.interactive.desc":
        "进入连续对话，AI 会结合上一轮的命令输出继续分析。",
      "usage.single.desc":
        "一次提问即得到诊断建议，执行建议命令并完成本轮分析后退出。",
      "usage.pipe.desc": "管道模式是只读的：只输出分析结果，不执行任何命令。",
      "usage.single.cmd": "$ aist \"服务器负载很高，帮我排查原因\"",
      "usage.pipe.cmd":
        "$ tail -n 200 /var/log/nginx/access.log | aist \"分析日志\"",
      "contact.bar.left": "[ ENTERPRISE · 商务洽谈 ]",
      "contact.bar.right": "专属顾问响应",
      "contact.title.line1": "定制专属方案，",
      "contact.title.line2": "让智能体常驻内网",
      "contact.lead":
        "为企业、团队与部门提供私有化部署与长期服务。欢迎沟通基础设施规模与场景需求，获取方案评估、部署调试与专属报价。",
      "contact.pillar.1.val": "私有化",
      "contact.pillar.1.label": "数据不出内网",
      "contact.pillar.2.val": "长期账号",
      "contact.pillar.2.label": "组织统一管理",
      "contact.pillar.3.val": "任意模型",
      "contact.pillar.3.label": "兼容 OpenAI 接口",
      "contact.pillar.4.val": "1v1",
      "contact.pillar.4.label": "全程专属支持",
      "contact.chat.badge": "LIVE CHAT · 在线",
      "contact.chat.title": "在线客服实时沟通",
      "contact.chat.desc":
        "即时发起对话，快速咨询部署方式、模型适配与授权报价。",
      "contact.chat.btn": "唤起在线客服",
      "contact.email.badge": "DIRECT EMAIL · 商务邮箱",
      "contact.email.btn": "发送商务邮件",
      "security.title": "安全设计",
      "security.sub": "事前拦截、事中确认、事后可审计——每一步都在你的掌控之中。",
      "security.flow.1": "生成命令",
      "security.flow.2": "黑名单提示",
      "security.flow.3": "执行前拦截",
      "security.flow.4": "分级确认",
      "security.flow.5": "执行命令",
      "security.flow.6": "审计留痕",
      "security.table.type": "类型",
      "security.table.label": "显示标签",
      "security.table.color": "颜色",
      "security.table.confirm": "确认次数",
      "security.table.example": "示例",
      "security.table.query": "查询命令",
      "security.table.query.label": "查询命令:",
      "security.table.color.gold": "金色",
      "security.table.confirm.once": "1 次",
      "security.table.modify": "修改命令",
      "security.table.modify.label": "修改命令:",
      "security.table.color.red": "红色",
      "security.table.confirm.twice": "2 次",
      "security.blacklist.title": "命令黑名单",
      "security.blacklist.sub.prefix":
        "黑名单会注入提示词，且命中命令在执行前直接拦截、不进入确认流程；尾部",
      "security.blacklist.sub.middle": "匹配至少一个参数，无",
      "security.blacklist.sub.suffix": "时按前缀词匹配。",
      "security.blacklist.code":
        "blacklist:\n  - \"rm *\"               # 禁止带参数的 rm\n  - \"dd *\"               # 禁止 dd\n  - \"kubectl delete *\"   # 禁止 kubectl delete\n  - \"shutdown\"           # 禁止 shutdown\n  - \"reboot\"             # 禁止 reboot",
      "security.audit.title": "操作审计日志",
      "security.audit.sub":
        "谁（账号、来源 IP）在何时对哪台目标服务器执行了什么命令（查询还是变更）、修改了哪个文件，人工确认、跳过还是拒绝，以及退出码与输出，全部记录在服务端审计日志中，可在管理后台按用户、主机、命令检索，满足事后复盘与合规审计。",
      "models.title": "支持的模型提供商",
      "models.sub": "兼容所有 OpenAI 接口。",
      "models.ticker": "支持的模型提供商",
      "models.qwen": "通义千问 Qwen",
      "models.gpt": "OpenAI GPT",
      "models.deepseek": "DeepSeek",
      "models.kimi": "Moonshot Kimi",
      "models.glm": "智谱 GLM",
      "models.compatible": "OpenAI-Compatible",
      "footer.tagline": "AI Shell assistanT · 首个 On-Call 运维智能体",
      "footer.quickstart": "快速开始",
      "footer.contact": "商务洽谈",
      "footer.security": "安全设计",
      "footer.docs": "文档",
      "footer.operational": "运行中",
      "register.meta.title": "AIST - 账号注册",
      "register.step.form": "01 · 创建账号",
      "register.step.done": "02 · 安装客户端",
      "register.title": "使用邀请码注册",
      "register.sub": "创建账号后即可在终端安装并登录 AIST 客户端。",
      "register.field.invite": "邀请码",
      "register.placeholder.invite": "输入邀请码",
      "register.field.email": "邮箱",
      "register.placeholder.email": "you@example.com",
      "register.field.password": "密码",
      "register.placeholder.password": "至少 8 位字符",
      "register.password.show": "显示密码",
      "register.password.hide": "隐藏密码",
      "register.submit": "注册",
      "register.submitting": "提交中...",
      "register.forgot_entry": "已有账号，忘记密码？",
      "register.done.title": "账号创建成功",
      "register.done.lead_prefix": "账号 ",
      "register.done.lead_suffix": " 已创建。复制下方命令在终端安装客户端并登录：",
      "register.done.verify_notice":
        "验证邮件已发出，请先点击邮件中的验证按钮激活账号，再执行下方命令登录。",
      "register.done.resend": "重新发送",
      "register.done.resend_wait": "已发送 ({s}s)",
      "register.reset.kicker": "PASSWORD RESET",
      "register.reset.title": "重置密码",
      "register.reset.sub": "输入注册邮箱，我们将向你发送密码重置链接。",
      "register.reset.submit": "发送重置邮件",
      "register.reset.sent":
        "若该邮箱已注册且当前服务端已配置邮件服务，重置链接已发送，请查收邮件（未收到请联系管理员）。",
      "register.reset.back": "返回邀请码注册",
      "register.error.invalid_invite_code": "邀请码无效、已过期或可使用次数已用完。",
      "register.error.invalid_email": "请输入有效的邮箱地址。",
      "register.error.email_taken": "该邮箱已注册，可直接登录或点击下方重置密码。",
      "register.error.invalid_password": "密码长度需在 8 到 71 个字符之间。",
      "register.error.rate_limited": "操作过于频繁，请稍后再试。",
      "register.error.generic": "请求失败，请检查网络后重试。",
      "terminal.thinking": "思考中",
      "terminal.interrupt": "ESC 取消",
      "terminal.options": ["执行", "跳过此命令", "拒绝全部，由我补充说明"],
      "terminal.confirm_hint": "↑↓ 选择 · Enter 确认 · Ctrl+C 退出会话",
      "terminal.script": [
        { c: "t-you", t: "用户:" },
        { t: "\n订单接口 P99 延迟升高，流控和扩容都没效果，帮我定位瓶颈\n" },
        { think: true },
        { t: "\n" },
        { c: "t-ai", t: "AI:" },
        {
          t:
            "\n已载入环境记忆：web1 为 nginx 入口层，web2 为应用节点。\n先看本机 CPU、IO 与 TCP 状态，再连 web1 分析慢请求与连接复用。\n\n"
        },
        { c: "t-cmd", t: "查询命令:" },
        {
          t:
            "\n1/2. 采集本机 CPU、IO 与 TCP 状态\n[local] $ vmstat 1 3 && iostat -xz 1 2 | tail -20\n\n是否执行此命令?\n"
        },
        { confirm: true },
        { t: "\n" },
        { c: "t-cmd", t: "查询命令:" },
        {
          t:
            "\n2/2. 分析 web1 慢请求与 TIME_WAIT 连接\n[web1] $ awk '$NF>1{print $7,$NF}' /var/log/nginx/access.log | sort -k2 -nr | head -10\n\n是否执行此命令?\n"
        },
        { confirm: true }
      ]
    },
    en: {
      "meta.title": "AIST - The First On-Call Ops Agent",
      "meta.description":
        "AIST (AI Shell assistanT) is an On-Call operations agent that lives on your servers and in your terminal console: natural-language diagnosis, confirmed execution, continuous analysis, and private deployment that keeps data inside your network.",
      "nav.aria": "Main navigation",
      "nav.features": "Features",
      "nav.usage": "Usage",
      "nav.quickstart": "Quick start",
      "nav.contact": "Contact",
      "nav.security": "Security",
      "nav.docs": "Docs",
      "nav.register": "Register",
      "docs.href": "https://docs.aistrun.cc/",
      "nav.open": "Open menu",
      "nav.close": "Close menu",
      "lang.switch": "Switch language",
      "hero.meta.left": "THE FIRST ON-CALL OPS AGENT",
      "hero.meta.right": "● 10MB · ZERO DEPENDENCY",
      "hero.eyebrow": "[ 01 ] ON-CALL AGENT · ALWAYS IN THE TERMINAL",
      "hero.title.line1": "Run every operation",
      "hero.title.line2": "in natural language",
      "hero.lead":
        "An On-Call operations agent that lives on your servers and terminal console. Natural-language diagnosis, confirmed execution, continuous analysis — with data kept inside your network.",
      "hero.cta.primary": "Quick start",
      "hero.cta.ghost": "See security design",
      "hero.spec.binary": "Single binary",
      "hero.spec.deps": "Runtime dependencies",
      "hero.spec.value.hosted": "Self-hosted",
      "hero.spec.hosted": "Data stays internal",
      "hero.spec.value.memory": "Memory",
      "hero.spec.memory": "Learns your environment",
      "hero.platform":
        "// linux-x86_64 · linux-arm64 · linux-armv6/v7 · linux-i386 · linux-riscv64 · darwin-x86_64 · darwin-arm64",
      "hero.caption":
        "Natural language → Diagnose → Confirm → Execute → Keep analyzing",
      "features.title": "Core capabilities",
      "features.sub":
        "Built for server and cloud-native operations. Wherever there is a shell, you have an AI.",
      "feature.1.title": "Natural language interaction",
      "feature.1.text":
        "Ask in plain language and get expert diagnosis plus the most suitable shell commands.",
      "feature.2.title": "Continuous analysis",
      "feature.2.text":
        "Automatically carries the previous command's output into the next step for recursive diagnosis.",
      "feature.3.title": "Operation audit logs",
      "feature.3.text":
        "Records who ran which commands or edited which files on which server, when, and with what outcome.",
      "feature.4.title": "Pipe mode",
      "feature.4.text":
        "Analyze log output directly, processing around 13,000 lines per run and producing a report.",
      "feature.5.title": "Multi-model support",
      "feature.5.text":
        "Compatible with every OpenAI-compatible API, with automatic failover when the current model is unavailable.",
      "feature.6.title": "Safe by design",
      "feature.6.text":
        "Query and modification commands are shown differently, and modifications require a second confirmation.",
      "feature.7.title": "Command denylist",
      "feature.7.text":
        "Prompt-level and execution-level protection block sensitive commands before they run.",
      "feature.8.title": "Self-hosted deployment",
      "feature.8.text":
        "Deploy to your own servers and private network, so data never leaves the intranet.",
      "feature.9.title": "Dedicated model configuration",
      "feature.9.text":
        "Connect public-cloud or self-hosted model services, with models and quotas configured per organization.",
      "memory.title": "Environment memory · Learns your infrastructure",
      "memory.text":
        "Over time, aist remembers your environment. Next time you troubleshoot, you don't need to re-explain server environments, cluster topology or node roles — it starts the analysis with context.",
      "memory.tag.topology": "Server and cluster topology",
      "memory.tag.roles": "Node roles and responsibilities",
      "memory.tag.ports": "Common services and ports",
      "memory.tag.deploy": "Deployment methods and paths",
      "memory.tag.incidents": "Past incidents and resolutions",
      "footprint.title": "Ultra-light · Ready in seconds",
      "footprint.text":
        "Compiled into a single native binary with no Node.js, Java or other runtime. Copy it to any environment or device with a shell.",
      "footprint.aist.size": "~10 MB",
      "footprint.codex.size": "~80 MB",
      "footprint.claude.size": "~250 MB",
      "why.title": "The first On-Call ops agent",
      "why.sub":
        "Not a chat window in a browser, but an ops agent that lives on your servers, right in the terminal console.",
      "why.a.title": "From reading docs to direct diagnosis",
      "why.a.text":
        "No more jumping between search engines, documentation and the terminal. AI understands the situation and gives you an executable troubleshooting path.",
      "why.b.title": "From manual stitching to continuous reasoning",
      "why.b.text":
        "Each command's output feeds the next decision, going deeper layer by layer like an experienced engineer — not a simple Q&A.",
      "why.c.title": "From browser to terminal",
      "why.c.text":
        "No web page or extra platform required. The agent lives on your servers and terminal console, standing by where operations actually happen.",
      "why.note":
        "This is a new way to operate: natural language, live execution and environment memory form a closed loop, keeping AI on call in your console.",
      "quickstart.title": "Quick start",
      "quickstart.sub": "One-line install · Ready to use · Only ~10 MB",
      "quickstart.note.lead":
        "The script detects your OS and architecture, then installs or upgrades automatically. The first run guides you through login — ",
      "quickstart.register": "register an account",
      "quickstart.note.mid": " if needed, or reset it if you ",
      "quickstart.forgot": "forgot your password",
      "quickstart.note.tail": ".",
      "quickstart.platform":
        "// Supports linux-x86_64 · linux-arm64 · linux-armv6/v7 · linux-i386 · linux-riscv64 · darwin-x86_64 · darwin-arm64",
      "copy.label": "Copy",
      "copy.done": "Copied",
      "usage.title": "Usage",
      "usage.sub": "Three modes cover everyday operations.",
      "usage.tabs.aria": "Usage modes",
      "usage.tab.interactive": "Interactive",
      "usage.tab.single": "One-shot",
      "usage.tab.pipe": "Pipe analysis",
      "usage.interactive.desc":
        "Start a continuous conversation; AI keeps analyzing based on the previous command's output.",
      "usage.single.desc":
        "Ask once to get a diagnosis, run the suggested commands, complete the analysis and exit.",
      "usage.pipe.desc":
        "Pipe mode is read-only: it only outputs the analysis and never executes commands.",
      "usage.single.cmd":
        "$ aist \"Server load is high, help me find the cause\"",
      "usage.pipe.cmd":
        "$ tail -n 200 /var/log/nginx/access.log | aist \"Analyze the logs\"",
      "contact.bar.left": "[ ENTERPRISE · BUSINESS INQUIRIES ]",
      "contact.bar.right": "DEDICATED SUPPORT",
      "contact.title.line1": "Tailored delivery, ",
      "contact.title.line2": "inside your network",
      "contact.lead":
        "Private deployment and long-term service for enterprises, teams and departments. Share your infrastructure scale and requirements for a plan review, deployment support and a tailored quote.",
      "contact.pillar.1.val": "Self-hosted",
      "contact.pillar.1.label": "Data stays internal",
      "contact.pillar.2.val": "Long-term",
      "contact.pillar.2.label": "Org-wide accounts",
      "contact.pillar.3.val": "Any model",
      "contact.pillar.3.label": "OpenAI-compatible",
      "contact.pillar.4.val": "1-on-1",
      "contact.pillar.4.label": "End-to-end support",
      "contact.chat.badge": "LIVE CHAT · ONLINE",
      "contact.chat.title": "Live chat with our team",
      "contact.chat.desc":
        "Start a conversation right away for deployment, model compatibility, and licensing questions.",
      "contact.chat.btn": "Start live chat",
      "contact.email.badge": "DIRECT EMAIL",
      "contact.email.btn": "Send business email",
      "security.title": "Security by design",
      "security.sub":
        "Pre-check, human confirmation, full audit trail — every step stays under your control.",
      "security.flow.1": "Generate command",
      "security.flow.2": "Denylist check",
      "security.flow.3": "Pre-execution block",
      "security.flow.4": "Tiered confirmation",
      "security.flow.5": "Execute command",
      "security.flow.6": "Audit trail",
      "security.table.type": "Type",
      "security.table.label": "Label",
      "security.table.color": "Color",
      "security.table.confirm": "Confirmations",
      "security.table.example": "Example",
      "security.table.query": "Query command",
      "security.table.query.label": "Query command:",
      "security.table.color.gold": "Gold",
      "security.table.confirm.once": "1",
      "security.table.modify": "Modification command",
      "security.table.modify.label": "Modification command:",
      "security.table.color.red": "Red",
      "security.table.confirm.twice": "2",
      "security.blacklist.title": "Command denylist",
      "security.blacklist.sub.prefix":
        "The denylist is injected into the prompt, and matching commands are blocked before execution without entering confirmation. A trailing",
      "security.blacklist.sub.middle":
        "matches at least one argument; without",
      "security.blacklist.sub.suffix": ", matching is by leading token.",
      "security.blacklist.code":
        "blacklist:\n  - \"rm *\"               # block rm with arguments\n  - \"dd *\"               # block dd\n  - \"kubectl delete *\"   # block kubectl delete\n  - \"shutdown\"           # block shutdown\n  - \"reboot\"             # block reboot",
      "security.audit.title": "Operation audit logs",
      "security.audit.sub":
        "Who (account and source IP) ran which command (query or modify) or edited which file on which target server and when, whether it was approved, skipped or rejected, plus the exit code and output — everything is recorded in server-side audit logs, searchable by user, host and command in the admin console, for post-incident review and compliance.",
      "models.title": "Supported model providers",
      "models.sub": "Compatible with every OpenAI-compatible API.",
      "models.ticker": "Supported model providers",
      "models.qwen": "Qwen",
      "models.gpt": "OpenAI GPT",
      "models.deepseek": "DeepSeek",
      "models.kimi": "Moonshot Kimi",
      "models.glm": "Zhipu GLM",
      "models.compatible": "OpenAI-Compatible",
      "footer.tagline": "AI Shell assistanT · The first On-Call ops agent",
      "footer.quickstart": "Quick start",
      "footer.contact": "Contact",
      "footer.security": "Security",
      "footer.docs": "Docs",
      "footer.operational": "operational",
      "register.meta.title": "AIST - Register",
      "register.step.form": "01 · Create Account",
      "register.step.done": "02 · Install Client",
      "register.title": "Register with Invite Code",
      "register.sub": "Create your account to install and sign in from the terminal.",
      "register.field.invite": "Invite code",
      "register.placeholder.invite": "Enter invite code",
      "register.field.email": "Email",
      "register.placeholder.email": "you@example.com",
      "register.field.password": "Password",
      "register.placeholder.password": "At least 8 characters",
      "register.password.show": "Show password",
      "register.password.hide": "Hide password",
      "register.submit": "Register",
      "register.submitting": "Submitting...",
      "register.forgot_entry": "Already have an account? Forgot password?",
      "register.done.title": "Account created",
      "register.done.lead_prefix": "Account ",
      "register.done.lead_suffix": " is ready. Run the command below in your terminal to install the client and sign in:",
      "register.done.verify_notice":
        "Verification email sent. Please click the verification button in your email to activate your account before signing in.",
      "register.done.resend": "Resend",
      "register.done.resend_wait": "Sent ({s}s)",
      "register.reset.kicker": "PASSWORD RESET",
      "register.reset.title": "Reset password",
      "register.reset.sub": "Enter your account email and we will send you a password reset link.",
      "register.reset.submit": "Send reset link",
      "register.reset.sent":
        "If the email is registered and mail delivery is configured on this server, a reset link is on its way. Contact your administrator if you don't receive it.",
      "register.reset.back": "Back to invite registration",
      "register.error.invalid_invite_code": "Invite code is invalid, expired, or has no remaining uses.",
      "register.error.invalid_email": "Please enter a valid email address.",
      "register.error.email_taken": "This email is already registered. Sign in directly or reset your password below.",
      "register.error.invalid_password": "Password must be between 8 and 71 characters.",
      "register.error.rate_limited": "Too many attempts. Please wait a moment and try again.",
      "register.error.generic": "Request failed. Please check your connection and try again.",
      "terminal.thinking": "Thinking",
      "terminal.interrupt": "ESC to cancel",
      "terminal.options": [
        "Execute",
        "Skip this command",
        "Reject all and let me add details"
      ],
      "terminal.confirm_hint": "↑↓ select · Enter confirm · Ctrl+C exit session",
      "terminal.script": [
        { c: "t-you", t: "User:" },
        {
          t:
            "\nOrder API P99 latency is rising. Rate limiting and scaling didn't help — help me find the bottleneck.\n"
        },
        { think: true },
        { t: "\n" },
        { c: "t-ai", t: "AI:" },
        {
          t:
            "\nEnvironment memory loaded: web1 is the nginx entry layer, web2 is an application node.\nFirst check local CPU, IO and TCP state, then connect to web1 to analyze slow requests and connection reuse.\n\n"
        },
        { c: "t-cmd", t: "Query command:" },
        {
          t:
            "\n1/2. Collect local CPU, IO and TCP state\n[local] $ vmstat 1 3 && iostat -xz 1 2 | tail -20\n\nRun this command?\n"
        },
        { confirm: true },
        { t: "\n" },
        { c: "t-cmd", t: "Query command:" },
        {
          t:
            "\n2/2. Analyze slow requests and TIME_WAIT connections on web1\n[web1] $ awk '$NF>1{print $7,$NF}' /var/log/nginx/access.log | sort -k2 -nr | head -10\n\nRun this command?\n"
        },
        { confirm: true }
      ]
    }
  };

  function normalize(lang) {
    return /^zh\b/i.test(lang || "") ? "zh" : "en";
  }

  function detect() {
    try {
      var saved = localStorage.getItem(STORAGE_KEY);
      if (saved === "zh" || saved === "en") return saved;
    } catch (e) {
      /* ignore */
    }

    var langs =
      navigator.languages && navigator.languages.length
        ? navigator.languages
        : [navigator.language || ""];

    for (var i = 0; i < langs.length; i++) {
      if (/^zh\b/i.test(langs[i])) return "zh";
      if (/^en\b/i.test(langs[i])) return "en";
    }
    return "en";
  }

  var current = detect();

  function t(key) {
    var table = messages[current] || messages.zh;
    var value = table[key];
    return value == null ? key : value;
  }

  function each(selector, fn) {
    Array.prototype.forEach.call(document.querySelectorAll(selector), fn);
  }

  function apply(lang) {
    current = normalize(lang || current);
    var table = messages[current] || messages.zh;

    document.documentElement.lang = current === "zh" ? "zh-CN" : "en";
    document.documentElement.setAttribute("data-lang", current);

    each("[data-i18n]", function (el) {
      var key = el.getAttribute("data-i18n");
      if (Object.prototype.hasOwnProperty.call(table, key)) {
        el.textContent = table[key];
      }
    });

    each("[data-i18n-content]", function (el) {
      var key = el.getAttribute("data-i18n-content");
      if (Object.prototype.hasOwnProperty.call(table, key)) {
        el.setAttribute("content", table[key]);
      }
    });

    each("[data-i18n-aria-label]", function (el) {
      var key = el.getAttribute("data-i18n-aria-label");
      if (Object.prototype.hasOwnProperty.call(table, key)) {
        el.setAttribute("aria-label", table[key]);
      }
    });

    each("[data-i18n-title]", function (el) {
      var key = el.getAttribute("data-i18n-title");
      if (Object.prototype.hasOwnProperty.call(table, key)) {
        el.setAttribute("title", table[key]);
      }
    });

    each("[data-i18n-placeholder]", function (el) {
      var key = el.getAttribute("data-i18n-placeholder");
      if (Object.prototype.hasOwnProperty.call(table, key)) {
        el.setAttribute("placeholder", table[key]);
      }
    });

    each("[data-i18n-href]", function (el) {
      var key = el.getAttribute("data-i18n-href");
      if (Object.prototype.hasOwnProperty.call(table, key)) {
        el.setAttribute("href", table[key]);
      }
    });

    each("[data-lang-switch] [data-lang]", function (button) {
      button.setAttribute(
        "aria-pressed",
        button.getAttribute("data-lang") === current ? "true" : "false"
      );
    });
  }

  function setLanguage(lang) {
    current = normalize(lang);
    try {
      localStorage.setItem(STORAGE_KEY, current);
    } catch (e) {
      /* ignore */
    }
    apply(current);
    document.dispatchEvent(
      new CustomEvent("aist:langchange", { detail: { lang: current } })
    );
  }

  function bindSwitches() {
    each("[data-lang-switch] [data-lang]", function (button) {
      button.addEventListener("click", function () {
        var lang = button.getAttribute("data-lang");
        if (lang !== current) setLanguage(lang);
      });
    });
  }

  window.AIST_I18N = {
    getLanguage: function () {
      return current;
    },
    t: t,
    apply: apply,
    setLanguage: setLanguage,
    messages: messages
  };

  apply(current);
  bindSwitches();
})();
