Output docs/public/demo-zh.gif

Set Shell "bash"
Set FontFamily "Menlo, PingFang SC, monospace"
Set FontSize 13
Set Width 1020
Set Height 900
Set Padding 18
Set Framerate 20
Set TypingSpeed 45ms
Set Theme { "name": "AIST Dark", "black": "#18181b", "red": "#f87171", "green": "#4ade80", "yellow": "#facc15", "blue": "#60a5fa", "magenta": "#c084fc", "cyan": "#22d3ee", "white": "#e4e4e7", "brightBlack": "#71717a", "brightRed": "#fca5a5", "brightGreen": "#86efac", "brightYellow": "#fde047", "brightBlue": "#93c5fd", "brightMagenta": "#d8b4fe", "brightCyan": "#67e8f9", "brightWhite": "#fafafa", "background": "#0b0d10", "foreground": "#e4e4e7", "selection": "#27272a", "cursor": "#fafafa" }

Hide
Type "export HOME=/tmp/aist-demo-home-zh PATH=/tmp/aist-demo-bin:$PATH PS1='$ '"
Enter
Type "clear"
Enter
Sleep 300ms
Show

Sleep 400ms
Type `aist "orders-api 报 502，检查 /tmp/aist-demo-app/nginx-upstream.conf 并修复"`
Sleep 400ms
Enter

Wait+Screen /是否执行此命令/
Sleep 900ms
Enter

Wait+Screen /是否应用对/
Sleep 1100ms
Enter

Wait+Screen /警告: 该操作将修改/
Sleep 800ms
Enter

Sleep 3500ms
Ctrl+C
Sleep 1200ms
